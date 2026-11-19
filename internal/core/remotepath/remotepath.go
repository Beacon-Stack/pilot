// Package remotepath implements remote path mappings, as in Sonarr. A
// download client that runs in a container or on another machine reports
// paths as it sees them (/downloads/Show.S01E01); a mapping rewrites that
// prefix to the same folder as Pilot sees it (/mnt/media/downloads).
//
// Remote paths may be Unix or Windows style (a client on Windows reports
// C:\Downloads\...). They are compared by path component, with either
// separator, and case-insensitively for Windows paths.
package remotepath

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// ErrNotFound is returned when a mapping ID doesn't exist.
var ErrNotFound = errors.New("remote path mapping not found")

// Mapping is one remote path mapping.
type Mapping struct {
	ID               string
	DownloadClientID string // "" = any client
	RemotePath       string
	LocalPath        string
}

// Request is the editable part of a mapping.
type Request struct {
	DownloadClientID string
	RemotePath       string
	LocalPath        string
}

// ValidationError explains why a Request was rejected.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

var reWindowsPath = regexp.MustCompile(`^(?:[A-Za-z]:[\\/]|\\\\)`)

func isWindowsPath(p string) bool { return reWindowsPath.MatchString(p) }

// canon puts a remote path (or a reported path) in comparable form:
// forward slashes, no trailing slash (except the root), lower-cased when
// it is a Windows path.
func canon(p string) string {
	win := isWindowsPath(p)
	p = strings.ReplaceAll(p, `\`, "/")
	for len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	if win {
		p = strings.ToLower(p)
	}
	return p
}

// Apply rewrites path with the best matching mapping: the longest remote
// path that is path itself or a parent folder of it, preferring one for
// this client over one for any client. With no match, path is returned
// unchanged.
func Apply(mappings []Mapping, clientID, path string) string {
	if path == "" {
		return path
	}
	cp := canon(path)
	var best *Mapping
	bestLen, bestSpecific := -1, false
	for i := range mappings {
		m := &mappings[i]
		if m.DownloadClientID != "" && m.DownloadClientID != clientID {
			continue
		}
		cr := canon(m.RemotePath)
		if cr == "" {
			continue
		}
		prefix := cr
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		if cp != cr && !strings.HasPrefix(cp, prefix) {
			continue
		}
		specific := m.DownloadClientID != ""
		if len(cr) > bestLen || (len(cr) == bestLen && specific && !bestSpecific) {
			best, bestLen, bestSpecific = m, len(cr), specific
		}
	}
	if best == nil {
		return path
	}
	// The rest of the path after the remote prefix, in its original case.
	slashed := strings.ReplaceAll(path, `\`, "/")
	if bestLen > len(slashed) {
		return path // lower-casing changed the length; don't guess
	}
	rest := strings.TrimLeft(slashed[bestLen:], "/")
	if rest == "" {
		return filepath.Clean(best.LocalPath)
	}
	return filepath.Join(best.LocalPath, filepath.FromSlash(rest))
}

// Service stores mappings and applies them.
type Service struct {
	q   db.Querier
	now func() time.Time
}

// NewService creates a Service.
func NewService(q db.Querier) *Service { return &Service{q: q, now: time.Now} }

// Map rewrites a path reported by the download client clientID. If the
// mappings can't be loaded, the path is returned unchanged.
func (s *Service) Map(ctx context.Context, clientID, path string) string {
	if s == nil || path == "" {
		return path
	}
	ms, err := s.List(ctx)
	if err != nil {
		return path
	}
	return Apply(ms, clientID, path)
}

// List returns every mapping.
func (s *Service) List(ctx context.Context) ([]Mapping, error) {
	rows, err := s.q.ListRemotePathMappings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Mapping, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

// Create validates and stores a mapping.
func (s *Service) Create(ctx context.Context, req Request) (Mapping, error) {
	req, err := normalize(req)
	if err != nil {
		return Mapping{}, err
	}
	if err := s.checkClient(ctx, req.DownloadClientID); err != nil {
		return Mapping{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	row, err := s.q.CreateRemotePathMapping(ctx, db.CreateRemotePathMappingParams{
		ID:               uuid.New().String(),
		DownloadClientID: nullable(req.DownloadClientID),
		RemotePath:       req.RemotePath,
		LocalPath:        req.LocalPath,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		return Mapping{}, err
	}
	return fromRow(row), nil
}

// Update validates and replaces a mapping.
func (s *Service) Update(ctx context.Context, id string, req Request) (Mapping, error) {
	req, err := normalize(req)
	if err != nil {
		return Mapping{}, err
	}
	if err := s.checkClient(ctx, req.DownloadClientID); err != nil {
		return Mapping{}, err
	}
	row, err := s.q.UpdateRemotePathMapping(ctx, db.UpdateRemotePathMappingParams{
		ID:               id,
		DownloadClientID: nullable(req.DownloadClientID),
		RemotePath:       req.RemotePath,
		LocalPath:        req.LocalPath,
		UpdatedAt:        s.now().UTC().Format(time.RFC3339),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Mapping{}, ErrNotFound
	}
	if err != nil {
		return Mapping{}, err
	}
	return fromRow(row), nil
}

// Delete removes a mapping.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.q.GetRemotePathMapping(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return s.q.DeleteRemotePathMapping(ctx, id)
}

func (s *Service) checkClient(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	_, err := s.q.GetDownloadClientConfig(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return &ValidationError{"download client not found"}
	}
	return err
}

func normalize(req Request) (Request, error) {
	req.RemotePath = strings.TrimSpace(req.RemotePath)
	req.LocalPath = strings.TrimSpace(req.LocalPath)
	req.DownloadClientID = strings.TrimSpace(req.DownloadClientID)
	if req.RemotePath == "" || req.LocalPath == "" {
		return req, &ValidationError{"both the remote path and the local path are required"}
	}
	if !strings.HasPrefix(req.RemotePath, "/") && !isWindowsPath(req.RemotePath) {
		return req, &ValidationError{`the remote path must be absolute, like /downloads or C:\Downloads`}
	}
	if !filepath.IsAbs(req.LocalPath) {
		return req, &ValidationError{"the local path must be an absolute path on this machine"}
	}
	req.LocalPath = filepath.Clean(req.LocalPath)
	return req, nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func fromRow(r db.RemotePathMapping) Mapping {
	m := Mapping{ID: r.ID, RemotePath: r.RemotePath, LocalPath: r.LocalPath}
	if r.DownloadClientID != nil {
		m.DownloadClientID = *r.DownloadClientID
	}
	return m
}
