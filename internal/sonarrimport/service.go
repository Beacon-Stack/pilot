package sonarrimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	"github.com/beacon-stack/pilot/internal/core/library"
	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ── Result types ─────────────────────────────────────────────────────────────

// PreviewResult summarises what would be imported from a Sonarr instance.
type PreviewResult struct {
	Version         string           `json:"version"`
	SeriesCount     int              `json:"series_count"`
	QualityProfiles []ProfilePreview `json:"quality_profiles"`
	RootFolders     []FolderPreview  `json:"root_folders"`
	Indexers        []IndexerPreview `json:"indexers"`
	DownloadClients []ClientPreview  `json:"download_clients"`
}

// ProfilePreview is a summary of a Sonarr quality profile.
type ProfilePreview struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// FolderPreview is a summary of a Sonarr root folder.
type FolderPreview struct {
	Path        string `json:"path"`
	FreeSpaceGB int    `json:"free_space_gb"`
}

// IndexerPreview is a summary of a Sonarr indexer.
type IndexerPreview struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// ClientPreview is a summary of a Sonarr download client.
type ClientPreview struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// ImportOptions controls which categories are imported.
type ImportOptions struct {
	QualityProfiles bool `json:"quality_profiles"`
	Libraries       bool `json:"libraries"`
	Indexers        bool `json:"indexers"`
	DownloadClients bool `json:"download_clients"`
	Series          bool `json:"series"`
}

// ImportResult reports how many records were created per category.
type ImportResult struct {
	QualityProfiles CategoryResult `json:"quality_profiles"`
	Libraries       CategoryResult `json:"libraries"`
	Indexers        CategoryResult `json:"indexers"`
	DownloadClients CategoryResult `json:"download_clients"`
	Series          CategoryResult `json:"series"`
	Errors          []string       `json:"errors"`
}

// CategoryResult holds import statistics for a single category.
type CategoryResult struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
}

// ── Service ──────────────────────────────────────────────────────────────────

// Service orchestrates the one-time import from a Sonarr instance.
type Service struct {
	shows       *show.Service
	qualities   *quality.Service
	libraries   *library.Service
	indexers    *indexer.Service
	downloaders *downloader.Service
}

// NewService creates an import Service wired to the given core services.
func NewService(
	shows *show.Service,
	qualities *quality.Service,
	libraries *library.Service,
	indexers *indexer.Service,
	downloaders *downloader.Service,
) *Service {
	return &Service{
		shows:       shows,
		qualities:   qualities,
		libraries:   libraries,
		indexers:    indexers,
		downloaders: downloaders,
	}
}

// Preview connects to a Sonarr instance and returns a summary of what would be
// imported, without making any changes to the Pilot database.
func (s *Service) Preview(ctx context.Context, sonarrURL, apiKey string) (*PreviewResult, error) {
	c := NewClient(sonarrURL, apiKey)

	status, err := c.GetStatus(ctx)
	if err != nil {
		return nil, err
	}

	profiles, err := c.GetQualityProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching quality profiles: %w", err)
	}

	folders, err := c.GetRootFolders(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching root folders: %w", err)
	}

	snrIndexers, err := c.GetIndexers(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching indexers: %w", err)
	}

	snrClients, err := c.GetDownloadClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching download clients: %w", err)
	}

	series, err := c.GetSeries(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching series: %w", err)
	}

	result := &PreviewResult{
		Version:         status.Version,
		SeriesCount:     len(series),
		QualityProfiles: []ProfilePreview{},
		RootFolders:     []FolderPreview{},
		Indexers:        []IndexerPreview{},
		DownloadClients: []ClientPreview{},
	}

	for _, p := range profiles {
		result.QualityProfiles = append(result.QualityProfiles, ProfilePreview{ID: p.ID, Name: p.Name})
	}
	for _, f := range folders {
		result.RootFolders = append(result.RootFolders, FolderPreview{
			Path:        f.Path,
			FreeSpaceGB: int(f.FreeSpace / (1024 * 1024 * 1024)),
		})
	}
	for _, idx := range snrIndexers {
		result.Indexers = append(result.Indexers, IndexerPreview{
			ID:   idx.ID,
			Name: idx.Name,
			Kind: mapIndexerKind(idx.ConfigContract),
		})
	}
	for _, cl := range snrClients {
		result.DownloadClients = append(result.DownloadClients, ClientPreview{
			ID:   cl.ID,
			Name: cl.Name,
			Kind: mapClientKind(cl.ConfigContract),
		})
	}

	return result, nil
}

// Execute imports data from Sonarr into Pilot according to opts.
func (s *Service) Execute(ctx context.Context, sonarrURL, apiKey string, opts ImportOptions) (*ImportResult, error) {
	c := NewClient(sonarrURL, apiKey)

	if _, err := c.GetStatus(ctx); err != nil {
		return nil, err
	}

	result := &ImportResult{Errors: []string{}}

	profileIDMap := map[int]string{}
	libraryPathMap := map[string]string{}
	var firstProfileID string

	// ── 1. Quality profiles ───────────────────────────────────────────────────
	if opts.QualityProfiles {
		profiles, err := c.GetQualityProfiles(ctx)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("fetch quality profiles: %v", err))
		} else {
			existingProfiles := map[string]string{}
			if existing, err := s.qualities.List(ctx); err == nil {
				for _, e := range existing {
					existingProfiles[e.Name] = e.ID
				}
			}
			for _, p := range profiles {
				if existingID, exists := existingProfiles[p.Name]; exists {
					profileIDMap[p.ID] = existingID
					if firstProfileID == "" {
						firstProfileID = existingID
					}
					result.QualityProfiles.Skipped++
					continue
				}
				req := mapProfile(p)
				created, err := s.qualities.Create(ctx, req)
				if err != nil {
					result.QualityProfiles.Failed++
					result.Errors = append(result.Errors, fmt.Sprintf("quality profile %q: %v", p.Name, err))
					continue
				}
				profileIDMap[p.ID] = created.ID
				if firstProfileID == "" {
					firstProfileID = created.ID
				}
				result.QualityProfiles.Imported++
			}
		}
	}

	// ── 2. Libraries (from root folders) ──────────────────────────────────────
	if opts.Libraries {
		folders, err := c.GetRootFolders(ctx)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("fetch root folders: %v", err))
		} else {
			for _, f := range folders {
				name := filepath.Base(strings.TrimRight(f.Path, "/\\"))
				if name == "" || name == "." {
					name = f.Path
				}
				req := library.CreateRequest{
					Name:                    name,
					RootPath:                f.Path,
					DefaultQualityProfileID: firstProfileID,
					MinFreeSpaceGB:          0,
					Tags:                    []string{},
				}
				created, err := s.libraries.Create(ctx, req)
				if err != nil {
					result.Libraries.Failed++
					result.Errors = append(result.Errors, fmt.Sprintf("library %q: %v", f.Path, err))
					continue
				}
				libraryPathMap[f.Path] = created.ID
				result.Libraries.Imported++
			}
		}
	}

	// ── 3. Indexers ───────────────────────────────────────────────────────────
	if opts.Indexers {
		snrIndexers, err := c.GetIndexers(ctx)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("fetch indexers: %v", err))
		} else {
			existingIndexers := map[string]struct{}{}
			if existing, err := s.indexers.List(ctx); err == nil {
				for _, e := range existing {
					existingIndexers[e.Name] = struct{}{}
				}
			}
			for _, idx := range snrIndexers {
				kind := mapIndexerKind(idx.ConfigContract)
				if kind == "" {
					result.Indexers.Skipped++
					continue
				}
				if _, exists := existingIndexers[idx.Name]; exists {
					result.Indexers.Skipped++
					continue
				}
				settings := map[string]string{
					"url":     fieldString(idx.Fields, "baseUrl"),
					"api_key": fieldString(idx.Fields, "apiKey"),
				}
				settingsJSON, _ := json.Marshal(settings)
				req := indexer.CreateRequest{
					Name:     idx.Name,
					Kind:     kind,
					Enabled:  idx.EnableRss,
					Priority: 25,
					Settings: settingsJSON,
				}
				if _, err := s.indexers.Create(ctx, req); err != nil {
					result.Indexers.Failed++
					result.Errors = append(result.Errors, fmt.Sprintf("indexer %q: %v", idx.Name, err))
					continue
				}
				result.Indexers.Imported++
			}
		}
	}

	// ── 4. Download clients ───────────────────────────────────────────────────
	if opts.DownloadClients {
		snrClients, err := c.GetDownloadClients(ctx)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("fetch download clients: %v", err))
		} else {
			existingClients := map[string]struct{}{}
			if existing, err := s.downloaders.List(ctx); err == nil {
				for _, e := range existing {
					existingClients[e.Name] = struct{}{}
				}
			}
			for _, cl := range snrClients {
				kind := mapClientKind(cl.ConfigContract)
				if kind == "" {
					result.DownloadClients.Skipped++
					continue
				}
				if _, exists := existingClients[cl.Name]; exists {
					result.DownloadClients.Skipped++
					continue
				}
				settingsJSON, err := buildClientSettings(kind, cl.Fields)
				if err != nil {
					result.DownloadClients.Failed++
					result.Errors = append(result.Errors, fmt.Sprintf("download client %q settings: %v", cl.Name, err))
					continue
				}
				req := downloader.CreateRequest{
					Name:     cl.Name,
					Kind:     kind,
					Enabled:  cl.Enable,
					Priority: 25,
					Settings: settingsJSON,
				}
				if _, err := s.downloaders.Create(ctx, req); err != nil {
					result.DownloadClients.Failed++
					result.Errors = append(result.Errors, fmt.Sprintf("download client %q: %v", cl.Name, err))
					continue
				}
				result.DownloadClients.Imported++
			}
		}
	}

	// ── 5. Series ─────────────────────────────────────────────────────────────
	if opts.Series {
		snrSeries, err := c.GetSeries(ctx)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("fetch series: %v", err))
		} else {
			for _, sr := range snrSeries {
				if sr.TmdbID == 0 {
					result.Series.Skipped++
					continue
				}
				profileID := profileIDMap[sr.QualityProfileID]
				if profileID == "" {
					profileID = firstProfileID
				}
				libID := libraryPathMap[sr.RootFolderPath]
				if libID == "" {
					for _, id := range libraryPathMap {
						libID = id
						break
					}
				}
				if libID == "" {
					result.Series.Skipped++
					result.Errors = append(result.Errors, fmt.Sprintf("series %q (tmdb:%d): no library available", sr.Title, sr.TmdbID))
					continue
				}
				seriesType := sr.SeriesType
				if seriesType == "" {
					seriesType = "standard"
				}
				req := show.AddRequest{
					TMDBID:           sr.TmdbID,
					LibraryID:        libID,
					QualityProfileID: profileID,
					Monitored:        sr.Monitored,
					MonitorType:      "all",
					SeriesType:       seriesType,
				}
				if _, err := s.shows.Add(ctx, req); err != nil {
					if errors.Is(err, show.ErrAlreadyExists) {
						result.Series.Skipped++
					} else {
						result.Series.Failed++
						result.Errors = append(result.Errors, fmt.Sprintf("series %q (tmdb:%d): %v", sr.Title, sr.TmdbID, err))
					}
					continue
				}
				result.Series.Imported++
			}
		}
	}

	return result, nil
}

// ── Mapping helpers ──────────────────────────────────────────────────────────

func mapIndexerKind(contract string) string {
	switch contract {
	case "NewznabSettings":
		return "newznab"
	case "TorznabSettings":
		return "torznab"
	default:
		return ""
	}
}

func mapClientKind(contract string) string {
	switch contract {
	case "QBittorrentSettings":
		return "qbittorrent"
	case "DelugeSettings":
		return "deluge"
	default:
		return ""
	}
}

func buildClientSettings(kind string, fields []sonarrField) (json.RawMessage, error) {
	host := fieldString(fields, "host")
	port := fieldInt(fields, "port")
	useSsl := fieldBool(fields, "useSsl")
	url := buildURL(host, port, useSsl, fieldString(fields, "urlBase"))

	var settings map[string]string
	switch kind {
	case "qbittorrent":
		settings = map[string]string{
			"url":      url,
			"username": fieldString(fields, "username"),
			"password": fieldString(fields, "password"),
			"category": fieldString(fields, "tvCategory"),
		}
	case "deluge":
		settings = map[string]string{
			"url":      url,
			"password": fieldString(fields, "password"),
			"label":    fieldString(fields, "tvCategory"),
		}
	default:
		return nil, fmt.Errorf("unsupported client kind %q", kind)
	}
	return json.Marshal(settings)
}

func cutoffName(items []sonarrProfileItem, cutoffID int) string {
	for _, item := range items {
		if len(item.Items) == 0 {
			if item.Quality.ID == cutoffID {
				return item.Quality.Name
			}
		} else {
			for _, child := range item.Items {
				if child.Quality.ID == cutoffID {
					return child.Quality.Name
				}
			}
		}
	}
	return ""
}

func mapProfile(p sonarrProfile) quality.CreateRequest {
	var qualities []plugin.Quality
	for _, item := range p.Items {
		for _, q := range item.qualities() {
			qualities = append(qualities, mapSonarrQuality(q.Name))
		}
	}
	cutoff := mapSonarrQuality(cutoffName(p.Items, p.Cutoff))
	return quality.CreateRequest{
		Name:           p.Name,
		Cutoff:         cutoff,
		Qualities:      qualities,
		UpgradeAllowed: p.UpgradeAllowed,
	}
}

func mapSonarrQuality(name string) plugin.Quality {
	type entry struct {
		resolution plugin.Resolution
		source     plugin.Source
	}

	table := map[string]entry{
		"SDTV":               {plugin.ResolutionSD, plugin.SourceHDTV},
		"DVD":                {plugin.ResolutionSD, plugin.SourceDVD},
		"HDTV-720p":          {plugin.Resolution720p, plugin.SourceHDTV},
		"HDTV-1080p":         {plugin.Resolution1080p, plugin.SourceHDTV},
		"HDTV-2160p":         {plugin.Resolution2160p, plugin.SourceHDTV},
		"WEBRip-480p":        {plugin.ResolutionSD, plugin.SourceWEBRip},
		"WEBRip-720p":        {plugin.Resolution720p, plugin.SourceWEBRip},
		"WEBRip-1080p":       {plugin.Resolution1080p, plugin.SourceWEBRip},
		"WEBRip-2160p":       {plugin.Resolution2160p, plugin.SourceWEBRip},
		"WEBDL-480p":         {plugin.ResolutionSD, plugin.SourceWEBDL},
		"WEBDL-720p":         {plugin.Resolution720p, plugin.SourceWEBDL},
		"WEBDL-1080p":        {plugin.Resolution1080p, plugin.SourceWEBDL},
		"WEBDL-2160p":        {plugin.Resolution2160p, plugin.SourceWEBDL},
		"Bluray-480p":        {plugin.ResolutionSD, plugin.SourceBluRay},
		"Bluray-720p":        {plugin.Resolution720p, plugin.SourceBluRay},
		"Bluray-1080p":       {plugin.Resolution1080p, plugin.SourceBluRay},
		"Bluray-2160p":       {plugin.Resolution2160p, plugin.SourceBluRay},
		"Bluray-720p Remux":  {plugin.Resolution720p, plugin.SourceRemux},
		"Bluray-1080p Remux": {plugin.Resolution1080p, plugin.SourceRemux},
		"Bluray-2160p Remux": {plugin.Resolution2160p, plugin.SourceRemux},
		"Raw-HD":             {plugin.Resolution1080p, plugin.SourceRemux},
	}

	e, ok := table[name]
	if !ok {
		return plugin.Quality{
			Resolution: plugin.ResolutionUnknown,
			Source:     plugin.SourceUnknown,
			Codec:      plugin.CodecUnknown,
			HDR:        plugin.HDRNone,
			Name:       name,
		}
	}
	return plugin.Quality{
		Resolution: e.resolution,
		Source:     e.source,
		Codec:      plugin.CodecUnknown,
		HDR:        plugin.HDRNone,
		Name:       name,
	}
}
