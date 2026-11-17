package recyclebin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/beacon-stack/pilot/internal/core/mediamanagement"
)

type fakeSettings struct {
	st  mediamanagement.Settings
	err error
}

func (f *fakeSettings) Get(context.Context) (mediamanagement.Settings, error) { return f.st, f.err }

type fixture struct {
	svc   *Service
	mm    *fakeSettings
	root  string // library root
	bin   string
	clock time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{
		mm:    &fakeSettings{},
		root:  filepath.Join(dir, "tv"),
		bin:   filepath.Join(dir, "recycle"),
		clock: time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC),
	}
	f.mm.st.RecycleBinPath = f.bin
	f.mm.st.RecycleBinCleanupDays = 7
	f.svc = New(f.mm, nil)
	f.svc.now = func() time.Time { return f.clock }
	return f
}

func write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestDelete_MovesIntoBinKeepingFolders(t *testing.T) {
	f := newFixture(t)
	file := filepath.Join(f.root, "Show (2024)", "Season 01", "Show - S01E01.mkv")
	write(t, file, "old")

	dest, err := f.svc.Delete(context.Background(), file, f.root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(f.bin, "Show (2024)", "Season 01", "Show - S01E01.mkv")
	if dest != want {
		t.Fatalf("recycled to %s, want %s", dest, want)
	}
	if exists(file) {
		t.Fatal("original still in the library")
	}
	if got, _ := os.ReadFile(want); string(got) != "old" {
		t.Fatalf("recycled content = %q", got)
	}
	// The cleanup period starts now.
	if info, _ := os.Stat(want); !info.ModTime().Equal(f.clock) {
		t.Errorf("mtime = %v, want the recycle time", info.ModTime())
	}

	// The same name again keeps both.
	write(t, file, "older")
	dest2, err := f.svc.Delete(context.Background(), file, f.root)
	if err != nil || dest2 != filepath.Join(f.bin, "Show (2024)", "Season 01", "Show - S01E01 (2).mkv") {
		t.Fatalf("second recycle = %s, %v", dest2, err)
	}
	if got, _ := os.ReadFile(want); string(got) != "old" {
		t.Fatal("the first recycled file was overwritten")
	}
}

func TestDelete_OutsideLibraryUsesFileName(t *testing.T) {
	f := newFixture(t)
	file := filepath.Join(t.TempDir(), "elsewhere", "ep.mkv")
	write(t, file, "x")
	dest, err := f.svc.Delete(context.Background(), file, f.root)
	if err != nil || dest != filepath.Join(f.bin, "ep.mkv") {
		t.Fatalf("dest = %s, %v", dest, err)
	}
}

func TestDelete_NoBinDeletesPermanently(t *testing.T) {
	f := newFixture(t)
	f.mm.st.RecycleBinPath = ""
	file := filepath.Join(f.root, "ep.mkv")
	write(t, file, "x")
	dest, err := f.svc.Delete(context.Background(), file, f.root)
	if err != nil || dest != "" || exists(file) {
		t.Fatalf("dest=%q err=%v exists=%v", dest, err, exists(file))
	}
	if exists(f.bin) {
		t.Fatal("bin created with no bin set")
	}
}

func TestDelete_SettingsErrorKeepsFile(t *testing.T) {
	f := newFixture(t)
	f.mm.err = errors.New("db down")
	file := filepath.Join(f.root, "ep.mkv")
	write(t, file, "x")
	if _, err := f.svc.Delete(context.Background(), file, f.root); err == nil {
		t.Fatal("no error")
	}
	if !exists(file) {
		t.Fatal("file deleted although the recycle bin setting was unknown")
	}
}

func TestDelete_MissingFileIsFine(t *testing.T) {
	f := newFixture(t)
	if dest, err := f.svc.Delete(context.Background(), filepath.Join(f.root, "gone.mkv"), f.root); err != nil || dest != "" {
		t.Fatalf("dest=%q err=%v", dest, err)
	}
}

func TestCleanup_DeletesOldFilesAndEmptyFolders(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	oldFile := filepath.Join(f.root, "Old Show", "Season 01", "old.mkv")
	newFile := filepath.Join(f.root, "New Show", "new.mkv")
	write(t, oldFile, "o")
	write(t, newFile, "n")
	oldDest, _ := f.svc.Delete(ctx, oldFile, f.root)
	f.clock = f.clock.Add(5 * 24 * time.Hour)
	newDest, _ := f.svc.Delete(ctx, newFile, f.root)
	f.clock = f.clock.Add(3 * 24 * time.Hour) // old: 8 days, new: 3 days

	n, err := f.svc.Cleanup(ctx)
	if err != nil || n != 1 {
		t.Fatalf("cleanup deleted %d, %v; want 1", n, err)
	}
	if exists(oldDest) || !exists(newDest) {
		t.Fatalf("old exists=%v new exists=%v", exists(oldDest), exists(newDest))
	}
	if exists(filepath.Join(f.bin, "Old Show")) {
		t.Error("empty folders left behind")
	}
	if !exists(f.bin) {
		t.Error("the bin folder itself was removed")
	}

	// 0 days: never cleaned.
	f.mm.st.RecycleBinCleanupDays = 0
	f.clock = f.clock.Add(365 * 24 * time.Hour)
	if n, _ := f.svc.Cleanup(ctx); n != 0 || !exists(newDest) {
		t.Fatal("cleanup ran with a period of 0")
	}

	if n, err := f.svc.Empty(ctx); err != nil || n != 1 || exists(newDest) {
		t.Fatalf("empty: %d, %v", n, err)
	}
}

func TestWithin(t *testing.T) {
	for _, c := range []struct {
		dir, p string
		want   bool
	}{
		{"/tv", "/tv/a.mkv", true},
		{"/tv", "/tv", true},
		{"/tv", "/tv2/a.mkv", false},
		{"/tv", "/a.mkv", false},
	} {
		if got := within(filepath.FromSlash(c.dir), filepath.FromSlash(c.p)); got != c.want {
			t.Errorf("within(%q, %q) = %v", c.dir, c.p, got)
		}
	}
}
