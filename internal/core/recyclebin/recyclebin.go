// Package recyclebin moves episode files Pilot deletes (replaced by an
// upgrade, or deleted from the UI) into a recycle bin folder instead of
// deleting them, as in Sonarr. Files keep their series/season folders
// inside the bin, and are deleted for good after the cleanup period.
//
// With no recycle bin folder set, files are deleted permanently, which is
// the behavior before the recycle bin existed.
package recyclebin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/beacon-stack/pilot/internal/core/mediamanagement"
)

// SettingsGetter loads the media management settings, which hold the
// recycle bin folder and cleanup period.
type SettingsGetter interface {
	Get(ctx context.Context) (mediamanagement.Settings, error)
}

// Service recycles files.
type Service struct {
	mm     SettingsGetter
	logger *slog.Logger
	now    func() time.Time
}

// New creates a Service.
func New(mm SettingsGetter, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{mm: mm, logger: logger, now: time.Now}
}

// Delete removes the file at path: into the recycle bin when one is set,
// otherwise permanently. libraryRoot (may be "") is the library the file
// belongs to; the file keeps its folders below it inside the bin. It
// returns where the file went ("" when deleted permanently). A file that
// is already gone is not an error.
//
// If the settings can't be loaded the file is left alone and an error
// returned: never delete permanently when the user may have a bin set.
func (s *Service) Delete(ctx context.Context, path, libraryRoot string) (string, error) {
	st, err := s.mm.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("loading recycle bin settings: %w", err)
	}
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	bin := strings.TrimSpace(st.RecycleBinPath)
	if bin == "" || within(bin, path) {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		return "", nil
	}
	return s.moveToBin(bin, path, libraryRoot)
}

// Preserve moves the file at path into the recycle bin when one is set,
// and otherwise leaves it alone. Use it just before overwriting a file:
// with no bin, the overwrite itself replaces it (atomically), and nothing
// is lost if the overwrite fails. Returns where the file went ("" when
// it was left in place).
func (s *Service) Preserve(ctx context.Context, path, libraryRoot string) (string, error) {
	st, err := s.mm.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("loading recycle bin settings: %w", err)
	}
	bin := strings.TrimSpace(st.RecycleBinPath)
	if bin == "" || within(bin, path) {
		return "", nil
	}
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return s.moveToBin(bin, path, libraryRoot)
}

func (s *Service) moveToBin(bin, path, libraryRoot string) (string, error) {
	rel := filepath.Base(path)
	if libraryRoot != "" && within(libraryRoot, path) {
		if r, err := filepath.Rel(libraryRoot, path); err == nil {
			rel = r
		}
	}
	dest := uniquePath(filepath.Join(bin, rel))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("creating recycle bin folder: %w", err)
	}
	if err := move(path, dest); err != nil {
		return "", fmt.Errorf("moving %s to the recycle bin: %w", path, err)
	}
	// The cleanup period counts from when the file was recycled.
	now := s.now()
	_ = os.Chtimes(dest, now, now)
	s.logger.Info("recycled file", "path", path, "recycled_to", dest)
	return dest, nil
}

// Cleanup deletes files that have been in the recycle bin longer than
// the cleanup period, then removes folders left empty. It returns how
// many files were deleted. Nothing happens with no bin or a period of 0.
func (s *Service) Cleanup(ctx context.Context) (int, error) {
	st, err := s.mm.Get(ctx)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(st.RecycleBinPath) == "" || st.RecycleBinCleanupDays <= 0 {
		return 0, nil
	}
	cutoff := s.now().Add(-time.Duration(st.RecycleBinCleanupDays) * 24 * time.Hour)
	return s.clear(st.RecycleBinPath, func(info fs.FileInfo) bool { return info.ModTime().Before(cutoff) })
}

// Empty deletes everything in the recycle bin. It returns how many files
// were deleted.
func (s *Service) Empty(ctx context.Context) (int, error) {
	st, err := s.mm.Get(ctx)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(st.RecycleBinPath) == "" {
		return 0, nil
	}
	return s.clear(st.RecycleBinPath, func(fs.FileInfo) bool { return true })
}

func (s *Service) clear(bin string, del func(fs.FileInfo) bool) (int, error) {
	bin = filepath.Clean(bin)
	if _, err := os.Stat(bin); errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	var dirs []string
	n := 0
	var errs []error
	walkErr := filepath.WalkDir(bin, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if d.IsDir() {
			if p != bin {
				dirs = append(dirs, p)
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		if del(info) {
			if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, err)
				return nil
			}
			n++
		}
		return nil
	})
	if walkErr != nil {
		errs = append(errs, walkErr)
	}
	// Deepest first, so a parent is empty by the time it is tried.
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		_ = os.Remove(d) // fails, harmlessly, when not empty
	}
	if n > 0 {
		s.logger.Info("recycle bin cleaned", "files_deleted", n)
	}
	return n, errors.Join(errs...)
}

// within reports whether p is inside (or is) dir.
func within(dir, p string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(p))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

// uniquePath returns p, or "name (2).ext", "name (3).ext"... when p is
// taken, so an earlier recycled file of the same name is kept.
func uniquePath(p string) string {
	if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Lstat(c); errors.Is(err, fs.ErrNotExist) {
			return c
		}
	}
}

// move renames src to dst, or copies and removes src when they are on
// different filesystems. On a failed copy, src is left in place.
func move(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".pilot-tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Remove(src)
}
