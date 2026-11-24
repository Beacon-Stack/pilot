// Package torznab implements the Torznab indexer plugin for Pilot.
// Torznab is an RSS/Atom-style XML protocol used by Prowlarr and Jackett
// to expose torrent indexers over a common API.
package torznab

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/beacon-stack/pilot/internal/redact"
	"github.com/beacon-stack/pilot/internal/registry"
	"github.com/beacon-stack/pilot/internal/safedialer"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

func init() {
	registry.Default.RegisterIndexer("torznab", func(settings json.RawMessage) (plugin.Indexer, error) {
		var cfg Config
		if err := json.Unmarshal(settings, &cfg); err != nil {
			return nil, fmt.Errorf("torznab: invalid settings: %w", err)
		}
		if cfg.URL == "" {
			return nil, fmt.Errorf("torznab: url is required")
		}
		return New(cfg), nil
	})
	registry.Default.RegisterIndexerSanitizer("torznab", func(settings json.RawMessage) json.RawMessage {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(settings, &m); err != nil {
			return json.RawMessage("{}")
		}
		if _, ok := m["api_key"]; ok {
			m["api_key"] = json.RawMessage(`"***"`)
		}
		out, _ := json.Marshal(m)
		return out
	})
}

// Config holds the user-supplied settings for a Torznab indexer instance.
type Config struct {
	URL       string `json:"url"`
	APIKey    string `json:"api_key,omitempty"`
	RateLimit int    `json:"rate_limit,omitempty"` // requests per minute; 0 = unlimited
}

// Indexer is a Torznab indexer plugin instance.
type Indexer struct {
	cfg    Config
	client *http.Client

	// What the caps document says searches may carry, read once and
	// reused. See searchCaps.
	capsMu      sync.Mutex
	caps        *searchCaps
	capsCheckAt time.Time

	// Whether tvsearch earns its round trip on this indexer; see
	// tryTVSearch.
	tvMu      sync.Mutex
	tvUseless int // searches in a row where tvsearch found nothing but text search did
	tvSkipped int // tvsearch skips since the last probe
}

const (
	// tvUselessAfter is how many searches in a row may find results by
	// text search after an empty (or failed) tvsearch before tvsearch is
	// skipped: on such an indexer it is a wasted round trip every time.
	tvUselessAfter = 3
	// tvReprobeEvery: while skipping, every this many searches try
	// tvsearch again, in case the indexer has started answering it.
	tvReprobeEvery = 20
)

// tryTVSearch reports whether this search should try tvsearch before the
// text search.
func (idx *Indexer) tryTVSearch(caps *searchCaps) bool {
	if caps != nil && caps.tvUnavailable {
		return false
	}
	idx.tvMu.Lock()
	defer idx.tvMu.Unlock()
	if idx.tvUseless < tvUselessAfter {
		return true
	}
	idx.tvSkipped++
	if idx.tvSkipped >= tvReprobeEvery {
		idx.tvSkipped = 0
		return true
	}
	return false
}

// noteTVSearch records how a search that tried tvsearch went.
func (idx *Indexer) noteTVSearch(tvFound, textFound bool) {
	idx.tvMu.Lock()
	defer idx.tvMu.Unlock()
	switch {
	case tvFound:
		idx.tvUseless, idx.tvSkipped = 0, 0
	case textFound:
		idx.tvUseless++
	}
}

// searchCaps is the part of an indexer's caps document a search needs.
type searchCaps struct {
	// tvParams are the tv-search supportedParams, lowercased ("q",
	// "season", "imdbid", ...). Empty when the indexer didn't list them.
	tvParams map[string]bool
	// tvCategories are the indexer's TV categories (5000-5999).
	tvCategories []int
	// tvUnavailable: the caps say tv-search is not available, so a
	// tvsearch request could only fail before the text search runs.
	tvUnavailable bool
}

const (
	// capsTTL is how long a caps document is trusted. Caps change only
	// when the indexer's definition does.
	capsTTL = 6 * time.Hour
	// capsRetryAfter is how long to search without caps after the caps
	// request failed, before asking again.
	capsRetryAfter = 10 * time.Minute
	// capsTimeout bounds the caps request made ahead of a search, so an
	// indexer whose caps hang costs that search seconds, not its budget.
	capsTimeout = 5 * time.Second
)

// New creates a new Torznab Indexer from the given config.
//
// The 30s per-call timeout matches Newznab and the Sonarr/Radarr
// defaults. A higher value lets one misbehaving indexer (Cloudflare-
// walled, dead or overloaded) hold up an interactive search.
func New(cfg Config) *Indexer {
	return &Indexer{
		cfg: cfg,
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: safedialer.LANTransport(),
		},
	}
}

// Name returns the human-readable plugin name.
func (idx *Indexer) Name() string { return "Torznab" }

// Protocol returns the protocol this indexer serves.
func (idx *Indexer) Protocol() plugin.Protocol { return plugin.ProtocolTorrent }

// Capabilities fetches and parses the indexer's capabilities document.
func (idx *Indexer) Capabilities(ctx context.Context) (plugin.Capabilities, error) {
	caps, err := idx.fetchCaps(ctx)
	if err != nil {
		return plugin.Capabilities{}, err
	}

	categories := make([]int, 0, len(caps.Categories.Items))
	for _, c := range caps.Categories.Items {
		categories = append(categories, c.ID)
	}

	return plugin.Capabilities{
		SearchAvailable:   caps.Searching.Search.Available == "yes",
		TVSearchAvailable: caps.Searching.TVSearch.Available == "yes",
		MovieSearch:       caps.Searching.MovieSearch.Available == "yes",
		Categories:        categories,
	}, nil
}

func (idx *Indexer) fetchCaps(ctx context.Context) (capsResponse, error) {
	u := idx.buildURL("caps", url.Values{})
	resp, err := idx.get(ctx, u)
	if err != nil {
		return capsResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return capsResponse{}, fmt.Errorf("torznab: capabilities returned HTTP %d", resp.StatusCode)
	}

	var caps capsResponse
	if err := decodeXML(resp.Body, &caps); err != nil {
		var terr *torznabError
		if errors.As(err, &terr) {
			return capsResponse{}, fmt.Errorf("torznab: %w", terr)
		}
		return capsResponse{}, fmt.Errorf("torznab: decoding capabilities: %w", err)
	}
	return caps, nil
}

// searchCaps returns the cached caps, fetching them when there are none
// or they are old. Nil when the indexer can't say: searches then go out
// as they always did, with no IDs and no category filter.
func (idx *Indexer) searchCaps(ctx context.Context) *searchCaps {
	idx.capsMu.Lock()
	defer idx.capsMu.Unlock()
	if time.Now().Before(idx.capsCheckAt) {
		return idx.caps
	}

	cctx, cancel := context.WithTimeout(ctx, capsTimeout)
	defer cancel()
	doc, err := idx.fetchCaps(cctx)
	if err != nil {
		// Keep whatever we had; a caps hiccup must not change searches.
		idx.capsCheckAt = time.Now().Add(capsRetryAfter)
		return idx.caps
	}
	idx.caps = parseSearchCaps(doc)
	idx.capsCheckAt = time.Now().Add(capsTTL)
	return idx.caps
}

func parseSearchCaps(doc capsResponse) *searchCaps {
	sc := &searchCaps{
		tvParams:      map[string]bool{},
		tvUnavailable: strings.EqualFold(strings.TrimSpace(doc.Searching.TVSearch.Available), "no"),
	}
	for _, p := range strings.Split(doc.Searching.TVSearch.SupportedParams, ",") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			sc.tvParams[p] = true
		}
	}
	// Top-level TV categories only. Subcategory IDs are often the site's
	// own ("50TV-X265", "501_0" on Pulse) rather than standard numbers,
	// and every implementation treats the parent as including them.
	for _, c := range doc.Categories.Items {
		if c.ID >= 5000 && c.ID < 6000 {
			sc.tvCategories = append(sc.tvCategories, c.ID)
		}
	}
	return sc
}

// supports reports whether a tv-search may carry param. IDs are only
// sent when the indexer lists them: an indexer that doesn't know an ID
// can answer an ID search with nothing, which costs a text fallback.
func (sc *searchCaps) supports(param string) bool {
	return sc != nil && sc.tvParams[param]
}

// categories is the cat= value for a TV search, or "" to send none.
func (sc *searchCaps) categories() string {
	if sc == nil || len(sc.tvCategories) == 0 {
		return ""
	}
	parts := make([]string, len(sc.tvCategories))
	for i, c := range sc.tvCategories {
		parts[i] = strconv.Itoa(c)
	}
	return strings.Join(parts, ",")
}

// Search queries the indexer for releases matching q.
//
// Strategy:
//  1. Try the TV search endpoint (?t=tvsearch) with season/episode params.
//     This lets the indexer filter server-side for better results.
//     If TVDBID is available, include it for more precise matching.
//  2. Fall back to free-text search (?t=search&q=…) which is universally
//     supported. The caller is expected to pass a well-formed show query in
//     q.Query (e.g. "Breaking Bad S01E05").
func (idx *Indexer) Search(ctx context.Context, q plugin.SearchQuery) ([]plugin.Release, error) {
	// 1. TV search with season/episode params — lets the indexer filter server-side.
	//
	// q is sent verbatim with any "S01" / "Season N" markers intact even
	// though the structured `season=` / `ep=` params are ALSO set. We
	// learned the hard way: many torznab implementations (TorrentGalaxy
	// is the canonical case) silently ignore `season=` and only text-
	// match `q`. Stripping the markers reduced those indexers to fuzzy
	// "Andor" → "Anderson / Andrea" substring garbage. The downside of
	// duplication (text gate + structured gate stacking) is strictly
	// less harmful than losing 95% of results from a healthy indexer.
	//
	// IDs and the TV category filter come from the indexer's caps: an
	// ID lets the indexer match the show exactly, and the category keeps
	// it from returning (and scraping pages of) movies, music and games.
	caps := idx.searchCaps(ctx)
	cats := caps.categories()

	var tvSearchErr error
	// tvdbid is sent when the caps list it, or when there are no caps to
	// ask (as before caps were read); imdbid/tmdbid only when listed.
	sendTVDB := q.TVDBID != 0 && (caps == nil || caps.supports("tvdbid"))
	hasID := sendTVDB || (q.IMDBID != "" && caps.supports("imdbid")) ||
		(q.TMDBID != 0 && caps.supports("tmdbid"))
	triedTV := false
	if (q.Season > 0 || hasID) && idx.tryTVSearch(caps) {
		triedTV = true
		params := url.Values{}
		params.Set("q", q.Query)
		if sendTVDB {
			params.Set("tvdbid", strconv.Itoa(q.TVDBID))
		}
		if q.IMDBID != "" && caps.supports("imdbid") {
			params.Set("imdbid", q.IMDBID)
		}
		if q.TMDBID != 0 && caps.supports("tmdbid") {
			params.Set("tmdbid", strconv.Itoa(q.TMDBID))
		}
		if q.Season > 0 {
			params.Set("season", strconv.Itoa(q.Season))
		}
		if q.Episode > 0 {
			params.Set("ep", strconv.Itoa(q.Episode))
		}
		if cats != "" {
			params.Set("cat", cats)
		}
		u := idx.buildURL("tvsearch", params)
		releases, err := idx.fetchReleases(ctx, u)
		if err == nil && len(releases) > 0 {
			idx.noteTVSearch(true, false)
			return releases, nil
		}
		// Zero results or error — fall through to text search, but keep
		// the error: if the fallback is also empty, a failed tvsearch must
		// not be reported as a healthy zero-result search.
		tvSearchErr = err
	}

	// 2. Free-text search — universally supported fallback.
	params := url.Values{}
	params.Set("q", q.Query)
	if cats != "" {
		params.Set("cat", cats)
	}
	u := idx.buildURL("search", params)
	releases, err := idx.fetchReleases(ctx, u)
	if err != nil {
		return nil, err
	}
	if triedTV {
		idx.noteTVSearch(false, len(releases) > 0)
	}
	if len(releases) == 0 && tvSearchErr != nil {
		return nil, tvSearchErr
	}
	return releases, nil
}

// GetRecent returns the most recent releases from the indexer's RSS feed.
func (idx *Indexer) GetRecent(ctx context.Context) ([]plugin.Release, error) {
	u := idx.buildURL("tvsearch", url.Values{})
	return idx.fetchReleases(ctx, u)
}

// Test checks that the indexer is reachable and returns a valid capabilities
// response. Returns a non-nil error if connectivity or configuration fails.
func (idx *Indexer) Test(ctx context.Context) error {
	u := idx.buildURL("caps", url.Values{})
	resp, err := idx.get(ctx, u)
	if err != nil {
		return fmt.Errorf("torznab: test request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("torznab: test returned HTTP %d", resp.StatusCode)
	}

	var caps capsResponse
	if err := decodeXML(resp.Body, &caps); err != nil {
		// An empty body means the indexer answered HTTP 200 but returned no
		// caps document. Some Prowlarr-backed indexers do this while still
		// serving search results.
		if errors.Is(err, errEmptyBody) {
			return nil
		}
		var terr *torznabError
		if errors.As(err, &terr) {
			return fmt.Errorf("torznab: %w", terr)
		}
		return fmt.Errorf("torznab: invalid caps response (wrong URL?): %w", err)
	}
	return nil
}

// buildURL constructs the full API URL for the given Torznab function and
// additional query parameters. The apikey is appended when configured.
// Trailing slashes on the configured URL are stripped so callers can enter
// either "http://host/5" or "http://host/5/" and both produce valid URLs.
func (idx *Indexer) buildURL(t string, params url.Values) string {
	params.Set("t", t)
	if idx.cfg.APIKey != "" {
		params.Set("apikey", idx.cfg.APIKey)
	}
	base := strings.TrimRight(idx.cfg.URL, "/")
	return base + "/api?" + params.Encode()
}

// get executes an HTTP GET against the given URL using the context for
// cancellation and deadline propagation.
func (idx *Indexer) get(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("torznab: building request: %w", redact.Error(err, idx.cfg.APIKey))
	}
	resp, err := idx.client.Do(req)
	if err != nil {
		// *url.Error includes the full URL, apikey and all.
		return nil, redact.Error(err, idx.cfg.APIKey)
	}
	return resp, nil
}

// maxResponseBytes caps how much of a response body decodeXML will read.
// The largest legitimate feeds are a few MB; the cap only guards against a
// misbehaving endpoint streaming forever.
const maxResponseBytes = 32 << 20

// errEmptyBody signals an HTTP-200 response with an empty body. Callers map
// it to their own contextual message (Test tolerates it entirely).
var errEmptyBody = errors.New("empty response body")

// torznabError is the protocol's standard failure document
// (<error code="…" description="…"/>), served with HTTP 200.
type torznabError struct {
	XMLName     xml.Name `xml:"error"`
	Code        string   `xml:"code,attr"`
	Description string   `xml:"description,attr"`
}

func (e *torznabError) Error() string {
	return fmt.Sprintf("indexer reported error %s: %s", e.Code, e.Description)
}

// decodeXML buffers the response body and decodes it into v. Torznab signals
// failures as an <error> document rather than an HTTP status, so that case is
// decoded explicitly and returned as *torznabError — otherwise the indexer's
// own description would be swallowed by an opaque "expected element type
// <rss>" mismatch from the decoder.
func decodeXML(r io.Reader, v any) error {
	data, err := io.ReadAll(io.LimitReader(r, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return errEmptyBody
	}
	protoErr := &torznabError{}
	if xml.Unmarshal(data, protoErr) == nil {
		return protoErr
	}
	return xml.Unmarshal(data, v)
}

// fetchReleases performs a GET to rawURL, decodes the Torznab RSS response,
// and returns the parsed releases.
func (idx *Indexer) fetchReleases(ctx context.Context, rawURL string) ([]plugin.Release, error) {
	resp, err := idx.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("torznab: search returned HTTP %d", resp.StatusCode)
	}

	var feed rssResponse
	if err := decodeXML(resp.Body, &feed); err != nil {
		if errors.Is(err, errEmptyBody) {
			return nil, fmt.Errorf("torznab: empty response body — check the API key and indexer URL")
		}
		var terr *torznabError
		if errors.As(err, &terr) {
			return nil, fmt.Errorf("torznab: %w", terr)
		}
		return nil, fmt.Errorf("torznab: decoding response: %w", err)
	}

	releases := make([]plugin.Release, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		releases = append(releases, idx.toRelease(item))
	}
	return releases, nil
}

// toRelease converts a parsed RSS item into a plugin.Release.
func (idx *Indexer) toRelease(item rssItem) plugin.Release {
	guid := item.GUID.Value
	if guid == "" {
		guid = item.Link
	}

	r := plugin.Release{
		GUID:        guid,
		Title:       item.Title,
		Indexer:     item.JackettIndexer, // Prowlarr fills this; empty for standalone indexers.
		Protocol:    plugin.ProtocolTorrent,
		DownloadURL: item.Enclosure.URL,
		InfoURL:     item.Link,
		Size:        item.Enclosure.Length,
	}

	var flags []plugin.IndexerFlag
	for _, attr := range item.Attrs {
		switch attr.Name {
		case "seeders":
			r.Seeds, _ = strconv.Atoi(attr.Value)
		case "peers":
			r.Peers, _ = strconv.Atoi(attr.Value)
		case "size":
			if sz, err := strconv.ParseInt(attr.Value, 10, 64); err == nil && sz > 0 {
				r.Size = sz
			}
		case "downloadvolumefactor":
			switch attr.Value {
			case "0":
				flags = append(flags, plugin.FlagFreeleech)
			case "0.5":
				flags = append(flags, plugin.FlagHalfleech)
			case "0.75":
				flags = append(flags, plugin.FlagFreeleech25)
			case "0.25":
				flags = append(flags, plugin.FlagFreeleech75)
			}
		case "uploadvolumefactor":
			if attr.Value == "2" {
				flags = append(flags, plugin.FlagDoubleUpload)
			}
		case "tag":
			switch strings.ToLower(attr.Value) {
			case "freeleech":
				flags = append(flags, plugin.FlagFreeleech)
			case "halfleech":
				flags = append(flags, plugin.FlagHalfleech)
			case "internal":
				flags = append(flags, plugin.FlagInternal)
			case "scene":
				flags = append(flags, plugin.FlagScene)
			case "nuked":
				flags = append(flags, plugin.FlagNuked)
			}
		}
	}
	r.IndexerFlags = dedupeFlags(flags)
	r.AgeDays = parseAgeDays(item.PubDate)
	return r
}

// dedupeFlags removes duplicate flags while preserving order.
func dedupeFlags(flags []plugin.IndexerFlag) []plugin.IndexerFlag {
	if len(flags) == 0 {
		return nil
	}
	seen := make(map[plugin.IndexerFlag]struct{}, len(flags))
	out := make([]plugin.IndexerFlag, 0, len(flags))
	for _, f := range flags {
		if _, ok := seen[f]; !ok {
			seen[f] = struct{}{}
			out = append(out, f)
		}
	}
	return out
}

// parseAgeDays parses an RFC1123Z pubDate string and returns the number of
// calendar days elapsed since that time. Returns 0 on parse failure.
func parseAgeDays(pubDate string) float64 {
	if pubDate == "" {
		return 0
	}
	t, err := time.Parse(time.RFC1123Z, pubDate)
	if err != nil {
		t, err = time.Parse(time.RFC1123, pubDate)
		if err != nil {
			return 0
		}
	}
	return time.Since(t).Hours() / 24
}

// XML types

type rssResponse struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title          string        `xml:"title"`
	GUID           rssGUID       `xml:"guid"`
	Link           string        `xml:"link"`
	PubDate        string        `xml:"pubDate"`
	Enclosure      enclosure     `xml:"enclosure"`
	JackettIndexer string        `xml:"jackettindexer"`
	Attrs          []torznabAttr `xml:"http://torznab.com/schemas/2015/feed attr"`
}

type rssGUID struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

type enclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

type torznabAttr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type capsResponse struct {
	XMLName    xml.Name       `xml:"caps"`
	Searching  capsSearching  `xml:"searching"`
	Categories capsCategories `xml:"categories"`
}

type capsSearching struct {
	Search      capsSearchType `xml:"search"`
	TVSearch    capsSearchType `xml:"tv-search"`
	MovieSearch capsSearchType `xml:"movie-search"`
}

type capsSearchType struct {
	Available       string `xml:"available,attr"`
	SupportedParams string `xml:"supportedParams,attr"`
}

type capsCategories struct {
	Items []capsCategory `xml:"category"`
}

// capsCategory is a top-level category. Subcategories aren't read: their
// IDs need not be numbers (see parseSearchCaps), and an int field would
// fail the whole document on the first one that isn't.
type capsCategory struct {
	ID   int    `xml:"id,attr"`
	Name string `xml:"name,attr"`
}
