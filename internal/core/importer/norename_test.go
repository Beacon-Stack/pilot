package importer

// With "Rename Episodes" off, files still go into the library's series
// and season folders, keeping their original names (as in Sonarr).
// Before, nothing was moved: the download path itself was recorded as
// the library file, so the library stayed empty and upgrades never
// replaced anything (the old "library" file was outside the library).

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAttachFile_RenameOffKeepsNameInLibraryFolders(t *testing.T) {
	f := newRetryFixture(t)
	ctx := context.Background()
	f.exec(`UPDATE media_management SET rename_episodes = FALSE WHERE id = 1`)
	dl := filepath.Join(t.TempDir(), "dl")

	src, size := writeSource(t, dl, "Show.S01E01.720p.WEB.mkv", "720p bytes")
	if err := f.svc.AttachFile(ctx, "ep-1", "series-1", src, size, q720); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(f.tvRoot, "Show (2022)", "Season 01", "Show.S01E01.720p.WEB.mkv")
	if got := f.filesFor("ep-1"); len(got) != 1 || got[0] != want {
		t.Fatalf("recorded files = %v, want [%s]", got, want)
	}
	if b, err := os.ReadFile(want); err != nil || string(b) != "720p bytes" {
		t.Fatalf("file not in the library: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("the download was moved; it must stay for the client to seed")
	}

	// An upgrade (a different original name) replaces the old file.
	src2, size2 := writeSource(t, dl, "Show.S01E01.1080p.BluRay.mkv", "1080p bytes, bigger")
	if err := f.svc.AttachFile(ctx, "ep-1", "series-1", src2, size2, q1080); err != nil {
		t.Fatal(err)
	}
	want2 := filepath.Join(f.tvRoot, "Show (2022)", "Season 01", "Show.S01E01.1080p.BluRay.mkv")
	if got := f.filesFor("ep-1"); len(got) != 1 || got[0] != want2 {
		t.Fatalf("after upgrade = %v, want [%s]", got, want2)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatal("the replaced file is still in the library")
	}
}
