// Package plex implements a Pilot media server plugin for Plex.
// On import_complete it triggers a library section refresh so the
// new movie appears immediately.
package plex

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/beacon-stack/pilot/internal/registry"
	"github.com/beacon-stack/pilot/internal/safedialer"
	"github.com/beacon-stack/pilot/internal/serveraddr"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

func init() {
	registry.Default.RegisterMediaServer("plex", func(settings json.RawMessage) (plugin.MediaServer, error) {
		var cfg Config
		if err := json.Unmarshal(settings, &cfg); err != nil {
			return nil, fmt.Errorf("plex: invalid settings: %w", err)
		}
		if cfg.URL == "" {
			return nil, fmt.Errorf("plex: url is required")
		}
		if cfg.Token == "" {
			return nil, fmt.Errorf("plex: token is required")
		}
		return New(cfg), nil
	})
	registry.Default.RegisterMediaServerSanitizer("plex", func(settings json.RawMessage) json.RawMessage {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(settings, &m); err != nil {
			return json.RawMessage("{}")
		}
		if _, ok := m["token"]; ok {
			m["token"] = json.RawMessage(`"***"`)
		}
		out, _ := json.Marshal(m)
		return out
	})
}

// Config holds the user-supplied settings for a Plex media server.
type Config struct {
	URL           string `json:"url"`
	Token         string `json:"token"`
	SkipTLSVerify bool   `json:"skip_tls_verify,omitempty"`
}

// Server is a Plex media server plugin instance.
type Server struct {
	cfg    Config
	client *http.Client
}

// New creates a new Server from the given config.
func New(cfg Config) *Server {
	cfg.URL = serveraddr.FromPasted(cfg.URL)
	transport := safedialer.LANTransport()
	if cfg.SkipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in for self-signed certs
	}
	return &Server{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second, Transport: transport},
	}
}

func (s *Server) Name() string { return "Plex" }

// plexSections represents the XML response from /library/sections.
type plexSections struct {
	XMLName     xml.Name      `xml:"MediaContainer"`
	Directories []plexSection `xml:"Directory"`
}

type plexSection struct {
	Key       string         `xml:"key,attr"`
	Title     string         `xml:"title,attr"`
	Type      string         `xml:"type,attr"`
	Locations []plexLocation `xml:"Location"`
}

type plexLocation struct {
	Path string `xml:"path,attr"`
}

// RefreshLibrary triggers a refresh of the Plex library section that contains
// moviePath. If no matching section is found, it falls back to refreshing all
// movie sections.
func (s *Server) RefreshLibrary(ctx context.Context, moviePath string) error {
	sections, err := s.getSections(ctx)
	if err != nil {
		return fmt.Errorf("plex: listing sections: %w", err)
	}

	// Find sections whose location path is a prefix of moviePath.
	var matched []string
	for _, sec := range sections.Directories {
		for _, loc := range sec.Locations {
			if strings.HasPrefix(moviePath, loc.Path) {
				matched = append(matched, sec.Key)
				break
			}
		}
	}

	// Fall back: refresh all movie sections if no path match.
	if len(matched) == 0 {
		for _, sec := range sections.Directories {
			if sec.Type == "movie" {
				matched = append(matched, sec.Key)
			}
		}
	}

	if len(matched) == 0 {
		return fmt.Errorf("plex: no movie library sections found")
	}

	for _, key := range matched {
		if err := s.refreshSection(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) getSections(ctx context.Context) (plexSections, error) {
	url := fmt.Sprintf("%s/library/sections", s.cfg.URL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return plexSections{}, err
	}
	req.Header.Set("X-Plex-Token", s.cfg.Token)
	req.Header.Set("Accept", "application/xml")

	resp, err := s.client.Do(req)
	if err != nil {
		return plexSections{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return plexSections{}, fmt.Errorf("plex: sections returned %d: %s", resp.StatusCode, body)
	}

	var sections plexSections
	if err := xml.NewDecoder(resp.Body).Decode(&sections); err != nil {
		return plexSections{}, fmt.Errorf("plex: decoding sections: %w", err)
	}
	return sections, nil
}

func (s *Server) refreshSection(ctx context.Context, key string) error {
	url := fmt.Sprintf("%s/library/sections/%s/refresh", s.cfg.URL, key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("plex: building refresh request: %w", err)
	}
	req.Header.Set("X-Plex-Token", s.cfg.Token)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("plex: refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("plex: refresh returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

// Test checks the address and token by asking the server for its
// libraries. Any page that answers 200 is not enough: the reply has to be
// Plex's own.
func (s *Server) Test(ctx context.Context) error {
	if _, err := s.getSections(ctx); err != nil {
		return fmt.Errorf("plex: no valid answer from %s (use the server's address, such as http://192.168.1.10:32400): %w", s.cfg.URL, err)
	}
	return nil
}

// Library sync support

// Section is a public representation of a Plex library section.
type Section struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// Movie represents a movie in a Plex library section.
type Movie struct {
	RatingKey string `json:"rating_key"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	TmdbID    int    `json:"tmdb_id"` // 0 if no TMDB guid found
}

// plexMovieContainer is the XML response from /library/sections/{key}/all.
type plexMovieContainer struct {
	XMLName xml.Name    `xml:"MediaContainer"`
	Videos  []plexVideo `xml:"Video"`
}

type plexVideo struct {
	RatingKey string    `xml:"ratingKey,attr"`
	Title     string    `xml:"title,attr"`
	Year      int       `xml:"year,attr"`
	GUID      string    `xml:"guid,attr"` // legacy agent format
	Guids     []plexGID `xml:"Guid"`      // new agent format
}

type plexGID struct {
	ID string `xml:"id,attr"` // e.g. "tmdb://12345", "imdb://tt1234567"
}

// ListSections returns the movie library sections from this Plex server.
func (s *Server) ListSections(ctx context.Context) ([]Section, error) {
	raw, err := s.getSections(ctx)
	if err != nil {
		return nil, err
	}
	var sections []Section
	for _, d := range raw.Directories {
		if d.Type == "movie" {
			sections = append(sections, Section{Key: d.Key, Title: d.Title, Type: d.Type})
		}
	}
	return sections, nil
}

// ListMovies returns all movies in the given Plex library section.
func (s *Server) ListMovies(ctx context.Context, sectionKey string) ([]Movie, error) {
	reqURL := fmt.Sprintf("%s/library/sections/%s/all?includeGuids=1", s.cfg.URL, sectionKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("plex: building list-movies request: %w", err)
	}
	req.Header.Set("X-Plex-Token", s.cfg.Token)
	req.Header.Set("Accept", "application/xml")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plex: list-movies request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("plex: list-movies returned %d: %s", resp.StatusCode, body)
	}

	var container plexMovieContainer
	if err := xml.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("plex: decoding movies: %w", err)
	}

	movies := make([]Movie, 0, len(container.Videos))
	for _, v := range container.Videos {
		movies = append(movies, Movie{
			RatingKey: v.RatingKey,
			Title:     v.Title,
			Year:      v.Year,
			TmdbID:    extractTmdbID(v),
		})
	}
	return movies, nil
}

// plexShowContainer is the XML response from
// /library/sections/{key}/all?type=2 (shows).
type plexShowContainer struct {
	XMLName xml.Name   `xml:"MediaContainer"`
	Shows   []plexShow `xml:"Directory"`
}

type plexShow struct {
	RatingKey string    `xml:"ratingKey,attr"`
	Title     string    `xml:"title,attr"`
	Year      int       `xml:"year,attr"`
	GUID      string    `xml:"guid,attr"`
	Guids     []plexGID `xml:"Guid"`
}

// plexEpisodeContainer is the XML response from
// /library/sections/{key}/all?type=4 (episodes).
type plexEpisodeContainer struct {
	XMLName  xml.Name      `xml:"MediaContainer"`
	Episodes []plexEpisode `xml:"Video"`
}

type plexEpisode struct {
	ShowRatingKey string `xml:"grandparentRatingKey,attr"`
	Season        int    `xml:"parentIndex,attr"`
	Episode       int    `xml:"index,attr"`
}

// getXML fetches a path from the server and decodes its XML into out.
func (s *Server) getXML(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.URL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Plex-Token", s.cfg.Token)
	req.Header.Set("Accept", "application/xml")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("returned %d: %s", resp.StatusCode, body)
	}
	return xml.NewDecoder(resp.Body).Decode(out)
}

// ListShowItems returns every series in every TV library on the server,
// with the episodes it holds. It implements plugin.ShowLister. Each
// library costs two requests: its shows, then all its episodes.
func (s *Server) ListShowItems(ctx context.Context) ([]plugin.ShowItem, error) {
	sections, err := s.getSections(ctx)
	if err != nil {
		return nil, err
	}
	var items []plugin.ShowItem
	for _, sec := range sections.Directories {
		if sec.Type != "show" {
			continue
		}
		var shows plexShowContainer
		if err := s.getXML(ctx, "/library/sections/"+sec.Key+"/all?type=2&includeGuids=1", &shows); err != nil {
			return nil, fmt.Errorf("plex: listing shows in %q: %w", sec.Title, err)
		}
		var episodes plexEpisodeContainer
		if err := s.getXML(ctx, "/library/sections/"+sec.Key+"/all?type=4", &episodes); err != nil {
			return nil, fmt.Errorf("plex: listing episodes in %q: %w", sec.Title, err)
		}
		byShow := make(map[string][]plugin.EpisodeNumber, len(shows.Shows))
		for _, e := range episodes.Episodes {
			byShow[e.ShowRatingKey] = append(byShow[e.ShowRatingKey], plugin.EpisodeNumber{Season: e.Season, Episode: e.Episode})
		}
		for _, sh := range shows.Shows {
			items = append(items, plugin.ShowItem{
				TMDBID:   guidID(sh.Guids, sh.GUID, "tmdb://", "themoviedb://"),
				TVDBID:   guidID(sh.Guids, sh.GUID, "tvdb://", "thetvdb://"),
				Title:    sh.Title,
				Year:     sh.Year,
				Episodes: byShow[sh.RatingKey],
			})
		}
	}
	return items, nil
}

// guidID finds a numeric ID among a Plex item's guids. The current agent
// lists them as child elements (<Guid id="tvdb://121361"/>); the legacy
// agents put one in the guid attribute
// (com.plexapp.agents.thetvdb://121361?lang=en).
func guidID(guids []plexGID, legacy, prefix, legacyMarker string) int {
	for _, g := range guids {
		if rest, ok := strings.CutPrefix(g.ID, prefix); ok {
			if id, err := strconv.Atoi(rest); err == nil {
				return id
			}
		}
	}
	if _, rest, ok := strings.Cut(legacy, legacyMarker); ok {
		idStr, _, _ := strings.Cut(rest, "?")
		idStr, _, _ = strings.Cut(idStr, "/")
		if id, err := strconv.Atoi(idStr); err == nil {
			return id
		}
	}
	return 0
}

// extractTmdbID tries to find a TMDB ID from the video's guid fields.
// New Plex agent: <Guid id="tmdb://12345"/>
// Legacy agent: guid="com.plexapp.agents.themoviedb://12345?lang=en"
func extractTmdbID(v plexVideo) int {
	// New agent format: child <Guid> elements.
	for _, g := range v.Guids {
		if strings.HasPrefix(g.ID, "tmdb://") {
			if id, err := strconv.Atoi(strings.TrimPrefix(g.ID, "tmdb://")); err == nil {
				return id
			}
		}
	}
	// Legacy agent format: top-level guid attribute.
	if strings.Contains(v.GUID, "themoviedb://") {
		// Format: com.plexapp.agents.themoviedb://12345?lang=en
		parts := strings.SplitN(v.GUID, "themoviedb://", 2)
		if len(parts) == 2 {
			idStr := strings.SplitN(parts[1], "?", 2)[0]
			if id, err := strconv.Atoi(idStr); err == nil {
				return id
			}
		}
	}
	return 0
}

// WatchProvider implementation

// plexHistoryContainer wraps the Plex watch history XML response.
type plexHistoryContainer struct {
	XMLName  xml.Name          `xml:"MediaContainer"`
	Metadata []plexHistoryItem `xml:"Metadata"`
}

type plexHistoryItem struct {
	RatingKey string    `xml:"ratingKey,attr"`
	Title     string    `xml:"title,attr"`
	Type      string    `xml:"type,attr"`
	ViewedAt  int64     `xml:"viewedAt,attr"` // Unix timestamp
	AccountID int       `xml:"accountID,attr"`
	GUID      string    `xml:"guid,attr"`
	Guids     []plexGID `xml:"Guid"`
}

// WatchHistory returns watch events since the given timestamp.
// Implements plugin.WatchProvider.
func (s *Server) WatchHistory(ctx context.Context, since time.Time) ([]plugin.WatchEvent, error) {
	// Plex history API: GET /status/sessions/history/all
	// Sorted by viewedAt descending. Filter with viewedAt>= (Unix timestamp).
	sinceUnix := since.Unix()
	reqURL := fmt.Sprintf("%s/status/sessions/history/all?sort=viewedAt:desc&viewedAt%%3E=%d&includeGuids=1",
		s.cfg.URL, sinceUnix)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("plex: building watch-history request: %w", err)
	}
	req.Header.Set("X-Plex-Token", s.cfg.Token)
	req.Header.Set("Accept", "application/xml")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plex: watch-history request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("plex: watch-history returned %d: %s", resp.StatusCode, body)
	}

	var container plexHistoryContainer
	if err := xml.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("plex: decoding watch history: %w", err)
	}

	var events []plugin.WatchEvent
	for _, item := range container.Metadata {
		if item.Type != "movie" {
			continue
		}

		// Extract TMDB ID using the same logic as ListMovies.
		tmdbID := extractTmdbID(plexVideo{
			GUID:  item.GUID,
			Guids: item.Guids,
		})
		if tmdbID == 0 {
			continue // skip items without TMDB mapping
		}

		events = append(events, plugin.WatchEvent{
			TMDBID:    tmdbID,
			Title:     item.Title,
			WatchedAt: time.Unix(item.ViewedAt, 0).UTC(),
			UserName:  fmt.Sprintf("plex-account-%d", item.AccountID),
		})
	}

	return events, nil
}
