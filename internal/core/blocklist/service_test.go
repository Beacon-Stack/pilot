// Package blocklist_test — regression suite for the release blocklist.
//
// ⚠ These tests exist because the blocklist is the persistence layer for
// the dead-torrent fix. If any of these break, automated stall blocklisting
// stops working, and we're back to re-grabbing dead releases on every
// search.
//
// When to run:
//
//	go test ./internal/core/blocklist/... -v
//
// Run this before editing any of:
//
//   - internal/core/blocklist/*
//   - internal/db/queries/postgres/blocklist.sql
//   - internal/db/migrations/00003_blocklist_stall_columns.sql
package blocklist

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// mockQuerier is the same pattern as the stallwatcher test's mock — embed
// db.Querier, override only the methods blocklist.Service calls.
type mockQuerier struct {
	db.Querier

	mu                  sync.Mutex
	entriesByGUID       map[string]db.Blocklist
	recentStallsByEpKey map[string]int64
	// inject: return this error on CreateBlocklistEntry if non-nil
	createErr error
}

func newMock() *mockQuerier {
	return &mockQuerier{
		entriesByGUID:       map[string]db.Blocklist{},
		recentStallsByEpKey: map[string]int64{},
	}
}

func (m *mockQuerier) CreateBlocklistEntry(ctx context.Context, arg db.CreateBlocklistEntryParams) (db.Blocklist, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return db.Blocklist{}, m.createErr
	}
	if _, exists := m.entriesByGUID[arg.ReleaseGuid]; exists {
		return db.Blocklist{}, &pgconn.PgError{Code: "23505", Message: "unique violation"}
	}
	row := db.Blocklist(arg)
	m.entriesByGUID[arg.ReleaseGuid] = row
	return row, nil
}

//nolint:revive // method name must match sqlc-generated Querier interface
func (m *mockQuerier) IsBlocklistedByGuidOrInfoHash(ctx context.Context, arg db.IsBlocklistedByGuidOrInfoHashParams) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.entriesByGUID {
		if row.ReleaseGuid == arg.ReleaseGuid {
			return 1, nil
		}
		if arg.InfoHash.Valid && row.InfoHash.Valid && row.InfoHash.String == arg.InfoHash.String {
			return 1, nil
		}
	}
	return 0, nil
}

//nolint:revive // param name must match sqlc-generated Querier interface
func (m *mockQuerier) DeleteBlocklistEntryByGUID(ctx context.Context, releaseGuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entriesByGUID, releaseGuid)
	return nil
}

func (m *mockQuerier) CountRecentStallsForEpisode(ctx context.Context, arg db.CountRecentStallsForEpisodeParams) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := arg.SeriesID + "|" + arg.EpisodeID.String
	return m.recentStallsByEpKey[key], nil
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestAddFromStall_RoundTrip(t *testing.T) {
	mock := newMock()
	svc := NewService(mock)
	ctx := context.Background()

	err := svc.AddFromStall(ctx, StallEntry{
		SeriesID:     "series-1",
		EpisodeID:    "ep-1",
		ReleaseGUID:  "guid-a",
		ReleaseTitle: "Old.Dead.Release",
		IndexerID:    "idx-1",
		Protocol:     "torrent",
		Size:         12345,
		Notes:        "auto-blocklisted by stall watcher",
		Reason:       ReasonStallNoPeersEver,
		InfoHash:     "deadbeef00",
	})
	if err != nil {
		t.Fatalf("AddFromStall: %v", err)
	}

	// Lookup by GUID.
	blocked, err := svc.IsBlocklistedGUIDOrInfoHash(ctx, "guid-a", "")
	if err != nil {
		t.Fatalf("IsBlocklistedGUIDOrInfoHash: %v", err)
	}
	if !blocked {
		t.Error("expected guid-a to be blocklisted")
	}

	// Lookup by info_hash with a different GUID — the two-keyed dedup.
	blocked, err = svc.IsBlocklistedGUIDOrInfoHash(ctx, "different-guid", "deadbeef00")
	if err != nil {
		t.Fatalf("IsBlocklistedGUIDOrInfoHash by info_hash: %v", err)
	}
	if !blocked {
		t.Error("expected info_hash lookup to find the entry under a different GUID. " +
			"This is the 'same release from a different indexer' dedup case — if it fails, " +
			"different-GUID-same-content releases will re-grab the same dead torrent.")
	}
}

func TestAddFromStall_DuplicateIsIdempotent(t *testing.T) {
	mock := newMock()
	svc := NewService(mock)
	ctx := context.Background()

	entry := StallEntry{
		SeriesID:     "series-1",
		ReleaseGUID:  "guid-dup",
		ReleaseTitle: "test",
		Protocol:     "torrent",
		Reason:       ReasonStallNoPeersEver,
	}

	if err := svc.AddFromStall(ctx, entry); err != nil {
		t.Fatalf("first add: %v", err)
	}
	err := svc.AddFromStall(ctx, entry)
	if !errors.Is(err, ErrAlreadyBlocklisted) {
		t.Errorf("expected ErrAlreadyBlocklisted on duplicate, got: %v", err)
	}
}

func TestRemoveByGUID(t *testing.T) {
	mock := newMock()
	svc := NewService(mock)
	ctx := context.Background()

	_ = svc.AddFromStall(ctx, StallEntry{
		SeriesID:     "series-1",
		ReleaseGUID:  "guid-remove",
		ReleaseTitle: "test",
		Reason:       ReasonUserMarked,
	})

	if err := svc.RemoveByGUID(ctx, "guid-remove"); err != nil {
		t.Fatalf("RemoveByGUID: %v", err)
	}

	blocked, _ := svc.IsBlocklistedGUIDOrInfoHash(ctx, "guid-remove", "")
	if blocked {
		t.Error("expected guid-remove to be gone after RemoveByGUID")
	}
}

func TestCountRecentStalls_PerEpisode(t *testing.T) {
	mock := newMock()
	svc := NewService(mock)
	ctx := context.Background()

	// Simulate 2 stalls for (series-1, ep-1)
	mock.recentStallsByEpKey["series-1|ep-1"] = 2
	// 0 stalls for (series-1, ep-2)
	// 5 stalls for (series-2, ep-1)
	mock.recentStallsByEpKey["series-2|ep-1"] = 5

	cases := []struct {
		series, ep string
		want       int64
	}{
		{"series-1", "ep-1", 2},
		{"series-1", "ep-2", 0},
		{"series-2", "ep-1", 5},
		{"series-unknown", "ep-unknown", 0},
	}
	for _, c := range cases {
		got, err := svc.CountRecentStalls(ctx, c.series, c.ep)
		if err != nil {
			t.Errorf("CountRecentStalls(%s, %s): %v", c.series, c.ep, err)
			continue
		}
		if got != c.want {
			t.Errorf("CountRecentStalls(%s, %s) = %d, want %d", c.series, c.ep, got, c.want)
		}
	}
}

func TestAddFromStall_SetsReasonExplicitly(t *testing.T) {
	mock := newMock()
	svc := NewService(mock)

	err := svc.AddFromStall(context.Background(), StallEntry{
		SeriesID:    "s",
		ReleaseGUID: "g",
		Reason:      ReasonStallActivityLost,
	})
	if err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	row := mock.entriesByGUID["g"]
	mock.mu.Unlock()
	if row.Reason != ReasonStallActivityLost {
		t.Errorf("expected reason %q, got %q", ReasonStallActivityLost, row.Reason)
	}
}

func TestAdd_SetsUserMarkedReason(t *testing.T) {
	mock := newMock()
	svc := NewService(mock)

	err := svc.Add(context.Background(), "series-1", "ep-1", "guid", "title", "idx", "torrent", 1000, "note")
	if err != nil {
		t.Fatal(err)
	}
	mock.mu.Lock()
	row := mock.entriesByGUID["guid"]
	mock.mu.Unlock()
	if row.Reason != ReasonUserMarked {
		t.Errorf("user-facing Add should set reason = user_marked, got %q", row.Reason)
	}
}

// Compile-time: mock must satisfy db.Querier.
var _ db.Querier = (*mockQuerier)(nil)

// Silence unused-import warnings if future edits remove references.
var _ = sql.NullString{}
var _ = time.Time{}
