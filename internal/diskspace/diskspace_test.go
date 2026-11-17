package diskspace

import (
	"path/filepath"
	"testing"
)

func TestFree(t *testing.T) {
	dir := t.TempDir()
	n, err := Free(dir)
	if err != nil || n == 0 {
		t.Fatalf("Free(%q) = %d, %v; want some space", dir, n, err)
	}
	// A folder that doesn't exist yet reports its parent's filesystem.
	m, err := Free(filepath.Join(dir, "Show (2024)", "Season 01"))
	if err != nil || m == 0 {
		t.Fatalf("Free(missing folder) = %d, %v", m, err)
	}
}
