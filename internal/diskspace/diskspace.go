// Package diskspace reports free disk space for a path.
package diskspace

import (
	"os"
	"path/filepath"
)

// GB is one gigabyte (10^9 bytes), the unit of min_free_space_gb.
const GB = 1_000_000_000

// Free returns the bytes available to Pilot on the filesystem holding
// path. A path that doesn't exist yet (a series folder before its first
// import) is resolved to its nearest existing parent.
func Free(path string) (uint64, error) {
	p, err := existingAncestor(path)
	if err != nil {
		return 0, err
	}
	return free(p)
}

func existingAncestor(path string) (string, error) {
	p := filepath.Clean(path)
	for {
		_, err := os.Stat(p)
		if err == nil {
			return p, nil
		}
		parent := filepath.Dir(p)
		if parent == p || !os.IsNotExist(err) {
			return "", err
		}
		p = parent
	}
}
