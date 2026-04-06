package nzbget_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beacon-media/pilot/pkg/plugin"
	"github.com/beacon-media/pilot/plugins/downloaders/nzbget"
)

type rpcHandler struct {
	handlers map[string]func(params json.RawMessage) any
}

func newRPCHandler() *rpcHandler {
	return &rpcHandler{handlers: make(map[string]func(params json.RawMessage) any)}
}

func (h *rpcHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		ID     int64           `json:"id"`
	}
	_ = json.Unmarshal(body, &req)

	handler, ok := h.handlers[req.Method]
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": "1.1",
			"error":   map[string]any{"code": -1, "message": "method not found: " + req.Method},
			"id":      req.ID,
		})
		return
	}

	result := handler(req.Params)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"version": "1.1",
		"result":  result,
		"id":      req.ID,
	})
}

func newClient(t *testing.T, h *rpcHandler) (*nzbget.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	c := nzbget.NewWithHTTPClient(
		nzbget.Config{URL: srv.URL, Username: "admin", Password: "pass"},
		&http.Client{},
	)
	return c, srv
}

func TestTest_Success(t *testing.T) {
	h := newRPCHandler()
	h.handlers["version"] = func(_ json.RawMessage) any { return "21.1" }
	c, srv := newClient(t, h)
	defer srv.Close()

	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test() error: %v", err)
	}
}

func TestTest_AuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := nzbget.NewWithHTTPClient(
		nzbget.Config{URL: srv.URL, Username: "bad", Password: "bad"},
		&http.Client{},
	)
	if err := c.Test(context.Background()); err == nil {
		t.Fatal("expected auth failure error, got nil")
	}
}

func TestAdd(t *testing.T) {
	h := newRPCHandler()
	h.handlers["append"] = func(_ json.RawMessage) any { return 12345 }
	c, srv := newClient(t, h)
	defer srv.Close()

	id, err := c.Add(context.Background(), plugin.Release{
		DownloadURL: "https://example.com/release.nzb",
		Title:       "Movie.2024.1080p",
	})
	if err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if id != "12345" {
		t.Errorf("expected NZBID 12345, got %q", id)
	}
}

func TestAdd_InvalidID(t *testing.T) {
	h := newRPCHandler()
	h.handlers["append"] = func(_ json.RawMessage) any { return 0 }
	c, srv := newClient(t, h)
	defer srv.Close()

	_, err := c.Add(context.Background(), plugin.Release{DownloadURL: "https://example.com/bad.nzb"})
	if err == nil {
		t.Fatal("expected error for NZBID 0, got nil")
	}
}

func TestGetQueue(t *testing.T) {
	h := newRPCHandler()
	h.handlers["listgroups"] = func(_ json.RawMessage) any {
		return []map[string]any{
			{"NZBID": 1, "NZBName": "Movie.2024", "Status": "DOWNLOADING", "FileSizeMB": 1024, "RemainingSizeMB": 512, "DestDir": "/downloads/Movie.2024", "Category": "movies"},
			{"NZBID": 2, "NZBName": "Film.2023", "Status": "QUEUED", "FileSizeMB": 2048, "RemainingSizeMB": 2048, "DestDir": "/downloads/Film.2023", "Category": ""},
		}
	}
	h.handlers["history"] = func(_ json.RawMessage) any {
		return []map[string]any{
			{"NZBID": 3, "Name": "Old.Movie", "Status": "SUCCESS/ALL", "FileSizeMB": 500, "DownloadedSizeMB": 500, "DestDir": "/inter/Old.Movie", "FinalDir": "/downloads/Old.Movie", "HistoryTime": 1700000000, "Category": "movies"},
		}
	}
	c, srv := newClient(t, h)
	defer srv.Close()

	items, err := c.GetQueue(context.Background())
	if err != nil {
		t.Fatalf("GetQueue() error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items (2 queue + 1 history), got %d", len(items))
	}

	// Item 1: downloading
	if items[0].ClientItemID != "1" {
		t.Errorf("expected ID 1, got %q", items[0].ClientItemID)
	}
	if items[0].Status != plugin.StatusDownloading {
		t.Errorf("expected StatusDownloading, got %v", items[0].Status)
	}
	if items[0].Downloaded != 512*1024*1024 {
		t.Errorf("expected downloaded ~512 MB, got %d", items[0].Downloaded)
	}

	// Item 2: queued
	if items[1].Status != plugin.StatusQueued {
		t.Errorf("expected StatusQueued, got %v", items[1].Status)
	}

	// Item 3: completed from history, FinalDir preferred
	if items[2].Status != plugin.StatusCompleted {
		t.Errorf("expected StatusCompleted, got %v", items[2].Status)
	}
	if items[2].ContentPath != "/downloads/Old.Movie" {
		t.Errorf("expected FinalDir as content path, got %q", items[2].ContentPath)
	}
}

func TestStatus_InHistory(t *testing.T) {
	h := newRPCHandler()
	h.handlers["listgroups"] = func(_ json.RawMessage) any { return []any{} }
	h.handlers["history"] = func(_ json.RawMessage) any {
		return []map[string]any{
			{"NZBID": 99, "Name": "Done", "Status": "SUCCESS/GOOD", "FileSizeMB": 100, "DownloadedSizeMB": 100, "DestDir": "/dl", "FinalDir": "/final", "HistoryTime": 1700000000, "Category": ""},
		}
	}
	c, srv := newClient(t, h)
	defer srv.Close()

	item, err := c.Status(context.Background(), "99")
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if item.Status != plugin.StatusCompleted {
		t.Errorf("expected StatusCompleted, got %v", item.Status)
	}
}

func TestStatus_NotFound(t *testing.T) {
	h := newRPCHandler()
	h.handlers["listgroups"] = func(_ json.RawMessage) any { return []any{} }
	h.handlers["history"] = func(_ json.RawMessage) any { return []any{} }
	c, srv := newClient(t, h)
	defer srv.Close()

	_, err := c.Status(context.Background(), "999")
	if err == nil {
		t.Fatal("expected error for missing item, got nil")
	}
}

func TestRemove(t *testing.T) {
	h := newRPCHandler()
	h.handlers["editqueue"] = func(_ json.RawMessage) any { return true }
	c, srv := newClient(t, h)
	defer srv.Close()

	if err := c.Remove(context.Background(), "42", true); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
}

func TestGroupStatusMapping(t *testing.T) {
	tests := []struct {
		status   string
		expected plugin.DownloadStatus
	}{
		{"DOWNLOADING", plugin.StatusDownloading},
		{"PAUSED", plugin.StatusPaused},
		{"QUEUED", plugin.StatusQueued},
		{"FETCHING", plugin.StatusQueued},
		{"LOADING_PARS", plugin.StatusQueued},
		{"PP_QUEUED", plugin.StatusDownloading},
		{"VERIFYING_SOURCES", plugin.StatusDownloading},
		{"REPAIRING", plugin.StatusDownloading},
		{"UNPACKING", plugin.StatusDownloading},
		{"MOVING", plugin.StatusDownloading},
		{"EXECUTING_SCRIPT", plugin.StatusDownloading},
		{"PP_FINISHED", plugin.StatusCompleted},
	}

	for _, tc := range tests {
		t.Run("group_"+tc.status, func(t *testing.T) {
			h := newRPCHandler()
			h.handlers["listgroups"] = func(_ json.RawMessage) any {
				return []map[string]any{
					{"NZBID": 1, "NZBName": "n", "Status": tc.status, "FileSizeMB": 0, "RemainingSizeMB": 0, "DestDir": "", "Category": ""},
				}
			}
			h.handlers["history"] = func(_ json.RawMessage) any { return []any{} }
			c, srv := newClient(t, h)
			defer srv.Close()

			items, err := c.GetQueue(context.Background())
			if err != nil {
				t.Fatalf("GetQueue() error: %v", err)
			}
			if len(items) == 0 {
				t.Fatal("expected 1 item")
			}
			if items[0].Status != tc.expected {
				t.Errorf("group %q: expected %v, got %v", tc.status, tc.expected, items[0].Status)
			}
		})
	}
}

func TestHistoryStatusMapping(t *testing.T) {
	tests := []struct {
		status   string
		expected plugin.DownloadStatus
	}{
		{"SUCCESS/ALL", plugin.StatusCompleted},
		{"SUCCESS/GOOD", plugin.StatusCompleted},
		{"SUCCESS/UNPACK", plugin.StatusCompleted},
		{"FAILURE/PAR", plugin.StatusFailed},
		{"FAILURE/UNPACK", plugin.StatusFailed},
		{"FAILURE/HEALTH", plugin.StatusFailed},
		{"WARNING/SCRIPT", plugin.StatusFailed},
		{"WARNING/SPACE", plugin.StatusFailed},
		{"DELETED/MANUAL", plugin.StatusFailed},
		{"DELETED/DUPE", plugin.StatusFailed},
	}

	for _, tc := range tests {
		t.Run("history_"+tc.status, func(t *testing.T) {
			h := newRPCHandler()
			h.handlers["listgroups"] = func(_ json.RawMessage) any { return []any{} }
			h.handlers["history"] = func(_ json.RawMessage) any {
				return []map[string]any{
					{"NZBID": 1, "Name": "n", "Status": tc.status, "FileSizeMB": 0, "DownloadedSizeMB": 0, "DestDir": "", "FinalDir": "", "HistoryTime": 0, "Category": ""},
				}
			}
			c, srv := newClient(t, h)
			defer srv.Close()

			items, err := c.GetQueue(context.Background())
			if err != nil {
				t.Fatalf("GetQueue() error: %v", err)
			}
			if len(items) == 0 {
				t.Fatal("expected 1 item from history")
			}
			if items[0].Status != tc.expected {
				t.Errorf("history %q: expected %v, got %v", tc.status, tc.expected, items[0].Status)
			}
		})
	}
}

func TestFactoryValidation(t *testing.T) {
	c := nzbget.New(nzbget.Config{URL: "http://localhost", Username: "u", Password: "p"})
	if c.Name() != "NZBGet" {
		t.Errorf("expected name NZBGet, got %q", c.Name())
	}
	if c.Protocol() != plugin.ProtocolNZB {
		t.Errorf("expected ProtocolNZB, got %v", c.Protocol())
	}
}

func TestInvalidNZBID(t *testing.T) {
	h := newRPCHandler()
	c, srv := newClient(t, h)
	defer srv.Close()

	_, err := c.Status(context.Background(), "not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric NZBID")
	}

	err = c.Remove(context.Background(), "not-a-number", false)
	if err == nil {
		t.Fatal("expected error for non-numeric NZBID")
	}
}
