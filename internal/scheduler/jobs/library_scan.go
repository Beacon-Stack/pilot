package jobs

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/beacon-stack/pilot/internal/core/library"
	"github.com/beacon-stack/pilot/internal/core/mediacheck"
	"github.com/beacon-stack/pilot/internal/core/parser"
	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/internal/core/unmapped"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/scheduler"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ffprobeTimeout caps how long ffprobe is allowed to run per file. 15s is
// generous for a single video stream probe — anything longer almost
// certainly means a stalled NFS mount or a corrupt header, and we'd
// rather skip than block the whole scan.
const ffprobeTimeout = 15 * time.Second

// libraryScanInterval is how often the library scan job runs.
const libraryScanInterval = 6 * time.Hour

// videoExtensions is the set of file extensions the scanner considers video files.
var libraryScanVideoExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".ts":   true,
	".m2ts": true,
	".mov":  true,
	".wmv":  true,
}

// yearSuffixRe strips a trailing (YYYY) or .YYYY from folder names.
var yearSuffixRe = regexp.MustCompile(`[\s.(]+(?:19|20)\d{2}[\s.)]*$`)

// releaseTagsRe strips common release tags from folder names.
var releaseTagsRe = regexp.MustCompile(`(?i)[\s.\[(-]*(720p|1080p|2160p|4k|bluray|brrip|webrip|web-dl|hdtv|dvdrip|x264|x265|h\.?264|h\.?265|aac|dts|atmos|proper|repack|complete|season|s\d{1,2}).*$`)

// LibraryScan returns a Job that walks all library root directories, records
// folders no series claims for review (they are not added automatically; see
// internal/core/unmapped), then matches video files to known series/episodes
// and creates episode_file records.
func LibraryScan(libSvc *library.Service, showSvc *show.Service, um *unmapped.Service, q db.Querier, logger *slog.Logger) scheduler.Job {
	return scheduler.Job{
		Name:     "library_scan",
		Interval: libraryScanInterval,
		Fn: func(ctx context.Context) {
			logger.Info("task started", "task", "library_scan")
			start := time.Now()

			if err := runLibraryScan(ctx, libSvc, showSvc, um, q, logger); err != nil {
				logger.Warn("task failed",
					"task", "library_scan",
					"error", err,
					"duration_ms", time.Since(start).Milliseconds(),
				)
				return
			}

			logger.Info("task finished",
				"task", "library_scan",
				"duration_ms", time.Since(start).Milliseconds(),
			)
		},
	}
}

// runLibraryScan performs the actual scan work.
func runLibraryScan(ctx context.Context, libSvc *library.Service, showSvc *show.Service, um *unmapped.Service, q db.Querier, logger *slog.Logger) error {
	libs, err := libSvc.List(ctx)
	if err != nil {
		return err
	}

	// Phase 1: record folders no series claims, for the user to review.
	// Nothing is added automatically: a wrong TMDB hit used to add the
	// wrong show and attach files to it.
	if um != nil {
		for _, lib := range libs {
			if err := um.Discover(ctx, lib.ID, lib.RootPath); err != nil {
				logger.Warn("library scan: recording unmapped folders failed", "library_id", lib.ID, "error", err)
			}
		}
	}

	// Phase 2: Match video files to known series/episodes.
	knownPaths, err := q.ListAllEpisodeFilePaths(ctx)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(knownPaths))
	for _, p := range knownPaths {
		known[p] = true
	}

	for _, lib := range libs {
		if err := scanLibraryFiles(ctx, lib, showSvc, q, known, logger); err != nil {
			logger.Warn("library scan error",
				"library_id", lib.ID,
				"library_name", lib.Name,
				"error", err,
			)
		}
	}
	return nil
}

// cleanFolderName normalizes a directory name into a human-readable show title
// suitable for a TMDB search. It strips release tags, year suffixes,
// and replaces separators with spaces.
func cleanFolderName(name string) string {
	// Replace common separators with spaces.
	name = strings.NewReplacer(
		".", " ",
		"_", " ",
		"-", " ",
	).Replace(name)

	// Strip release tags (720p, x264, BRRip, etc.).
	name = releaseTagsRe.ReplaceAllString(name, "")

	// Strip trailing year like (2019) or .2019
	name = yearSuffixRe.ReplaceAllString(name, "")

	// Strip content inside square brackets (release groups like [TGx]).
	name = regexp.MustCompile(`\[.*?\]`).ReplaceAllString(name, "")

	// Collapse whitespace and trim.
	for strings.Contains(name, "  ") {
		name = strings.ReplaceAll(name, "  ", " ")
	}
	name = strings.TrimSpace(name)

	// Skip names that are too short or look like just episode fragments.
	if len(name) < 2 {
		return ""
	}
	return name
}

// scanLibraryFiles walks a single library root directory and imports new files.
func scanLibraryFiles(
	ctx context.Context,
	lib library.Library,
	showSvc *show.Service,
	q db.Querier,
	known map[string]bool,
	logger *slog.Logger,
) error {
	// Load all series in this library so we can match by title.
	result, err := showSvc.List(ctx, show.ListRequest{LibraryID: lib.ID, Page: 1, PerPage: 10000})
	if err != nil {
		return err
	}

	// Build a lowercase-title → Series map for fuzzy matching, and a
	// folder → Series map for series pointed at a folder (confirmed
	// unmapped folders, whose names may not match the show's title).
	seriesByTitle := make(map[string]show.Series, len(result.Series))
	seriesByFolder := make(map[string]show.Series, len(result.Series))
	for _, s := range result.Series {
		seriesByTitle[strings.ToLower(s.Title)] = s
		if s.Path != "" {
			seriesByFolder[filepath.Clean(s.Path)] = s
		}
	}

	return filepath.WalkDir(lib.RootPath, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			logger.Warn("library scan: walk error", "path", path, "error", werr)
			return nil // keep walking
		}
		if d.IsDir() {
			if path != lib.RootPath && skipScanDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !libraryScanVideoExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		if known[path] {
			return nil // already tracked
		}

		if err := importScannedFile(ctx, path, lib, seriesByTitle, seriesByFolder, showSvc, q, logger); err != nil {
			logger.Warn("library scan: import failed", "path", path, "error", err)
		}
		return nil
	})
}

// importScannedFile tries to match a discovered file to an existing
// series/episode and creates an episode_file record if successful.
func importScannedFile(
	ctx context.Context,
	path string,
	lib library.Library,
	seriesByTitle map[string]show.Series,
	seriesByFolder map[string]show.Series,
	showSvc *show.Service,
	q db.Querier,
	logger *slog.Logger,
) error {
	parsed := parser.Parse(filepath.Base(path))
	info := parsed.EpisodeInfo

	if len(info.Episodes) == 0 {
		logger.Debug("library scan: no episode info in filename, skipping", "path", path)
		return nil
	}

	// A series pointed at this file's top-level folder claims it first.
	var series show.Series
	ok := false
	if rel, err := filepath.Rel(lib.RootPath, path); err == nil {
		topDir := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		series, ok = seriesByFolder[filepath.Join(lib.RootPath, topDir)]
	}
	// Then the parsed show title.
	if !ok {
		series, ok = seriesByTitle[strings.ToLower(parsed.ShowTitle)]
	}
	if !ok {
		// Fallback: try matching against the top-level folder name within the library root.
		rel, err := filepath.Rel(lib.RootPath, path)
		if err == nil {
			topDir := strings.SplitN(rel, string(filepath.Separator), 2)[0]
			folderTitle := cleanFolderName(topDir)
			series, ok = seriesByTitle[strings.ToLower(folderTitle)]
		}
	}
	if !ok {
		logger.Debug("library scan: no matching series for title",
			"title", parsed.ShowTitle,
			"path", path,
		)
		return nil
	}

	// Load all episodes for the series.
	episodes, err := q.ListEpisodesBySeriesID(ctx, series.ID)
	if err != nil {
		return err
	}

	type epKey struct{ season, episode int }
	epIndex := make(map[epKey]db.Episode, len(episodes))
	for _, ep := range episodes {
		epIndex[epKey{int(ep.SeasonNumber), int(ep.EpisodeNumber)}] = ep
	}

	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	// An all-zero file (an incomplete download) is not an episode file:
	// recording it would hide the episode from searches forever.
	if err := mediacheck.Check(path); err != nil {
		logger.Warn("library scan: skipping file with no data", "path", path, "error", err)
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, epNum := range info.Episodes {
		ep, ok := epIndex[epKey{info.Season, epNum}]
		if !ok {
			logger.Debug("library scan: no matching episode",
				"series", series.Title,
				"season", info.Season,
				"episode", epNum,
				"path", path,
			)
			continue
		}
		// An episode that already has a file keeps it: a second copy found
		// on disk (a duplicate, or a mislabelled release with the same
		// SxxEyy) is not recorded against it.
		if existing, err := q.ListEpisodeFilesByEpisodeID(ctx, ep.ID); err == nil && len(existing) > 0 {
			logger.Debug("library scan: episode already has a file, not recording another",
				"series", series.Title, "season", info.Season, "episode", epNum, "path", path)
			continue
		}

		// Probe the file for resolution/codec/HDR. Source (web-dl vs bluray
		// etc.) is unrecoverable from a clean post-import filename and is
		// not in the bytes themselves; leave it empty so the stats UI
		// renders it as "unknown" rather than guessing.
		quality := probeQuality(ctx, path, logger)
		qualityJSON, _ := json.Marshal(quality)

		if _, err := q.CreateEpisodeFile(ctx, db.CreateEpisodeFileParams{
			ID:          uuid.New().String(),
			EpisodeID:   ep.ID,
			SeriesID:    series.ID,
			Path:        path,
			SizeBytes:   fi.Size(),
			QualityJson: string(qualityJSON),
			ImportedAt:  now,
			IndexedAt:   now,
		}); err != nil {
			logger.Warn("library scan: failed to create episode_file record",
				"path", path,
				"episode_id", ep.ID,
				"error", err,
			)
			continue
		}

		// Mark episode as having a file.
		if _, err := q.UpdateEpisode(ctx, db.UpdateEpisodeParams{
			ID:             ep.ID,
			Title:          ep.Title,
			Overview:       ep.Overview,
			AirDate:        ep.AirDate,
			HasFile:        true,
			StillPath:      ep.StillPath,
			RuntimeMinutes: ep.RuntimeMinutes,
		}); err != nil {
			logger.Warn("library scan: failed to mark episode has_file",
				"episode_id", ep.ID,
				"error", err,
			)
		}

		logger.Info("library scan: indexed episode file",
			"series", series.Title,
			"season", info.Season,
			"episode", epNum,
			"path", path,
		)
	}

	return nil
}

// probeQuality runs ffprobe on a file and maps the result onto a
// plugin.Quality. Returns the zero value (which the stats UI renders as
// "unknown") on any failure — the scan must not stall on a single bad
// file, and a probe failure is informational, not fatal.
//
// Only Resolution, Codec, and HDR are populated. Source (web-dl vs
// bluray etc.) is not recoverable from the bytes themselves, and from a
// clean post-import filename it's also gone, so it stays empty.
// skipScanDir reports whether the library scan should not descend into a
// folder: "_unsorted" (files set aside by hand because they could not be
// identified), NAS system folders, and hidden folders.
func skipScanDir(name string) bool {
	switch strings.ToLower(name) {
	case "_unsorted", "@eadir", "#recycle", "#snapshot", "$recycle.bin":
		return true
	}
	return strings.HasPrefix(name, ".")
}

func probeQuality(ctx context.Context, path string, logger *slog.Logger) plugin.Quality {
	probeCtx, cancel := context.WithTimeout(ctx, ffprobeTimeout)
	defer cancel()

	//nolint:gosec // path is a trusted internal value (walked from a configured library root)
	cmd := exec.CommandContext(probeCtx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		logger.Debug("library scan: ffprobe failed (leaving quality unknown)",
			"path", path, "error", err)
		return plugin.Quality{}
	}

	var raw struct {
		Streams []struct {
			CodecType      string `json:"codec_type"`
			CodecName      string `json:"codec_name"`
			Height         int    `json:"height"`
			ColorTransfer  string `json:"color_transfer"`
			ColorPrimaries string `json:"color_primaries"`
			SideDataList   []struct {
				SideDataType string `json:"side_data_type"`
			} `json:"side_data_list"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		logger.Debug("library scan: ffprobe parse failed", "path", path, "error", err)
		return plugin.Quality{}
	}

	var q plugin.Quality
	for _, s := range raw.Streams {
		if s.CodecType != "video" {
			continue
		}
		q.Resolution = resolutionFromHeight(s.Height)
		q.Codec = codecFromName(s.CodecName)
		q.HDR = hdrFromStream(s.ColorTransfer, s.SideDataList)
		break
	}
	return q
}

func resolutionFromHeight(h int) plugin.Resolution {
	switch {
	case h >= 2160:
		return plugin.Resolution2160p
	case h >= 1080:
		return plugin.Resolution1080p
	case h >= 720:
		return plugin.Resolution720p
	case h >= 576:
		return plugin.Resolution576p
	case h >= 480:
		return plugin.Resolution480p
	case h > 0:
		return plugin.ResolutionSD
	default:
		return ""
	}
}

func codecFromName(name string) plugin.Codec {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "hevc", "h265":
		return plugin.CodecX265
	case "h264", "avc":
		return plugin.CodecX264
	case "av1", "av01":
		return plugin.CodecAV1
	case "mpeg4":
		return plugin.CodecXVID
	default:
		return ""
	}
}

func hdrFromStream(colorTransfer string, sideData []struct {
	SideDataType string `json:"side_data_type"`
}) plugin.HDRFormat {
	for _, sd := range sideData {
		t := strings.ToLower(sd.SideDataType)
		if strings.Contains(t, "dovi") || strings.Contains(t, "dolby") {
			return plugin.HDRDolbyVision
		}
	}
	switch colorTransfer {
	case "smpte2084":
		return plugin.HDRHDR10
	case "arib-std-b67":
		return plugin.HDRHLG
	default:
		return plugin.HDRNone
	}
}
