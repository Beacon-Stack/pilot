// Package releaseprofile implements release profiles: per-indexer (or
// global) lists of terms a release title must contain or must not
// contain, as in Sonarr.
//
// A term is a case-insensitive substring, or a regular expression when
// written as /pattern/ (also case-insensitive). A release passes a
// profile when it contains at least one required term (if the profile
// has any) and none of the ignored terms. Every enabled profile that
// applies to the release's indexer must pass.
package releaseprofile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// FilterReason is the search-result filter reason for a release rejected
// by a release profile.
const FilterReason = "release_profile"

// ErrNotFound is returned when a profile ID doesn't exist.
var ErrNotFound = errors.New("release profile not found")

// Profile is a release profile.
type Profile struct {
	ID        string
	Name      string
	Enabled   bool
	Required  []string
	Ignored   []string
	IndexerID string // "" = all indexers
	CreatedAt string
	UpdatedAt string
}

// Request is the editable part of a profile.
type Request struct {
	Name      string
	Enabled   bool
	Required  []string
	Ignored   []string
	IndexerID string
}

// ValidationError explains why a Request was rejected.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// Service stores release profiles.
type Service struct {
	q   db.Querier
	now func() time.Time
}

// NewService creates a Service.
func NewService(q db.Querier) *Service {
	return &Service{q: q, now: time.Now}
}

// List returns every profile, by name.
func (s *Service) List(ctx context.Context) ([]Profile, error) {
	rows, err := s.q.ListReleaseProfiles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Profile, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out, nil
}

// Get returns one profile.
func (s *Service) Get(ctx context.Context, id string) (Profile, error) {
	row, err := s.q.GetReleaseProfile(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	return fromRow(row), nil
}

// Create validates and stores a new profile.
func (s *Service) Create(ctx context.Context, req Request) (Profile, error) {
	req, err := normalize(req)
	if err != nil {
		return Profile{}, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	row, err := s.q.CreateReleaseProfile(ctx, db.CreateReleaseProfileParams{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Enabled:      req.Enabled,
		RequiredJson: mustJSON(req.Required),
		IgnoredJson:  mustJSON(req.Ignored),
		IndexerID:    nullable(req.IndexerID),
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return Profile{}, err
	}
	return fromRow(row), nil
}

// Update validates and replaces a profile's settings.
func (s *Service) Update(ctx context.Context, id string, req Request) (Profile, error) {
	req, err := normalize(req)
	if err != nil {
		return Profile{}, err
	}
	row, err := s.q.UpdateReleaseProfile(ctx, db.UpdateReleaseProfileParams{
		ID:           id,
		Name:         req.Name,
		Enabled:      req.Enabled,
		RequiredJson: mustJSON(req.Required),
		IgnoredJson:  mustJSON(req.Ignored),
		IndexerID:    nullable(req.IndexerID),
		UpdatedAt:    s.now().UTC().Format(time.RFC3339),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	return fromRow(row), nil
}

// Delete removes a profile.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	return s.q.DeleteReleaseProfile(ctx, id)
}

// LoadMatcher compiles the enabled profiles stored in q.
func LoadMatcher(ctx context.Context, q db.Querier) (*Matcher, error) {
	rows, err := q.ListEnabledReleaseProfiles(ctx)
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(rows))
	for _, r := range rows {
		profiles = append(profiles, fromRow(r))
	}
	return NewMatcher(profiles), nil
}

// Matching

type term struct {
	raw string
	sub string         // lower-cased substring, when not a regex
	re  *regexp.Regexp // when written as /pattern/
}

func (t term) matches(lowerTitle, title string) bool {
	if t.re != nil {
		return t.re.MatchString(title)
	}
	return strings.Contains(lowerTitle, t.sub)
}

type compiled struct {
	name      string
	indexerID string
	required  []term
	ignored   []term
}

// Matcher checks release titles against a set of profiles. A nil Matcher
// accepts everything.
type Matcher struct {
	profiles []compiled
}

// NewMatcher compiles profiles. Disabled profiles are skipped, and so is
// any term that fails to compile (Create/Update reject those, so this
// only guards against rows edited outside Pilot).
func NewMatcher(profiles []Profile) *Matcher {
	m := &Matcher{}
	for _, p := range profiles {
		if !p.Enabled {
			continue
		}
		c := compiled{name: p.Name, indexerID: p.IndexerID}
		for _, raw := range p.Required {
			if t, err := compileTerm(raw); err == nil {
				c.required = append(c.required, t)
			}
		}
		for _, raw := range p.Ignored {
			if t, err := compileTerm(raw); err == nil {
				c.ignored = append(c.ignored, t)
			}
		}
		m.profiles = append(m.profiles, c)
	}
	return m
}

// Check reports whether a release with this title from this indexer
// passes every applicable profile. When it doesn't, reason says which
// profile rejected it and why.
func (m *Matcher) Check(title, indexerID string) (ok bool, reason string) {
	if m == nil {
		return true, ""
	}
	lower := strings.ToLower(title)
	for _, p := range m.profiles {
		if p.indexerID != "" && p.indexerID != indexerID {
			continue
		}
		for _, t := range p.ignored {
			if t.matches(lower, title) {
				return false, fmt.Sprintf("release profile %q: contains \"%s\"", p.name, t.raw)
			}
		}
		if len(p.required) == 0 {
			continue
		}
		found := false
		for _, t := range p.required {
			if t.matches(lower, title) {
				found = true
				break
			}
		}
		if !found {
			return false, fmt.Sprintf("release profile %q: contains none of the required terms", p.name)
		}
	}
	return true, ""
}

func compileTerm(raw string) (term, error) {
	if len(raw) >= 3 && strings.HasPrefix(raw, "/") && strings.HasSuffix(raw, "/") {
		re, err := regexp.Compile("(?i)" + raw[1:len(raw)-1])
		if err != nil {
			return term{}, err
		}
		return term{raw: raw, re: re}, nil
	}
	return term{raw: raw, sub: strings.ToLower(raw)}, nil
}

// Helpers

func normalize(req Request) (Request, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return req, &ValidationError{"name is required"}
	}
	var err error
	if req.Required, err = cleanTerms(req.Required); err != nil {
		return req, err
	}
	if req.Ignored, err = cleanTerms(req.Ignored); err != nil {
		return req, err
	}
	if len(req.Required) == 0 && len(req.Ignored) == 0 {
		return req, &ValidationError{"add at least one term to must contain or must not contain"}
	}
	req.IndexerID = strings.TrimSpace(req.IndexerID)
	return req, nil
}

func cleanTerms(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool)
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		if _, err := compileTerm(t); err != nil {
			return nil, &ValidationError{fmt.Sprintf("term %s is not a valid regular expression: %v", t, err)}
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out, nil
}

func fromRow(r db.ReleaseProfile) Profile {
	p := Profile{
		ID:        r.ID,
		Name:      r.Name,
		Enabled:   r.Enabled,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	_ = json.Unmarshal([]byte(r.RequiredJson), &p.Required)
	_ = json.Unmarshal([]byte(r.IgnoredJson), &p.Ignored)
	if r.IndexerID != nil {
		p.IndexerID = *r.IndexerID
	}
	if p.Required == nil {
		p.Required = []string{}
	}
	if p.Ignored == nil {
		p.Ignored = []string{}
	}
	return p
}

func mustJSON(v []string) string {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
