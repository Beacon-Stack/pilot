package importer

// Manual import (Sonarr's "Manual Import"): scan a folder for video files,
// propose a series, episodes and quality for each, let the user review,
// then import the chosen files through AttachFile, so renaming, free
// space, the recycle bin and replaced-file handling all apply exactly as
// for downloads.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/beacon-stack/pilot/internal/core/parser"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
	releaseparser "github.com/beacon-stack/pilot/internal/parser"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// Rejection reasons on a manual import candidate. A rejected candidate can
// still be imported if the user chooses to.
const (
	RejectSample          = "sample"
	RejectUnknownSeries   = "unknown_series"
	RejectUnknownEpisode  = "unknown_episode"
	RejectAlreadyImported = "already_imported"
	RejectNotAnUpgrade    = "existing_file_is_equal_or_better"
)

// MaxManualScanFiles caps one scan.
const MaxManualScanFiles = 2000

// ErrInvalidPath is returned for a scan or import path that is not an
// absolute path to an existing folder or video file.
var ErrInvalidPath = errors.New("path must be an absolute path to an existing folder or video file")

// CandidateEpisode is an episode a candidate file would be attached to.
type CandidateEpisode struct {
	ID            string
	SeasonNumber  int
	EpisodeNumber int
	Title         string
}

// Candidate is one video file found by a manual import scan.
type Candidate struct {
	Path        string
	Size        int64
	SeriesID    string // "" when no series matched
	SeriesTitle string
	Season      int
	Episodes    []CandidateEpisode
	Quality     plugin.Quality
	Languages   []string
	Rejections  []string
}

// ManualItem is one file the user chose to import.
type ManualItem struct {
	Path       string
	SeriesID   string
	EpisodeIDs []string
	Quality    plugin.Quality
}

// ManualResult is the outcome for one ManualItem.
type ManualResult struct {
	Path  string
	Error string // "" on success
}

type seriesRef struct {
	row    db.Series
	titles []string
}

func (s *Service) loadSeriesRefs(ctx context.Context) ([]seriesRef, error) {
	rows, err := s.q.ListSeries(ctx, db.ListSeriesParams{Limit: 100000, Offset: 0})
	if err != nil {
		return nil, err
	}
	refs := make([]seriesRef, 0, len(rows))
	for _, r := range rows {
		titles := []string{r.Title}
		var alt []string
		if r.AlternateTitles != "" && json.Unmarshal([]byte(r.AlternateTitles), &alt) == nil {
			titles = append(titles, alt...)
		}
		refs = append(refs, seriesRef{row: r, titles: titles})
	}
	return refs, nil
}

// reEpisodeMarker finds the SxxEyy token, to take the show name from the
// text before it ("Show - S01E01 - Title" names the show "Show").
var reEpisodeMarker = regexp.MustCompile(`(?i)[\s._-]*S\d{1,2}E\d{1,3}`)

// matchSeries finds the series named by the file name, else by the
// folder it is in (download folders are named after the release), else
// by the folder two levels up (a library's "Series/Season 01/file").
func matchSeries(refs []seriesRef, path string) (db.Series, bool) {
	base := filepath.Base(path)
	var titles []string
	if loc := reEpisodeMarker.FindStringIndex(base); loc != nil && loc[0] > 0 {
		titles = append(titles, base[:loc[0]])
	}
	for _, name := range []string{base, filepath.Base(filepath.Dir(path)), filepath.Base(filepath.Dir(filepath.Dir(path)))} {
		titles = append(titles, releaseparser.Parse(name).Title)
	}
	for _, title := range titles {
		if strings.TrimSpace(title) == "" {
			continue
		}
		for _, r := range refs {
			if parser.TitleMatchesAny(r.titles, title) {
				return r.row, true
			}
		}
	}
	return db.Series{}, false
}

// episodeInfo parses season and episodes from the file name, falling
// back to the folder name for the season.
func episodeInfo(path string) (int, []int) {
	info := releaseparser.Parse(filepath.Base(path)).EpisodeInfo
	if len(info.Episodes) > 0 {
		return info.Season, info.Episodes
	}
	return 0, nil
}

func qualityFromName(name string) plugin.Quality {
	p := releaseparser.Parse(name)
	q := p.Quality()
	q.Name = releaseparser.QualityName(q)
	q.Revision = p.Revision.Version
	return q
}

func absExisting(path string) (string, os.FileInfo, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", nil, ErrInvalidPath
	}
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil {
		return "", nil, ErrInvalidPath
	}
	return clean, info, nil
}

// ScanManual lists the video files under path (a folder, or one file)
// with a proposed series, episodes and quality for each. It changes
// nothing. With seriesID set, every file is proposed for that series.
func (s *Service) ScanManual(ctx context.Context, path, seriesID string) ([]Candidate, error) {
	root, info, err := absExisting(path)
	if err != nil {
		return nil, err
	}
	var files []string
	if info.IsDir() {
		walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil //nolint:nilerr // an unreadable entry is skipped, not fatal to the scan
			}
			if d.IsDir() {
				return nil
			}
			if videoExtensions[strings.ToLower(filepath.Ext(p))] {
				files = append(files, p)
				if len(files) >= MaxManualScanFiles {
					return fs.SkipAll
				}
			}
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	} else if videoExtensions[strings.ToLower(filepath.Ext(root))] {
		files = []string{root}
	}
	sort.Strings(files)

	refs, err := s.loadSeriesRefs(ctx)
	if err != nil {
		return nil, err
	}
	var forced *db.Series
	if seriesID != "" {
		sr, err := s.q.GetSeries(ctx, seriesID)
		if err != nil {
			return nil, fmt.Errorf("series %q not found", seriesID)
		}
		forced = &sr
	}

	episodesBySeries := map[string][]db.Episode{}
	out := make([]Candidate, 0, len(files))
	for _, f := range files {
		c := Candidate{Path: f, Quality: qualityFromName(filepath.Base(f))}
		if st, err := os.Stat(f); err == nil {
			c.Size = st.Size()
		}
		parsed := releaseparser.Parse(filepath.Base(f))
		for _, l := range parsed.Languages {
			c.Languages = append(c.Languages, string(l))
		}
		if parsed.IsSample {
			c.Rejections = append(c.Rejections, RejectSample)
		}
		if _, err := s.q.GetEpisodeFileByPath(ctx, f); err == nil {
			c.Rejections = append(c.Rejections, RejectAlreadyImported)
		}

		var sr db.Series
		ok := false
		if forced != nil {
			sr, ok = *forced, true
		} else {
			sr, ok = matchSeries(refs, f)
		}
		if !ok {
			c.Rejections = append(c.Rejections, RejectUnknownSeries)
			out = append(out, c)
			continue
		}
		c.SeriesID, c.SeriesTitle = sr.ID, sr.Title

		eps, loaded := episodesBySeries[sr.ID]
		if !loaded {
			if eps, err = s.q.ListEpisodesBySeriesID(ctx, sr.ID); err != nil {
				return nil, err
			}
			episodesBySeries[sr.ID] = eps
		}
		season, nums := episodeInfo(f)
		c.Season = season
		for _, n := range nums {
			for _, ep := range eps {
				if int(ep.SeasonNumber) == season && int(ep.EpisodeNumber) == n {
					c.Episodes = append(c.Episodes, CandidateEpisode{ID: ep.ID, SeasonNumber: season, EpisodeNumber: n, Title: ep.Title})
				}
			}
		}
		if len(c.Episodes) == 0 || len(c.Episodes) != len(nums) {
			c.Rejections = append(c.Rejections, RejectUnknownEpisode)
		}
		if s.notAnUpgrade(ctx, c) {
			c.Rejections = append(c.Rejections, RejectNotAnUpgrade)
		}
		out = append(out, c)
	}
	return out, nil
}

// rank orders qualities the way the decision engine does: resolution and
// source (Score()/10, codec ignored), then revision.
func rank(q plugin.Quality) int { return (q.Score()/10)*100 + q.EffectiveRevision() }

// notAnUpgrade reports whether every episode already has a file at least
// as good as the candidate (by quality and revision).
func (s *Service) notAnUpgrade(ctx context.Context, c Candidate) bool {
	if len(c.Episodes) == 0 {
		return false
	}
	for _, ep := range c.Episodes {
		files, err := s.q.ListEpisodeFilesByEpisodeID(ctx, ep.ID)
		if err != nil || len(files) == 0 {
			return false
		}
		best := -1
		for _, f := range files {
			if f.Path == c.Path {
				continue
			}
			var q plugin.Quality
			if json.Unmarshal([]byte(f.QualityJson), &q) == nil {
				if sc := rank(q); sc > best {
					best = sc
				}
			}
		}
		if best < 0 || rank(c.Quality) > best {
			return false
		}
	}
	return true
}

// ImportManual imports each item: every listed episode gets the file via
// AttachFile. With move, the source file is removed once all its
// episodes are imported (unless it is the library file itself). Results
// are per item; one failure does not stop the rest.
func (s *Service) ImportManual(ctx context.Context, items []ManualItem, move bool) []ManualResult {
	out := make([]ManualResult, 0, len(items))
	for _, it := range items {
		err := s.importManualItem(ctx, it, move)
		r := ManualResult{Path: it.Path}
		if err != nil {
			r.Error = err.Error()
			s.logger.Warn("manual import failed", "path", it.Path, "error", err)
		}
		out = append(out, r)
	}
	return out
}

func (s *Service) importManualItem(ctx context.Context, it ManualItem, move bool) error {
	path, info, err := absExisting(it.Path)
	if err != nil {
		return err
	}
	if info.IsDir() || !videoExtensions[strings.ToLower(filepath.Ext(path))] {
		return fmt.Errorf("%s is not a video file", path)
	}
	if len(it.EpisodeIDs) == 0 {
		return errors.New("choose at least one episode")
	}
	series, err := s.q.GetSeries(ctx, it.SeriesID)
	if err != nil {
		return fmt.Errorf("series %q not found", it.SeriesID)
	}
	for _, id := range it.EpisodeIDs {
		ep, err := s.q.GetEpisode(ctx, id)
		if err != nil || ep.SeriesID != series.ID {
			return fmt.Errorf("episode %q is not an episode of %s", id, series.Title)
		}
	}
	q := it.Quality
	if q.Resolution == "" && q.Source == "" {
		q = qualityFromName(filepath.Base(path))
	}
	if q.Name == "" {
		q.Name = releaseparser.QualityName(q)
	}

	recorded := map[string]bool{}
	for _, id := range it.EpisodeIDs {
		if err := s.AttachFile(ctx, id, series.ID, path, info.Size(), q); err != nil {
			return err
		}
		files, _ := s.q.ListEpisodeFilesByEpisodeID(ctx, id)
		for _, f := range files {
			recorded[f.Path] = true
		}
	}
	if move && !recorded[path] {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("imported, but the source could not be removed: %w", err)
		}
	}
	if s.bus != nil {
		s.bus.Publish(ctx, events.Event{
			Type:   events.TypeImportComplete,
			ShowID: series.ID,
			Data: map[string]any{
				"content_path": path,
				"series_title": series.Title,
				"quality":      qualityLabel(q),
				"manual":       true,
			},
		})
	}
	return nil
}
