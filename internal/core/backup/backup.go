// Package backup makes, lists, prunes and restores database backups, as
// in Sonarr's System > Backup: scheduled backups every N days kept for M
// days, plus backups made on demand.
//
// A backup is a consistent copy of the SQLite database made with VACUUM
// INTO, stored as a plain .db file in a "backups" folder next to the
// database. Restoring stages the file next to the database; it is swapped
// in on the next start (ApplyStagedRestore), before the database opens.
package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// Kinds of backup.
const (
	KindScheduled = "scheduled"
	KindManual    = "manual"
)

// ErrNotFound is returned for a backup name that doesn't exist.
var ErrNotFound = errors.New("backup not found")

// ErrInvalidName is returned for a name that isn't a backup file name.
// Names come from the API, so this also blocks path traversal.
var ErrInvalidName = errors.New("invalid backup name")

// nameRe matches backup file names: pilot-<kind>-YYYYMMDD-HHMMSS[-N].db
var nameRe = regexp.MustCompile(`^pilot-(scheduled|manual)-(\d{8}-\d{6})(?:-\d+)?\.db$`)

const timeLayout = "20060102-150405"

// Backup describes one backup file.
type Backup struct {
	Name      string
	Kind      string
	Size      int64
	CreatedAt time.Time
}

// Settings is the backup schedule.
type Settings struct {
	// IntervalDays between scheduled backups; 0 turns them off.
	IntervalDays int `json:"interval_days"`
	// RetentionDays scheduled backups are kept. The newest is always kept.
	RetentionDays int `json:"retention_days"`
}

// DefaultSettings matches Sonarr: weekly, kept four weeks.
func DefaultSettings() Settings { return Settings{IntervalDays: 7, RetentionDays: 28} }

// ValidationError explains why settings can't be saved.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// Validate checks s.
func (s Settings) Validate() error {
	if s.IntervalDays < 0 || s.IntervalDays > 365 {
		return &ValidationError{"the interval must be between 0 (off) and 365 days"}
	}
	if s.RetentionDays < 1 || s.RetentionDays > 3650 {
		return &ValidationError{"retention must be between 1 and 3650 days"}
	}
	return nil
}

const settingKey = "backup_settings"

// Service manages backups for one SQLite database.
type Service struct {
	db     *sql.DB
	q      db.Querier
	dbPath string
	dir    string
	now    func() time.Time
}

// NewService creates a Service. Backups go in a "backups" folder next to
// dbPath.
func NewService(sqlDB *sql.DB, q db.Querier, dbPath string) *Service {
	return &Service{db: sqlDB, q: q, dbPath: dbPath, dir: filepath.Join(filepath.Dir(dbPath), "backups"), now: time.Now}
}

// Dir is the folder backups are stored in.
func (s *Service) Dir() string { return s.dir }

// GetSettings returns the saved schedule, or DefaultSettings.
func (s *Service) GetSettings(ctx context.Context) (Settings, error) {
	raw, err := s.q.GetSetting(ctx, settingKey)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	st := DefaultSettings()
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return Settings{}, fmt.Errorf("decoding backup settings: %w", err)
	}
	return st, nil
}

// SaveSettings validates and saves the schedule.
func (s *Service) SaveSettings(ctx context.Context, st Settings) error {
	if err := st.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.q.SetSetting(ctx, db.SetSettingParams{Key: settingKey, Value: string(b), UpdatedAt: s.now().UTC().Format(time.RFC3339)})
}

// Create makes a backup of the given kind.
func (s *Service) Create(ctx context.Context, kind string) (Backup, error) {
	if kind != KindScheduled && kind != KindManual {
		return Backup{}, fmt.Errorf("unknown backup kind %q", kind)
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return Backup{}, fmt.Errorf("creating backup folder: %w", err)
	}
	stamp := s.now().UTC().Format(timeLayout)
	name := fmt.Sprintf("pilot-%s-%s.db", kind, stamp)
	for n := 2; fileExists(filepath.Join(s.dir, name)); n++ {
		name = fmt.Sprintf("pilot-%s-%s-%d.db", kind, stamp, n)
	}
	path := filepath.Join(s.dir, name)
	// The path is a bound parameter, so quotes in it need no escaping.
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		_ = os.Remove(path)
		return Backup{}, fmt.Errorf("writing backup: %w", err)
	}
	_ = os.Chmod(path, 0o600) // the database holds API keys and passwords
	info, err := os.Stat(path)
	if err != nil {
		return Backup{}, err
	}
	return Backup{Name: name, Kind: kind, Size: info.Size(), CreatedAt: s.now().UTC().Truncate(time.Second)}, nil
}

// List returns every backup, newest first.
func (s *Service) List() ([]Backup, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Backup{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Backup{}
	for _, e := range entries {
		m := nameRe.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() {
			continue
		}
		at, err := time.Parse(timeLayout, m[2])
		if err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Backup{Name: e.Name(), Kind: m[1], Size: info.Size(), CreatedAt: at})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].Name > out[j].Name
	})
	return out, nil
}

// Path returns the file path of a backup, checking the name.
func (s *Service) Path(name string) (string, error) {
	if !nameRe.MatchString(name) {
		return "", ErrInvalidName
	}
	p := filepath.Join(s.dir, name)
	if !fileExists(p) {
		return "", ErrNotFound
	}
	return p, nil
}

// Delete removes a backup.
func (s *Service) Delete(name string) error {
	p, err := s.Path(name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// Restore stages a backup to replace the database on the next start.
func (s *Service) Restore(name string) error {
	p, err := s.Path(name)
	if err != nil {
		return err
	}
	return copyFile(p, StagingPath(s.dbPath))
}

// RunScheduled makes a scheduled backup if one is due and prunes old
// ones. It reports whether a backup was made. Safe to call often: the
// schedule is read from the newest scheduled backup on disk, so it
// survives restarts.
func (s *Service) RunScheduled(ctx context.Context) (bool, error) {
	st, err := s.GetSettings(ctx)
	if err != nil {
		return false, err
	}
	if st.IntervalDays == 0 {
		return false, nil
	}
	all, err := s.List()
	if err != nil {
		return false, err
	}
	made := false
	if last := newestScheduled(all); last == nil || s.now().Sub(last.CreatedAt) >= time.Duration(st.IntervalDays)*24*time.Hour {
		if _, err := s.Create(ctx, KindScheduled); err != nil {
			return false, err
		}
		made = true
		if all, err = s.List(); err != nil {
			return made, err
		}
	}
	return made, s.prune(all, st.RetentionDays)
}

// prune deletes scheduled backups older than retentionDays, always
// keeping the newest one. Manual backups are never pruned.
func (s *Service) prune(all []Backup, retentionDays int) error {
	newest := newestScheduled(all)
	cutoff := s.now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	var errs []error
	for _, b := range all {
		if b.Kind != KindScheduled || (newest != nil && b.Name == newest.Name) || !b.CreatedAt.Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, b.Name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func newestScheduled(all []Backup) *Backup {
	for i := range all { // newest first
		if all[i].Kind == KindScheduled {
			return &all[i]
		}
	}
	return nil
}

// StagingPath is where a restore waits for the next start.
func StagingPath(dbPath string) string { return dbPath + ".restore" }

// ApplyStagedRestore swaps a staged restore into place. Call it before
// opening the database. It reports whether a restore was applied.
//
// The database runs in WAL mode: the old database's -wal and -shm files
// are removed first. Left in place, SQLite would replay the old
// database's write-ahead log onto the restored file after an unclean
// shutdown, corrupting it.
func ApplyStagedRestore(dbPath string) (bool, error) {
	staged := StagingPath(dbPath)
	if !fileExists(staged) {
		return false, nil
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(dbPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("removing %s: %w", dbPath+suffix, err)
		}
	}
	if err := os.Rename(staged, dbPath); err != nil {
		return false, err
	}
	return true, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
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
	return os.Rename(tmp, dst)
}
