// Package delay implements the delay profile (as in Sonarr): which
// download protocols are used, which is preferred, and how long to wait
// after a release is published before grabbing it automatically, so a
// better release has time to show up.
//
// The delay applies to automatic grabs from RSS sync. A release still
// inside its delay is stored as pending and grabbed later if it is still
// the best wanted release. Searches the user triggers are not delayed.
package delay

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/beacon-stack/pilot/internal/core/quality"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// settingKey is where the profile is stored in the settings table.
const settingKey = "delay_profile"

// MaxDelayMinutes caps a delay at one week.
const MaxDelayMinutes = 7 * 24 * 60

// Profile is the delay profile.
type Profile struct {
	UsenetEnabled  bool `json:"usenet_enabled"`
	TorrentEnabled bool `json:"torrent_enabled"`
	// PreferredProtocol is "usenet" or "torrent". It breaks ties between
	// releases of equal quality and revision.
	PreferredProtocol   string `json:"preferred_protocol"`
	UsenetDelayMinutes  int    `json:"usenet_delay_minutes"`
	TorrentDelayMinutes int    `json:"torrent_delay_minutes"`
	// BypassIfHighestQuality grabs immediately when the release is the
	// best quality the series' profile allows.
	BypassIfHighestQuality bool `json:"bypass_if_highest_quality"`
}

// Default is the profile used until one is saved: both protocols, prefer
// usenet, no delay. With no delay, behavior is the same as before delay
// profiles existed.
func Default() Profile {
	return Profile{
		UsenetEnabled:          true,
		TorrentEnabled:         true,
		PreferredProtocol:      "usenet",
		BypassIfHighestQuality: true,
	}
}

// ValidationError explains why a profile can't be saved.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// Validate checks p.
func (p Profile) Validate() error {
	if !p.UsenetEnabled && !p.TorrentEnabled {
		return &ValidationError{"enable at least one protocol"}
	}
	switch p.PreferredProtocol {
	case "usenet", "torrent":
	default:
		return &ValidationError{`preferred protocol must be "usenet" or "torrent"`}
	}
	for _, m := range []int{p.UsenetDelayMinutes, p.TorrentDelayMinutes} {
		if m < 0 || m > MaxDelayMinutes {
			return &ValidationError{fmt.Sprintf("delays must be between 0 and %d minutes", MaxDelayMinutes)}
		}
	}
	return nil
}

func isUsenet(p plugin.Protocol) bool { return p == plugin.ProtocolNZB }

// Allows reports whether releases of this protocol may be grabbed.
func (p Profile) Allows(proto plugin.Protocol) bool {
	if isUsenet(proto) {
		return p.UsenetEnabled
	}
	return p.TorrentEnabled
}

// Delay is how long to wait after publication for this protocol.
func (p Profile) Delay(proto plugin.Protocol) time.Duration {
	if isUsenet(proto) {
		return time.Duration(p.UsenetDelayMinutes) * time.Minute
	}
	return time.Duration(p.TorrentDelayMinutes) * time.Minute
}

// Preferred reports whether proto is the preferred protocol.
func (p Profile) Preferred(proto plugin.Protocol) bool {
	if isUsenet(proto) {
		return p.PreferredProtocol == "usenet"
	}
	return p.PreferredProtocol == "torrent"
}

// EligibleAt is when a release published ageDays ago (as of now) may be
// grabbed: its publish time plus the delay for its protocol.
func (p Profile) EligibleAt(proto plugin.Protocol, ageDays float64, now time.Time) time.Time {
	published := now.Add(-time.Duration(ageDays * float64(24*time.Hour)))
	return published.Add(p.Delay(proto))
}

// Bypasses reports whether a release of quality q skips the delay: the
// option is on and q is the best quality the profile allows (resolution
// and source; the cutoff when the profile lists no qualities).
func (p Profile) Bypasses(q plugin.Quality, prof *quality.Profile) bool {
	if !p.BypassIfHighestQuality || prof == nil {
		return false
	}
	best := prof.Cutoff.Score()
	for _, a := range prof.Qualities {
		if s := a.Score(); s > best {
			best = s
		}
	}
	if best == 0 {
		return false
	}
	return q.Score()/10 >= best/10
}

// Store loads and saves the profile in the settings table.
type Store struct {
	q   db.Querier
	now func() time.Time
}

// NewStore creates a Store.
func NewStore(q db.Querier) *Store { return &Store{q: q, now: time.Now} }

// Get returns the saved profile, or Default() when none is saved.
func (s *Store) Get(ctx context.Context) (Profile, error) {
	raw, err := s.q.GetSetting(ctx, settingKey)
	if errors.Is(err, sql.ErrNoRows) {
		return Default(), nil
	}
	if err != nil {
		return Profile{}, err
	}
	p := Default()
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Profile{}, fmt.Errorf("decoding delay profile: %w", err)
	}
	return p, nil
}

// Set validates and saves p.
func (s *Store) Set(ctx context.Context, p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.q.SetSetting(ctx, db.SetSettingParams{
		Key:       settingKey,
		Value:     string(b),
		UpdatedAt: s.now().UTC().Format(time.RFC3339),
	})
}
