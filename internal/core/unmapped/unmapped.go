// Package unmapped tracks folders in a library root that no series in
// Pilot claims, so the user can review them (Sonarr's library import).
//
// The library scan used to auto-add a series for each such folder from
// its best TMDB search hit, without asking; a wrong hit added the wrong
// show and attached files to it. Now Discover records the folder with
// that best hit as a guess, and nothing is added until the user confirms
// it (Add, possibly with a different show) or ignores the folder.
package unmapped

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/beacon-stack/pilot/internal/core/show"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/metadata/tmdbtv"
)

// ErrNotFound is returned for an unknown folder id.
var ErrNotFound = errors.New("unmapped folder not found")

// Shows is the part of show.Service this package uses.
type Shows interface {
	Lookup(ctx context.Context, req show.LookupRequest) ([]tmdbtv.SearchResult, error)
	Add(ctx context.Context, req show.AddRequest) (show.Series, error)
}

// Folder is one unmapped folder.
type Folder struct {
	ID              string
	LibraryID       string
	Path            string
	Name            string
	GuessTMDBID     int
	GuessTitle      string
	GuessYear       int
	GuessPosterPath string
	Ignored         bool
	FirstSeenAt     string
}

// Service records and resolves unmapped folders.
type Service struct {
	q     db.Querier
	shows Shows
	now   func() time.Time
}

// New creates a Service.
func New(q db.Querier, shows Shows) *Service {
	return &Service{q: q, shows: shows, now: time.Now}
}

var (
	reYear     = regexp.MustCompile(`[\s.(\[]((?:19|20)\d{2})[\s.)\]]*$`)
	reTags     = regexp.MustCompile(`(?i)[\s.\[(-]*(720p|1080p|2160p|4k|bluray|brrip|webrip|web-dl|hdtv|dvdrip|x264|x265|h\.?264|h\.?265|aac|dts|atmos|proper|repack|complete|season|s\d{1,2}).*$`)
	reBrackets = regexp.MustCompile(`\[.*?\]`)
	reSpaces   = regexp.MustCompile(`\s+`)
)

// FolderTitle turns a folder name into a show title and year for a TMDB
// search: "Show.Name.(2019).1080p" → ("Show Name", 2019).
func FolderTitle(name string) (string, int) {
	s := reBrackets.ReplaceAllString(name, " ")
	s = strings.NewReplacer(".", " ", "_", " ").Replace(s)
	s = reTags.ReplaceAllString(s, "")
	year := 0
	if m := reYear.FindStringSubmatchIndex(s); m != nil {
		year, _ = strconv.Atoi(s[m[2]:m[3]])
		s = s[:m[0]]
	}
	s = strings.TrimSpace(reSpaces.ReplaceAllString(s, " "))
	s = strings.Trim(s, " -")
	if len(s) < 2 {
		return "", 0
	}
	return s, year
}

// Discover reads the top-level folders of a library root and records
// each one no series claims, with a TMDB guess. Folders already recorded
// are not looked up again. Records whose folder is gone, or that a
// series now claims, are removed. It adds no series.
func (s *Service) Discover(ctx context.Context, libraryID, root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("reading library root: %w", err)
	}
	series, err := s.q.ListSeries(ctx, db.ListSeriesParams{Limit: 100000, Offset: 0})
	if err != nil {
		return err
	}
	claimedPath := map[string]bool{}
	claimedTitle := map[string]bool{}
	for _, sr := range series {
		if sr.Path != nil && *sr.Path != "" {
			claimedPath[filepath.Clean(*sr.Path)] = true
		}
		if sr.LibraryID == libraryID {
			claimedTitle[strings.ToLower(sr.Title)] = true
		}
	}
	claimed := func(path, name string) bool {
		if claimedPath[filepath.Clean(path)] {
			return true
		}
		title, _ := FolderTitle(name)
		return title == "" || claimedTitle[strings.ToLower(title)]
	}

	existing, err := s.q.ListUnmappedFoldersByLibrary(ctx, libraryID)
	if err != nil {
		return err
	}
	recorded := map[string]bool{}
	present := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			present[filepath.Join(root, e.Name())] = true
		}
	}
	for _, f := range existing {
		if !present[f.Path] || claimed(f.Path, f.Name) {
			if err := s.q.DeleteUnmappedFolder(ctx, f.ID); err != nil {
				return err
			}
			continue
		}
		recorded[f.Path] = true
	}

	now := s.now().UTC().Format(time.RFC3339)
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(root, e.Name())
		if recorded[path] || claimed(path, e.Name()) {
			continue
		}
		title, year := FolderTitle(e.Name())
		p := db.InsertUnmappedFolderParams{
			ID: uuid.New().String(), LibraryID: libraryID, Path: path, Name: e.Name(), FirstSeenAt: now,
		}
		if results, err := s.shows.Lookup(ctx, show.LookupRequest{Query: title, Year: year}); err == nil && len(results) > 0 {
			best := results[0]
			p.GuessTmdbID, p.GuessTitle, p.GuessYear, p.GuessPosterPath = int64(best.ID), best.Title, int64(best.Year), best.PosterPath
		}
		if err := s.q.InsertUnmappedFolder(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// List returns the unmapped folders, without ignored ones unless asked.
func (s *Service) List(ctx context.Context, includeIgnored bool) ([]Folder, error) {
	rows, err := s.q.ListUnmappedFolders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Folder, 0, len(rows))
	for _, r := range rows {
		if r.Ignored && !includeIgnored {
			continue
		}
		out = append(out, fromRow(r))
	}
	return out, nil
}

// AddRequest confirms a folder as a series.
type AddRequest struct {
	TMDBID           int    // 0 = the guess
	QualityProfileID string // "" = the library's default
	MonitorType      string // "" = "existing"
}

// Add adds the series for a folder (the guess, or another TMDB id),
// points the series at the folder so its files are found there, and
// removes the record.
func (s *Service) Add(ctx context.Context, id string, req AddRequest) (show.Series, error) {
	f, err := s.q.GetUnmappedFolder(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return show.Series{}, ErrNotFound
	}
	if err != nil {
		return show.Series{}, err
	}
	tmdbID := req.TMDBID
	if tmdbID == 0 {
		tmdbID = int(f.GuessTmdbID)
	}
	if tmdbID == 0 {
		return show.Series{}, errors.New("no show chosen and no guess for this folder")
	}
	lib, err := s.q.GetLibrary(ctx, f.LibraryID)
	if err != nil {
		return show.Series{}, err
	}
	profile := req.QualityProfileID
	if profile == "" {
		profile = lib.DefaultQualityProfileID
	}
	monitor := req.MonitorType
	if monitor == "" {
		monitor = "existing"
	}
	added, err := s.shows.Add(ctx, show.AddRequest{
		TMDBID: tmdbID, LibraryID: f.LibraryID, QualityProfileID: profile,
		Monitored: true, MonitorType: monitor, SeriesType: "standard",
	})
	if err != nil {
		return show.Series{}, err
	}
	// Point the series at this folder: its name may not match the show's
	// title, and the library scan finds a series' files by its folder.
	row, err := s.q.GetSeries(ctx, added.ID)
	if err == nil {
		path := f.Path
		_, _ = s.q.UpdateSeries(ctx, db.UpdateSeriesParams{
			ID: row.ID, Title: row.Title, Monitored: row.Monitored, LibraryID: row.LibraryID,
			QualityProfileID: row.QualityProfileID, SeriesType: row.SeriesType, Path: &path,
			UpdatedAt: s.now().UTC().Format(time.RFC3339),
		})
		added.Path = path
	}
	if err := s.q.DeleteUnmappedFolder(ctx, f.ID); err != nil {
		return added, err
	}
	return added, nil
}

// SetIgnored ignores a folder (or stops ignoring it).
func (s *Service) SetIgnored(ctx context.Context, id string, ignored bool) error {
	if _, err := s.q.GetUnmappedFolder(ctx, id); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	return s.q.SetUnmappedFolderIgnored(ctx, db.SetUnmappedFolderIgnoredParams{Ignored: ignored, ID: id})
}

func fromRow(r db.UnmappedFolder) Folder {
	return Folder{
		ID: r.ID, LibraryID: r.LibraryID, Path: r.Path, Name: r.Name,
		GuessTMDBID: int(r.GuessTmdbID), GuessTitle: r.GuessTitle, GuessYear: int(r.GuessYear),
		GuessPosterPath: r.GuessPosterPath, Ignored: r.Ignored, FirstSeenAt: r.FirstSeenAt,
	}
}
