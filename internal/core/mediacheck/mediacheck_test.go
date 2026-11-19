package mediacheck

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ep.mkv")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheck_EmptyFileRejected(t *testing.T) {
	if err := Check(write(t, nil)); !errors.Is(err, ErrNoData) {
		t.Fatalf("empty file: got %v, want ErrNoData", err)
	}
}

// The real failure: a preallocated download of the right size that never
// received a byte (Patriot S02, Ted Lasso S04E02-E04).
func TestCheck_AllZeroFileRejected(t *testing.T) {
	if err := Check(write(t, make([]byte, 3<<20))); !errors.Is(err, ErrNoData) {
		t.Fatalf("zero-filled file: got %v, want ErrNoData", err)
	}
}

// A sparse file (holes, no blocks allocated) reads as zeros too.
func TestCheck_SparseFileRejected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sparse.mkv")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(50 << 20); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := Check(p); !errors.Is(err, ErrNoData) {
		t.Fatalf("sparse file: got %v, want ErrNoData", err)
	}
}

func TestCheck_RealHeaderAccepted(t *testing.T) {
	data := make([]byte, 3<<20)
	copy(data, []byte{0x1a, 0x45, 0xdf, 0xa3}) // EBML (Matroska) magic
	if err := Check(write(t, data)); err != nil {
		t.Fatalf("file with a header: got %v, want nil", err)
	}
}

// Only the tail has data (header piece missing): still accepted, since
// the file is not empty. Check catches no-data files, not every damaged one.
func TestCheck_DataLaterInFileAccepted(t *testing.T) {
	data := make([]byte, 3<<20)
	data[len(data)-1] = 1
	if err := Check(write(t, data)); err != nil {
		t.Fatalf("file with data near the end: got %v, want nil", err)
	}
}

func TestCheck_SmallFileAccepted(t *testing.T) {
	if err := Check(write(t, []byte("video"))); err != nil {
		t.Fatalf("small non-zero file: got %v, want nil", err)
	}
}

func TestCheck_MissingFile(t *testing.T) {
	if err := Check(filepath.Join(t.TempDir(), "nope.mkv")); err == nil || errors.Is(err, ErrNoData) {
		t.Fatalf("missing file: got %v, want a not-exist error", err)
	}
}
