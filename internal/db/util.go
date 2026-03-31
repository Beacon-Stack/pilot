package db

import (
	"os"
	"path/filepath"
)

func ensureDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return os.MkdirAll(dir, 0o700)
}
