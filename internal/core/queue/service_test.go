// queue.PollAndUpdate is the bridge between download-client status
// and Pilot's grab_history. Bugs here surface as wrong-state queue UI
// (downloads stuck "queued" forever, or completing without firing
// TypeDownloadDone so the importer never runs). The whole package
// was at 0% coverage before this file.

package queue

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ── mocks ────────────────────────────────────────────────────────────────────

// mockQuerier embeds db.Querier so only the methods PollAndUpdate
// touches need real implementations. Records every call so tests can
// assert on what the SUT did, not just the public outcome.
type mockQuerier struct {
	db.Querier

	activeGrabs   []db.GrabHistory
	statusUpdates []db.UpdateGrabStatusParams

	mu sync.Mutex
}

func (m *mockQuerier) ListActiveGrabs(_ context.Context) ([]db.GrabHistory, error) {
	return m.activeGrabs, nil
}

func (m *mockQuerier) UpdateGrabStatus(_ context.Context, p db.UpdateGrabStatusParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusUpdates = append(m.statusUpdates, p)
	return nil
}

func (m *mockQuerier) updates() []db.UpdateGrabStatusParams {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]db.UpdateGrabStatusParams, len(m.statusUpdates))
	copy(out, m.statusUpdates)
	return out
}

// mockDownloader returns a hard-coded plugin.DownloadClient for any
// configID. The underlying client returns the configured Status for
// any ClientItemID.
type mockDownloader struct {
	client *mockClient
	err    error
}

func (m *mockDownloader) ClientFor(_ context.Context, _ string) (plugin.DownloadClient, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.client, nil
}

type mockClient struct {
	statusByItemID map[string]plugin.QueueItem
	statusErr      error
}

func (m *mockClient) Name() string              { return "mock" }
func (m *mockClient) Protocol() plugin.Protocol { return plugin.ProtocolTorrent }

func (m *mockClient) Add(_ context.Context, _ plugin.Release) (string, error) {
	return "", nil
}

func (m *mockClient) Status(_ context.Context, itemID string) (plugin.QueueItem, error) {
	if m.statusErr != nil {
		return plugin.QueueItem{}, m.statusErr
	}
	if item, ok := m.statusByItemID[itemID]; ok {
		return item, nil
	}
	return plugin.QueueItem{}, errors.New("not found")
}

func (m *mockClient) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockClient) Test(_ context.Context) error                     { return nil }
func (m *mockClient) GetQueue(_ context.Context) ([]plugin.QueueItem, error) {
	return nil, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// activeGrab returns a GrabHistory with the minimum fields required to
// reach the pollClient → status-update branches.
func activeGrab(id, clientItemID, currentStatus string) db.GrabHistory {
	return db.GrabHistory{
		ID:               id,
		SeriesID:         "series-1",
		ReleaseTitle:     "Test Release " + id,
		Protocol:         "torrent",
		DownloadStatus:   currentStatus,
		DownloadClientID: sql.NullString{String: "client-1", Valid: true},
		ClientItemID:     sql.NullString{String: clientItemID, Valid: true},
		GrabbedAt:        time.Now().UTC().Format(time.RFC3339),
	}
}

// ── PollAndUpdate ────────────────────────────────────────────────────────────

// No active grabs → no-op. Important: the SUT short-circuits without
// calling ClientFor, so a misbehaving downloader during periods with
// no active downloads can't crash the poller.
func TestPollAndUpdate_NoActiveGrabsIsNoOp(t *testing.T) {
	q := &mockQuerier{} // empty activeGrabs
	dl := &mockDownloader{
		err: errors.New("ClientFor must NOT be called when there are no active grabs"),
	}
	bus := events.New(discardLogger())
	svc := NewService(q, dl, bus, discardLogger())

	if err := svc.PollAndUpdate(context.Background()); err != nil {
		t.Fatalf("PollAndUpdate: %v", err)
	}
	if len(q.updates()) != 0 {
		t.Errorf("expected zero updates, got %d", len(q.updates()))
	}
}

// Grabs missing DownloadClientID or ClientItemID must NOT be polled
// — pre-grab rows that haven't reached the download client yet would
// otherwise fail in pollClient when ClientItemID is empty.
func TestPollAndUpdate_GrabsWithoutClientIDsAreSkipped(t *testing.T) {
	q := &mockQuerier{
		activeGrabs: []db.GrabHistory{
			{ID: "no-client", SeriesID: "s", ReleaseTitle: "x", Protocol: "torrent",
				DownloadStatus: "queued"}, // no DownloadClientID, no ClientItemID
		},
	}
	dl := &mockDownloader{
		err: errors.New("ClientFor must NOT be called for grabs missing client wiring"),
	}
	svc := NewService(q, dl, events.New(discardLogger()), discardLogger())

	if err := svc.PollAndUpdate(context.Background()); err != nil {
		t.Fatalf("PollAndUpdate: %v", err)
	}
	if len(q.updates()) != 0 {
		t.Errorf("expected zero updates for no-client grab, got %d", len(q.updates()))
	}
}

// Status unchanged AND downloaded bytes unchanged → no UpdateGrabStatus
// call. This is the "polling is cheap" optimisation; if it regresses
// every poll cycle writes a no-op row to the DB, multiplying load.
func TestPollAndUpdate_NoChangeSkipsDBWrite(t *testing.T) {
	q := &mockQuerier{
		activeGrabs: []db.GrabHistory{activeGrab("g1", "item-1", "downloading")},
	}
	dl := &mockDownloader{
		client: &mockClient{statusByItemID: map[string]plugin.QueueItem{
			"item-1": {ClientItemID: "item-1", Status: plugin.StatusDownloading, Downloaded: 0},
		}},
	}
	svc := NewService(q, dl, events.New(discardLogger()), discardLogger())

	if err := svc.PollAndUpdate(context.Background()); err != nil {
		t.Fatalf("PollAndUpdate: %v", err)
	}
	if got := len(q.updates()); got != 0 {
		t.Errorf("expected zero status updates when nothing changed; got %d (%+v)", got, q.updates())
	}
}

// Status transitions queued→downloading must persist via
// UpdateGrabStatus. Verified: the param shape carries grab ID + new
// status + downloaded bytes (the queue UI's progress bar reads this).
func TestPollAndUpdate_StatusChangePersists(t *testing.T) {
	q := &mockQuerier{
		activeGrabs: []db.GrabHistory{activeGrab("g1", "item-1", "queued")},
	}
	dl := &mockDownloader{
		client: &mockClient{statusByItemID: map[string]plugin.QueueItem{
			"item-1": {ClientItemID: "item-1", Status: plugin.StatusDownloading, Downloaded: 12345},
		}},
	}
	svc := NewService(q, dl, events.New(discardLogger()), discardLogger())

	if err := svc.PollAndUpdate(context.Background()); err != nil {
		t.Fatalf("PollAndUpdate: %v", err)
	}
	updates := q.updates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 status update, got %d", len(updates))
	}
	u := updates[0]
	if u.ID != "g1" {
		t.Errorf("update.ID = %q, want %q", u.ID, "g1")
	}
	if u.DownloadStatus != "downloading" {
		t.Errorf("update.DownloadStatus = %q, want %q", u.DownloadStatus, "downloading")
	}
	if u.DownloadedBytes != 12345 {
		t.Errorf("update.DownloadedBytes = %d, want 12345", u.DownloadedBytes)
	}
}

// Headline test: a transition to StatusCompleted MUST fire
// TypeDownloadDone with the grab_id and content_path the importer
// reads. If this event is dropped, completed downloads silently
// don't get imported — the user's library stays empty despite a
// 100% progress bar.
func TestPollAndUpdate_CompletedFiresDownloadDoneEvent(t *testing.T) {
	q := &mockQuerier{
		activeGrabs: []db.GrabHistory{activeGrab("g1", "item-1", "downloading")},
	}
	dl := &mockDownloader{
		client: &mockClient{statusByItemID: map[string]plugin.QueueItem{
			"item-1": {
				ClientItemID: "item-1",
				Status:       plugin.StatusCompleted,
				Downloaded:   1_000_000,
				ContentPath:  "/downloads/test.mkv",
			},
		}},
	}
	bus := events.New(discardLogger())
	var got atomic.Pointer[events.Event]
	bus.Subscribe(func(_ context.Context, e events.Event) {
		if e.Type == events.TypeDownloadDone {
			ev := e
			got.Store(&ev)
		}
	})
	svc := NewService(q, dl, bus, discardLogger())

	if err := svc.PollAndUpdate(context.Background()); err != nil {
		t.Fatalf("PollAndUpdate: %v", err)
	}

	// Event handlers run in goroutines per Pulse's bus contract; give
	// them a tick to land.
	time.Sleep(20 * time.Millisecond)

	ev := got.Load()
	if ev == nil {
		t.Fatal("expected TypeDownloadDone event, got none — completed downloads will silently fail to import")
	}
	if ev.ShowID != "series-1" {
		t.Errorf("event.ShowID = %q, want %q", ev.ShowID, "series-1")
	}
	if grabID, _ := ev.Data["grab_id"].(string); grabID != "g1" {
		t.Errorf("event.Data[grab_id] = %v, want g1", ev.Data["grab_id"])
	}
	if cp, _ := ev.Data["content_path"].(string); cp != "/downloads/test.mkv" {
		t.Errorf("event.Data[content_path] = %v, want /downloads/test.mkv", ev.Data["content_path"])
	}
}

// Failed transitions must NOT fire TypeDownloadDone — the importer
// would otherwise try to import a failed download (probably 0 bytes
// or partial) and produce a corrupted library entry.
func TestPollAndUpdate_FailedDoesNotFireDownloadDone(t *testing.T) {
	q := &mockQuerier{
		activeGrabs: []db.GrabHistory{activeGrab("g1", "item-1", "downloading")},
	}
	dl := &mockDownloader{
		client: &mockClient{statusByItemID: map[string]plugin.QueueItem{
			"item-1": {ClientItemID: "item-1", Status: plugin.StatusFailed},
		}},
	}
	bus := events.New(discardLogger())
	var sawDone atomic.Bool
	bus.Subscribe(func(_ context.Context, e events.Event) {
		if e.Type == events.TypeDownloadDone {
			sawDone.Store(true)
		}
	})
	svc := NewService(q, dl, bus, discardLogger())

	if err := svc.PollAndUpdate(context.Background()); err != nil {
		t.Fatalf("PollAndUpdate: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	if sawDone.Load() {
		t.Error("TypeDownloadDone fired on a failed transition — the importer would try to import a failed download")
	}
	// But the status WAS updated.
	if len(q.updates()) != 1 || q.updates()[0].DownloadStatus != "failed" {
		t.Errorf("expected the failed status to be persisted; updates = %+v", q.updates())
	}
}

// A client that errors on Status should not stop the poll loop —
// other items with the same client must still be checked. Without
// this guarantee one bad item would freeze the entire queue.
func TestPollAndUpdate_StatusErrorIsLoggedAndContinues(t *testing.T) {
	q := &mockQuerier{
		activeGrabs: []db.GrabHistory{
			activeGrab("g-bad", "bad-item", "downloading"),
			activeGrab("g-good", "good-item", "downloading"),
		},
	}
	dl := &mockDownloader{
		client: &mockClient{statusByItemID: map[string]plugin.QueueItem{
			// bad-item missing → mockClient.Status returns "not found"
			"good-item": {ClientItemID: "good-item", Status: plugin.StatusCompleted, Downloaded: 999, ContentPath: "/c"},
		}},
	}
	svc := NewService(q, dl, events.New(discardLogger()), discardLogger())

	if err := svc.PollAndUpdate(context.Background()); err != nil {
		t.Fatalf("PollAndUpdate: %v", err)
	}
	updates := q.updates()
	if len(updates) != 1 {
		t.Fatalf("expected 1 update (good-item only); got %d (%+v)", len(updates), updates)
	}
	if updates[0].ID != "g-good" {
		t.Errorf("updated wrong grab: got %q, want g-good", updates[0].ID)
	}
}
