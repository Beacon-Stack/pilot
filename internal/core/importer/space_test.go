package importer

// The library's min_free_space_gb is enforced when an import has to copy
// a file into the library. Before this the setting was stored and shown
// but never read, so imports could fill the disk.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beacon-stack/pilot/internal/diskspace"
)

// forceCopy makes hardlinks fail and reports avail bytes free.
func forceCopy(t *testing.T, avail uint64) {
	t.Helper()
	oldLink, oldFree := linkFile, freeSpace
	linkFile = func(string, string) error { return errors.New("cross-device link") }
	freeSpace = func(string) (uint64, error) { return avail, nil }
	t.Cleanup(func() { linkFile, freeSpace = oldLink, oldFree })
}

func TestTransferFile_CopyRefusedBelowMinimum(t *testing.T) {
	forceCopy(t, 6*diskspace.GB)
	dir := t.TempDir()
	src, _ := writeSource(t, dir, "src.mkv", "video")
	dst := filepath.Join(dir, "lib", "dst.mkv")
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)

	err := transferFile(src, dst, 10*diskspace.GB)
	if !errors.Is(err, ErrInsufficientSpace) {
		t.Fatalf("err = %v, want ErrInsufficientSpace", err)
	}
	for _, p := range []string{dst, dst + ".pilot-tmp"} {
		if _, statErr := os.Stat(p); !os.IsNotExist(statErr) {
			t.Fatalf("%s was written despite the refusal", p)
		}
	}

	// Enough space: the copy goes ahead.
	freeSpace = func(string) (uint64, error) { return 20 * diskspace.GB, nil }
	if err := transferFile(src, dst, 10*diskspace.GB); err != nil {
		t.Fatalf("copy with enough space: %v", err)
	}
	// No minimum: never checked.
	freeSpace = func(string) (uint64, error) { return 0, nil }
	if err := transferFile(src, dst+"2", 0); err != nil {
		t.Fatalf("copy with no minimum: %v", err)
	}
}

// A hardlink takes no space, so a full disk doesn't block it.
func TestTransferFile_HardlinkIgnoresFreeSpace(t *testing.T) {
	old := freeSpace
	freeSpace = func(string) (uint64, error) { return 0, nil }
	t.Cleanup(func() { freeSpace = old })
	dir := t.TempDir()
	src, _ := writeSource(t, dir, "src.mkv", "video")
	if err := transferFile(src, filepath.Join(dir, "dst.mkv"), 10*diskspace.GB); err != nil {
		t.Fatalf("hardlink refused: %v", err)
	}
}

// End to end: the library's minimum (5 GB by default) reaches the copy.
func TestAttachFile_RespectsLibraryMinimum(t *testing.T) {
	f := newRetryFixture(t)
	ctx := context.Background()
	forceCopy(t, 2*diskspace.GB)
	src, size := writeSource(t, filepath.Join(t.TempDir(), "dl"), "Show.S01E01.1080p.WEB.mkv", "bytes")

	err := f.svc.AttachFile(ctx, "ep-1", "series-1", src, size, q1080)
	if !errors.Is(err, ErrInsufficientSpace) {
		t.Fatalf("err = %v, want ErrInsufficientSpace", err)
	}
	if errors.Is(err, ErrUnimportableContent) {
		t.Fatal("a full disk must be a soft failure, never a blocklist")
	}
	if files := f.filesFor("ep-1"); len(files) != 0 {
		t.Fatalf("file recorded despite the refusal: %v", files)
	}

	f.exec(`UPDATE libraries SET min_free_space_gb = 0 WHERE id = 'lib'`)
	if err := f.svc.AttachFile(ctx, "ep-1", "series-1", src, size, q1080); err != nil {
		t.Fatalf("with no minimum: %v", err)
	}
}

// A path that doesn't exist for Pilot (a client in a container reporting
// its own paths) points the user at remote path mappings, and stays a
// soft failure.
func TestImportFile_MissingPathSuggestsRemotePathMapping(t *testing.T) {
	f := newRetryFixture(t)
	err := f.svc.ImportFile(context.Background(), f.grabID, "/downloads/Show.S01.1080p.WEB")
	if err == nil || !strings.Contains(err.Error(), "remote path mapping") {
		t.Fatalf("err = %v, want a remote path mapping hint", err)
	}
	if errors.Is(err, ErrUnimportableContent) {
		t.Fatal("a missing path must never blocklist the release")
	}
}
