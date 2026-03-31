// Package newznab implements the Newznab indexer plugin for Screenarr.
// Newznab is an RSS/Atom-style XML protocol for NZB indexers. It shares the
// same feed envelope and API parameter shape as Torznab, but serves NZB
// releases rather than torrents — so there are no seeder/peer attributes.
package newznab

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/screenarr/screenarr/internal/registry"
	"github.com/screenarr/screenarr/internal/safedialer"
	"github.com/screenarr/screenarr/pkg/plugin"
)

func init() {
	registry.Default.RegisterIndexer("newznab", func(settings json.RawMessage) (plugin.Indexer, error) {
		var cfg Config
		if err := json.Unmarshal(settings, &cfg); err != nil {
			return nil, fmt.Errorf("newznab: invalid settings: %w", err)
		}
		if cfg.URL == "" {
			return nil, fmt.Errorf("newznab: url is required")
		}
		return New(cfg), nil
	})
	registry.Default.RegisterIndexerSanitizer("newznab", func(settings json.RawMessage) json.RawMessage {
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

// Config holds the user-supplied settings for a Newznab indexer instance.
type Config struct {
	URL       string `json:"url"`
	APIKey    string `json:"api_key,omitempty"`
	RateLimit int    `json:"rate_limit,omitempty"` // requests per minute; 0 = unlimited
}

// Indexer is a Newznab indexer plugin instance.
type Indexer struct {
	cfg    Config
	client *http.Client
}

// New creates a new Newznab Indexer from the given config.
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
func (idx *Indexer) Name() string { return "Newznab" }

// Protocol returns the protocol this indexer serves.
func (idx *Indexer) Protocol() plugin.Protocol { return plugin.ProtocolNZB }

// Capabilities fetches and parses the indexer's capabilities document.
func (idx *Indexer) Capabilities(ctx context.Context) (plugin.Capabilities, error) {
	u := idx.buildURL("caps", url.Values{})
	resp, err := idx.get(ctx, u)
	if err != nil {
		return plugin.Capabilities{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return plugin.Capabilities{}, fmt.Errorf("newznab: capabilities returned HTTP %d", resp.StatusCode)
	}

	var caps capsResponse
	if err := xml.NewDecoder(resp.Body).Decode(&caps); err != nil {
		return plugin.Capabilities{}, fmt.Errorf("newznab: decoding capabilities: %w", err)
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

// Search queries the indexer for releases matching q.
// Uses TV search (?t=tvsearch&tvdbid=…) when a TVDB ID is available,
// otherwise falls back to free-text search. The caller should build a
// well-formed show query in q.Query (e.g. "Breaking Bad S01E05").
func (idx *Indexer) Search(ctx context.Context, q plugin.SearchQuery) ([]plugin.Release, error) {
	if q.TVDBID != 0 {
		params := url.Values{}
		params.Set("tvdbid", strconv.Itoa(q.TVDBID))
		params.Set("cat", "5000") // TV category
		u := idx.buildURL("tvsearch", params)
		releases, err := idx.fetchReleases(ctx, u)
		if err == nil && len(releases) > 0 {
			return releases, nil
		}
		// Fall through to text search.
	}

	params := url.Values{}
	params.Set("q", q.Query)
	params.Set("cat", "5000")
	u := idx.buildURL("search", params)
	return idx.fetchReleases(ctx, u)
}

// GetRecent returns the most recent TV releases from the indexer's RSS feed.
func (idx *Indexer) GetRecent(ctx context.Context) ([]plugin.Release, error) {
	params := url.Values{}
	params.Set("cat", "5000")
	u := idx.buildURL("tvsearch", params)
	return idx.fetchReleases(ctx, u)
}

// Test checks that the indexer is reachable and returns a valid capabilities
// response. Returns a non-nil error if connectivity or configuration fails.
func (idx *Indexer) Test(ctx context.Context) error {
	u := idx.buildURL("caps", url.Values{})
	resp, err := idx.get(ctx, u)
	if err != nil {
		return fmt.Errorf("newznab: test request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("newznab: test returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// buildURL constructs the full API URL for the given Newznab function and
// additional query parameters.
func (idx *Indexer) buildURL(t string, params url.Values) string {
	params.Set("t", t)
	if idx.cfg.APIKey != "" {
		params.Set("apikey", idx.cfg.APIKey)
	}
	base := strings.TrimRight(idx.cfg.URL, "/")
	return base + "/api?" + params.Encode()
}

// get executes an HTTP GET against the given URL using the context.
func (idx *Indexer) get(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("newznab: building request: %w", err)
	}
	return idx.client.Do(req)
}

// fetchReleases performs a GET to rawURL, decodes the Newznab RSS response,
// and returns the parsed releases.
func (idx *Indexer) fetchReleases(ctx context.Context, rawURL string) ([]plugin.Release, error) {
	resp, err := idx.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("newznab: search returned HTTP %d", resp.StatusCode)
	}

	var feed rssResponse
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return nil, fmt.Errorf("newznab: decoding response: %w", err)
	}

	releases := make([]plugin.Release, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		releases = append(releases, idx.toRelease(item))
	}
	return releases, nil
}

// toRelease converts a parsed RSS item into a plugin.Release.
// Seeds and Peers are always 0 for NZB releases.
func (idx *Indexer) toRelease(item rssItem) plugin.Release {
	guid := item.GUID.Value
	if guid == "" {
		guid = item.Link
	}

	r := plugin.Release{
		GUID:        guid,
		Title:       item.Title,
		Indexer:     item.JackettIndexer,
		Protocol:    plugin.ProtocolNZB,
		DownloadURL: item.Enclosure.URL,
		InfoURL:     item.Link,
		Size:        item.Enclosure.Length,
	}

	var flags []plugin.IndexerFlag
	for _, attr := range item.Attrs {
		switch attr.Name {
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
			}
		case "uploadvolumefactor":
			if attr.Value == "2" {
				flags = append(flags, plugin.FlagDoubleUpload)
			}
		case "tag":
			switch strings.ToLower(attr.Value) {
			case "freeleech":
				flags = append(flags, plugin.FlagFreeleech)
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

// ---------------------------------------------------------------------------
// XML types
// ---------------------------------------------------------------------------

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
	Attrs          []newznabAttr `xml:"http://www.newznab.com/DTD/2010/feeds/attributes/ attr"`
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

type newznabAttr struct {
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
	Available string `xml:"available,attr"`
}

type capsCategories struct {
	Items []capsCategory `xml:"category"`
}

type capsCategory struct {
	ID   int    `xml:"id,attr"`
	Name string `xml:"name,attr"`
}
