package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// struggleTimeout is how long an interactive search waits on an indexer
// that keeps missing (see indexerHealth) — typically a Cloudflare-walled
// site whose challenge FlareSolverr often can't solve. Its request still
// goes out: Pulse keeps scraping after we stop waiting and caches what it
// finds, and the search modal's automatic re-search picks that up. What
// changes is that everyone else's results don't sit behind it for 25s.
// A var so tests can shorten it.
var struggleTimeout = 6 * time.Second

// indexerHealth remembers each indexer's last few search outcomes.
// An indexer that missed (timed out or failed) struggleMisses of its
// last healthWindow searches is "struggling". A window rather than a
// streak, because a struggling indexer's cached answers come back fast
// and would otherwise reset a streak between every two cold misses.
type indexerHealth struct {
	mu     sync.Mutex
	recent map[string][]bool // true = miss, oldest first
}

const (
	healthWindow   = 3
	struggleMisses = 2
)

func (h *indexerHealth) record(id string, miss bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.recent == nil {
		h.recent = make(map[string][]bool)
	}
	r := append(h.recent[id], miss)
	if len(r) > healthWindow {
		r = r[len(r)-healthWindow:]
	}
	h.recent[id] = r
}

// snapshot copies the recent outcomes, for saving.
func (h *indexerHealth) snapshot() map[string][]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string][]bool, len(h.recent))
	for id, r := range h.recent {
		out[id] = append([]bool(nil), r...)
	}
	return out
}

// restore replaces the recent outcomes with saved ones.
func (h *indexerHealth) restore(saved map[string][]bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recent = make(map[string][]bool, len(saved))
	for id, r := range saved {
		if len(r) > healthWindow {
			r = r[len(r)-healthWindow:]
		}
		h.recent[id] = append([]bool(nil), r...)
	}
}

// healthSettingKey is where indexer health is saved in the settings
// table, so a restart doesn't forget which indexers keep missing and
// give them the full timeout again.
const healthSettingKey = "indexer_health"

// LoadHealth restores indexer health saved by an earlier run. Nothing
// saved, or something unreadable, starts fresh.
func (s *Service) LoadHealth(ctx context.Context) error {
	if s.q == nil {
		return nil
	}
	raw, err := s.q.GetSetting(ctx, healthSettingKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("loading indexer health: %w", err)
	}
	var saved map[string][]bool
	if err := json.Unmarshal([]byte(raw), &saved); err != nil {
		return fmt.Errorf("reading saved indexer health: %w", err)
	}
	s.health.restore(saved)
	return nil
}

// saveHealth writes the current indexer health. Called in the background
// after each search; saves are serialized so an older snapshot can't
// land after a newer one.
func (s *Service) saveHealth() {
	if s.q == nil {
		return
	}
	s.healthSaveMu.Lock()
	defer s.healthSaveMu.Unlock()
	b, err := json.Marshal(s.health.snapshot())
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.q.SetSetting(ctx, db.SetSettingParams{
		Key:       healthSettingKey,
		Value:     string(b),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil && s.logger != nil {
		s.logger.Warn("could not save indexer health", "error", err)
	}
}

func (h *indexerHealth) struggling(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	misses := 0
	for _, miss := range h.recent[id] {
		if miss {
			misses++
		}
	}
	return misses >= struggleMisses
}
