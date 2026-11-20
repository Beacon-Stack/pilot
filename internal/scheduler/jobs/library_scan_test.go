package jobs

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/beacon-stack/pilot/internal/config"
	"github.com/beacon-stack/pilot/internal/core/library"
	"github.com/beacon-stack/pilot/internal/core/show"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// A series pointed at a folder (a confirmed unmapped folder) gets the
// files in it even when neither the folder nor the file names match the
// series title.
func TestScanLibraryFiles_MatchesSeriesByFolder(t *testing.T) {
	scanOneFile(t, []byte("video"), 1)
}

// A preallocated, never-written download (all zeros) must not become an
// episode file: it would mark the episode as downloaded forever. This is
// how 16 empty Patriot S02 files and 3 Ted Lasso files got recorded.
func TestScanLibraryFiles_SkipsAllZeroFile(t *testing.T) {
	scanOneFile(t, make([]byte, 1<<20), 0)
}

// Files set aside in _unsorted (unidentified or mislabelled releases) are
// never attached to an episode.
func TestScanLibraryFiles_SkipsUnsortedFolder(t *testing.T) {
	scanFileAt(t, filepath.Join("_unsorted", "odd.name.S01E01.mkv"), []byte("video"), 0, false)
}

// A second file for an episode that already has one is not recorded: a
// mislabelled release with the same SxxEyy must not become its file.
func TestScanLibraryFiles_EpisodeWithFileGetsNoSecondFile(t *testing.T) {
	scanFileAt(t, filepath.Join("Season 1", "odd.name.S01E01.mkv"), []byte("video"), 1, true)
}

func scanOneFile(t *testing.T, content []byte, wantFiles int) {
	t.Helper()
	scanFileAt(t, filepath.Join("Season 1", "odd.name.S01E01.mkv"), content, wantFiles, false)
}

func scanFileAt(t *testing.T, rel string, content []byte, wantFiles int, preexisting bool) {
	t.Helper()
	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pilot.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	folder := filepath.Join(root, "Some Odd Folder")
	file := filepath.Join(folder, rel)
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	_ = os.WriteFile(file, content, 0o644)

	now := time.Now().UTC().Format(time.RFC3339)
	for _, q := range []string{
		`INSERT INTO quality_profiles (id, name, created_at, updated_at) VALUES ('qp', 'Any', '` + now + `', '` + now + `')`,
		`INSERT INTO libraries (id, name, root_path, default_quality_profile_id, created_at, updated_at) VALUES ('lib', 'TV', '` + root + `', 'qp', '` + now + `', '` + now + `')`,
		`INSERT INTO series (id, tmdb_id, title, sort_title, year, library_id, quality_profile_id, path, added_at, updated_at) VALUES ('s', 1, 'Real Title', 'real title', 2020, 'lib', 'qp', '` + folder + `', '` + now + `', '` + now + `')`,
		`INSERT INTO seasons (id, series_id, season_number) VALUES ('s1', 's', 1)`,
		`INSERT INTO episodes (id, series_id, season_id, season_number, episode_number, title) VALUES ('e1', 's', 's1', 1, 1, 'Pilot')`,
	} {
		if _, err := d.SQL.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if preexisting {
		if _, err := d.SQL.Exec(`INSERT INTO episode_files (id, episode_id, series_id, path, size_bytes, quality_json, imported_at, indexed_at) VALUES ('ef0', 'e1', 's', '/elsewhere/existing.mkv', 1, '{}', '` + now + `', '` + now + `')`); err != nil {
			t.Fatal(err)
		}
	}
	q := db.New(d.SQL)
	logger := slog.New(slog.DiscardHandler)
	showSvc := show.NewService(q, nil, nil, nil, logger)
	lib := library.Library{ID: "lib", RootPath: root}

	if err := scanLibraryFiles(context.Background(), lib, showSvc, q, map[string]bool{}, logger); err != nil {
		t.Fatal(err)
	}
	files, _ := q.ListEpisodeFilesByEpisodeID(context.Background(), "e1")
	if preexisting {
		if len(files) != 1 || files[0].Path != "/elsewhere/existing.mkv" {
			t.Fatalf("episode files = %+v, want only the existing file", files)
		}
		return
	}
	if len(files) != wantFiles || (wantFiles == 1 && files[0].Path != file) {
		t.Fatalf("episode files = %+v, want %d (%s)", files, wantFiles, file)
	}
}
