package importer

// Upgrades move the replaced file into the recycle bin (when one is set),
// and a failed transfer never loses the file being replaced.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beacon-stack/pilot/internal/diskspace"
)

func (f *retryFixture) setRecycleBin(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "recycle")
	f.exec(`UPDATE media_management SET recycle_bin_path = ? WHERE id = 1`, bin)
	return bin
}

func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

func TestUpgrade_ReplacedFileGoesToRecycleBin(t *testing.T) {
	f := newRetryFixture(t)
	ctx := context.Background()
	bin := f.setRecycleBin(t)
	dl := filepath.Join(t.TempDir(), "dl")

	oldSrc, oldSize := writeSource(t, dl, "Show.S01E01.720p.WEB.mkv", "old 720p")
	if err := f.svc.AttachFile(ctx, "ep-1", "series-1", oldSrc, oldSize, q720); err != nil {
		t.Fatal(err)
	}
	oldLib := f.filesFor("ep-1")[0]
	newSrc, newSize := writeSource(t, dl, "Show.S01E01.1080p.BluRay.mkv", "new 1080p")
	if err := f.svc.AttachFile(ctx, "ep-1", "series-1", newSrc, newSize, q1080); err != nil {
		t.Fatal(err)
	}

	rel, _ := filepath.Rel(f.tvRoot, oldLib)
	recycled := filepath.Join(bin, rel)
	if got, err := os.ReadFile(recycled); err != nil || string(got) != "old 720p" {
		t.Fatalf("replaced file not in the recycle bin at %s (bin has %v): %v", recycled, filesUnder(t, bin), err)
	}
	if _, err := os.Stat(oldLib); !os.IsNotExist(err) {
		t.Fatal("replaced file still in the library")
	}
}

func TestUpgrade_SameFilenameRecyclesOldContent(t *testing.T) {
	f := newRetryFixture(t)
	ctx := context.Background()
	bin := f.setRecycleBin(t)
	f.exec(`UPDATE libraries SET naming_format = ? WHERE id = 'lib'`, "{Series Title} - S{Season:00}E{Episode:00} - {Episode Title}")
	dl := filepath.Join(t.TempDir(), "dl")

	oldSrc, oldSize := writeSource(t, dl, "Show.S01E01.720p.WEB.mkv", "old")
	if err := f.svc.AttachFile(ctx, "ep-1", "series-1", oldSrc, oldSize, q720); err != nil {
		t.Fatal(err)
	}
	path := f.filesFor("ep-1")[0]
	newSrc, newSize := writeSource(t, dl, "Show.S01E01.1080p.BluRay.mkv", "new and bigger")
	if err := f.svc.AttachFile(ctx, "ep-1", "series-1", newSrc, newSize, q1080); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new and bigger" {
		t.Fatalf("library file = %q", got)
	}
	in := filesUnder(t, bin)
	if len(in) != 1 || !strings.HasSuffix(in[0], filepath.Base(path)) {
		t.Fatalf("recycle bin = %v, want the old file", in)
	}
	if got, _ := os.ReadFile(filepath.Join(bin, filepath.FromSlash(in[0]))); string(got) != "old" {
		t.Fatalf("recycled content = %q", got)
	}
	// The download the old library file was a hard link to is untouched.
	if got, _ := os.ReadFile(oldSrc); string(got) != "old" {
		t.Fatal("the old download was changed")
	}
}

// If the new file can't be transferred, the file it would replace stays,
// with or without a recycle bin.
func TestUpgrade_FailedTransferKeepsOldFile(t *testing.T) {
	for _, withBin := range []bool{false, true} {
		t.Run(map[bool]string{false: "no bin", true: "bin"}[withBin], func(t *testing.T) {
			f := newRetryFixture(t)
			ctx := context.Background()
			bin := ""
			if withBin {
				bin = f.setRecycleBin(t)
			}
			f.exec(`UPDATE libraries SET naming_format = ? WHERE id = 'lib'`, "{Series Title} - S{Season:00}E{Episode:00} - {Episode Title}")
			dl := filepath.Join(t.TempDir(), "dl")
			oldSrc, oldSize := writeSource(t, dl, "Show.S01E01.720p.WEB.mkv", "old")
			if err := f.svc.AttachFile(ctx, "ep-1", "series-1", oldSrc, oldSize, q720); err != nil {
				t.Fatal(err)
			}
			path := f.filesFor("ep-1")[0]

			forceCopy(t, 1*diskspace.GB) // below the library's 5 GB minimum
			// A different size: same-size files count as already imported
			// (the cross-device retry check in sameImportedFile).
			newSrc, newSize := writeSource(t, dl, "Show.S01E01.1080p.BluRay.mkv", "new and bigger")
			err := f.svc.AttachFile(ctx, "ep-1", "series-1", newSrc, newSize, q1080)
			if !errors.Is(err, ErrInsufficientSpace) {
				t.Fatalf("bin=%v: err = %v, want ErrInsufficientSpace", withBin, err)
			}
			if got, _ := os.ReadFile(path); string(got) != "old" {
				t.Fatalf("bin=%v: the old file was lost (now %q)", withBin, got)
			}
			if withBin {
				if in := filesUnder(t, bin); len(in) != 0 {
					t.Fatalf("recycled although nothing replaced it: %v", in)
				}
			}
		})
	}
}
