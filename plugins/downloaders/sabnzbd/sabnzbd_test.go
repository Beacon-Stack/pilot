package sabnzbd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beacon-stack/pilot/pkg/plugin"
	"github.com/beacon-stack/pilot/plugins/downloaders/sabnzbd"
)

func newTestServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")
		name := r.URL.Query().Get("name")
		// For queue/history modes, "name" is a sub-command (e.g. "delete").
		// For addurl, "name" is the NZB URL — don't include it in the key.
		key := mode
		if (mode == "queue" || mode == "history") && name != "" {
			key = mode + "/" + name
		}
		h, ok := handlers[key]
		if !ok {
			http.Error(w, "unknown mode: "+key, http.StatusNotFound)
			return
		}
		h(w, r)
	}))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestTest_Success(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"version": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]string{"version": "4.3.1"})
		},
		"queue": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{"queue": map[string]any{"slots": []any{}}})
		},
	})
	defer srv.Close()

	c := sabnzbd.NewWithHTTPClient(sabnzbd.Config{URL: srv.URL, APIKey: "test-key"}, &http.Client{})
	if err := c.Test(context.Background()); err != nil {
		t.Fatalf("Test() error: %v", err)
	}
}

func TestTest_EmptyVersion(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"version": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]string{"version": ""})
		},
	})
	defer srv.Close()

	c := sabnzbd.NewWithHTTPClient(sabnzbd.Config{URL: srv.URL, APIKey: "key"}, &http.Client{})
	if err := c.Test(context.Background()); err == nil {
		t.Fatal("expected error for empty version, got nil")
	}
}

func TestAdd(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"addurl": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("name") == "" {
				writeJSON(w, map[string]any{"status": false, "error": "missing name"})
				return
			}
			writeJSON(w, map[string]any{"status": true, "nzo_ids": []string{"SABnzbd_nzo_abc123"}})
		},
	})
	defer srv.Close()

	c := sabnzbd.NewWithHTTPClient(sabnzbd.Config{URL: srv.URL, APIKey: "key"}, &http.Client{})
	id, err := c.Add(context.Background(), plugin.Release{
		DownloadURL: "https://example.com/release.nzb",
		Title:       "Movie.2024.1080p",
	})
	if err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if id != "SABnzbd_nzo_abc123" {
		t.Errorf("expected nzo_id SABnzbd_nzo_abc123, got %q", id)
	}
}

func TestAdd_Failure(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"addurl": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{"status": false, "error": "bad url"})
		},
	})
	defer srv.Close()

	c := sabnzbd.NewWithHTTPClient(sabnzbd.Config{URL: srv.URL, APIKey: "key"}, &http.Client{})
	_, err := c.Add(context.Background(), plugin.Release{DownloadURL: "bad"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetQueue(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"queue": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{
				"queue": map[string]any{
					"slots": []map[string]any{
						{"nzo_id": "nzo_1", "filename": "Movie.2024", "status": "Downloading", "mb": "1024.00", "mbleft": "512.00", "percentage": "50"},
						{"nzo_id": "nzo_2", "filename": "Film.2023", "status": "Queued", "mb": "2048.00", "mbleft": "2048.00", "percentage": "0"},
					},
				},
			})
		},
		"history": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{
				"history": map[string]any{
					"slots": []map[string]any{
						{"nzo_id": "nzo_3", "name": "Old.Movie", "status": "Completed", "bytes": int64(5000000), "downloaded": int64(5000000), "storage": "/downloads/Old.Movie", "fail_message": "", "completed": int64(1700000000)},
					},
				},
			})
		},
	})
	defer srv.Close()

	c := sabnzbd.NewWithHTTPClient(sabnzbd.Config{URL: srv.URL, APIKey: "key"}, &http.Client{})
	items, err := c.GetQueue(context.Background())
	if err != nil {
		t.Fatalf("GetQueue() error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items (2 queue + 1 history), got %d", len(items))
	}

	// First item: downloading
	if items[0].ClientItemID != "nzo_1" {
		t.Errorf("expected nzo_1, got %q", items[0].ClientItemID)
	}
	if items[0].Status != plugin.StatusDownloading {
		t.Errorf("expected StatusDownloading, got %v", items[0].Status)
	}
	// Size should be ~1024 MB in bytes
	if items[0].Size < 1000000000 {
		t.Errorf("expected ~1 GB size, got %d", items[0].Size)
	}

	// Second item: queued
	if items[1].Status != plugin.StatusQueued {
		t.Errorf("expected StatusQueued, got %v", items[1].Status)
	}

	// Third item: completed from history
	if items[2].Status != plugin.StatusCompleted {
		t.Errorf("expected StatusCompleted, got %v", items[2].Status)
	}
	if items[2].ContentPath != "/downloads/Old.Movie" {
		t.Errorf("expected content path from storage, got %q", items[2].ContentPath)
	}
}

func TestStatus_InHistory(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"queue": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{"queue": map[string]any{"slots": []any{}}})
		},
		"history": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{
				"history": map[string]any{
					"slots": []map[string]any{
						{"nzo_id": "nzo_done", "name": "Completed.Movie", "status": "Completed", "bytes": int64(1000), "downloaded": int64(1000), "storage": "/dl/done", "fail_message": "", "completed": int64(1700000000)},
					},
				},
			})
		},
	})
	defer srv.Close()

	c := sabnzbd.NewWithHTTPClient(sabnzbd.Config{URL: srv.URL, APIKey: "key"}, &http.Client{})
	item, err := c.Status(context.Background(), "nzo_done")
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if item.Status != plugin.StatusCompleted {
		t.Errorf("expected StatusCompleted, got %v", item.Status)
	}
}

func TestStatus_NotFound(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"queue": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{"queue": map[string]any{"slots": []any{}}})
		},
		"history": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{"history": map[string]any{"slots": []any{}}})
		},
	})
	defer srv.Close()

	c := sabnzbd.NewWithHTTPClient(sabnzbd.Config{URL: srv.URL, APIKey: "key"}, &http.Client{})
	_, err := c.Status(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing item, got nil")
	}
}

func TestRemove_FromQueue(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"queue/delete": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("value") != "nzo_1" {
				writeJSON(w, map[string]bool{"status": false})
				return
			}
			writeJSON(w, map[string]any{"status": true, "nzo_ids": []string{"nzo_1"}})
		},
	})
	defer srv.Close()

	c := sabnzbd.NewWithHTTPClient(sabnzbd.Config{URL: srv.URL, APIKey: "key"}, &http.Client{})
	if err := c.Remove(context.Background(), "nzo_1", true); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
}

func TestRemove_FallbackToHistory(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"queue/delete": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]bool{"status": false})
		},
		"history/delete": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, map[string]any{"status": true, "nzo_ids": []string{"nzo_old"}})
		},
	})
	defer srv.Close()

	c := sabnzbd.NewWithHTTPClient(sabnzbd.Config{URL: srv.URL, APIKey: "key"}, &http.Client{})
	if err := c.Remove(context.Background(), "nzo_old", false); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
}

func TestQueueStatusMapping(t *testing.T) {
	tests := []struct {
		status   string
		expected plugin.DownloadStatus
	}{
		{"Downloading", plugin.StatusDownloading},
		{"Queued", plugin.StatusQueued},
		{"Paused", plugin.StatusPaused},
		{"Fetching", plugin.StatusQueued},
		{"Grabbing", plugin.StatusQueued},
		{"Propagating", plugin.StatusQueued},
		{"Checking", plugin.StatusQueued},
	}

	for _, tc := range tests {
		t.Run("queue_"+tc.status, func(t *testing.T) {
			srv := newTestServer(t, map[string]http.HandlerFunc{
				"queue": func(w http.ResponseWriter, _ *http.Request) {
					writeJSON(w, map[string]any{
						"queue": map[string]any{
							"slots": []map[string]any{
								{"nzo_id": "nzo_x", "filename": "n", "status": tc.status, "mb": "1.0", "mbleft": "0.5", "percentage": "50"},
							},
						},
					})
				},
				"history": func(w http.ResponseWriter, _ *http.Request) {
					writeJSON(w, map[string]any{"history": map[string]any{"slots": []any{}}})
				},
			})

			c := sabnzbd.NewWithHTTPClient(sabnzbd.Config{URL: srv.URL, APIKey: "key"}, &http.Client{})
			items, err := c.GetQueue(context.Background())
			srv.Close()

			if err != nil {
				t.Fatalf("GetQueue() error: %v", err)
			}
			if len(items) == 0 {
				t.Fatal("expected 1 item")
			}
			if items[0].Status != tc.expected {
				t.Errorf("status %q: expected %v, got %v", tc.status, tc.expected, items[0].Status)
			}
		})
	}
}

func TestHistoryStatusMapping(t *testing.T) {
	tests := []struct {
		status   string
		expected plugin.DownloadStatus
	}{
		{"Completed", plugin.StatusCompleted},
		{"Failed", plugin.StatusFailed},
		{"Extracting", plugin.StatusDownloading},
		{"Verifying", plugin.StatusDownloading},
		{"Repairing", plugin.StatusDownloading},
	}

	for _, tc := range tests {
		t.Run("history_"+tc.status, func(t *testing.T) {
			srv := newTestServer(t, map[string]http.HandlerFunc{
				"queue": func(w http.ResponseWriter, _ *http.Request) {
					writeJSON(w, map[string]any{"queue": map[string]any{"slots": []any{}}})
				},
				"history": func(w http.ResponseWriter, _ *http.Request) {
					writeJSON(w, map[string]any{
						"history": map[string]any{
							"slots": []map[string]any{
								{"nzo_id": "nzo_h", "name": "n", "status": tc.status, "bytes": int64(0), "downloaded": int64(0), "storage": "", "fail_message": "", "completed": int64(0)},
							},
						},
					})
				},
			})

			c := sabnzbd.NewWithHTTPClient(sabnzbd.Config{URL: srv.URL, APIKey: "key"}, &http.Client{})
			items, err := c.GetQueue(context.Background())
			srv.Close()

			if err != nil {
				t.Fatalf("GetQueue() error: %v", err)
			}
			if len(items) == 0 {
				t.Fatal("expected 1 item from history")
			}
			if items[0].Status != tc.expected {
				t.Errorf("history status %q: expected %v, got %v", tc.status, tc.expected, items[0].Status)
			}
		})
	}
}

func TestFactoryValidation(t *testing.T) {
	c := sabnzbd.New(sabnzbd.Config{URL: "http://localhost", APIKey: "key"})
	if c.Name() != "SABnzbd" {
		t.Errorf("expected name SABnzbd, got %q", c.Name())
	}
	if c.Protocol() != plugin.ProtocolNZB {
		t.Errorf("expected ProtocolNZB, got %v", c.Protocol())
	}
}
