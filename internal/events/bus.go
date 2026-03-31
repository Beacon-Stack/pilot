package events

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Type identifies what happened.
type Type string

const (
	TypeShowAdded      Type = "show_added"
	TypeShowDeleted    Type = "show_deleted"
	TypeShowUpdated    Type = "show_updated"
	TypeEpisodeGrabbed Type = "episode_grabbed"
	TypeGrabFailed     Type = "grab_failed"
	TypeDownloadDone   Type = "download_done"
	TypeImportComplete Type = "import_complete"
	TypeImportFailed   Type = "import_failed"
	TypeHealthIssue    Type = "health_issue"
	TypeHealthOK       Type = "health_ok"
	TypeTaskStarted    Type = "task_started"
	TypeTaskFinished   Type = "task_finished"
)

// Event carries the context of something that happened.
type Event struct {
	Type      Type           `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	ShowID    string         `json:"show_id,omitempty"`
	EpisodeID string         `json:"episode_id,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// Handler is a function that receives events.
type Handler func(ctx context.Context, e Event)

// Bus is a simple in-process publish/subscribe event bus.
type Bus struct {
	mu       sync.RWMutex
	handlers []Handler
	logger   *slog.Logger
}

// New creates a new Bus.
func New(logger *slog.Logger) *Bus {
	return &Bus{logger: logger}
}

// Subscribe registers a handler to receive all future events.
func (b *Bus) Subscribe(h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, h)
}

// Publish sends an event to all registered handlers asynchronously.
func (b *Bus) Publish(ctx context.Context, e Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}

	handlerCtx := context.WithoutCancel(ctx)

	b.mu.RLock()
	handlers := make([]Handler, len(b.handlers))
	copy(handlers, b.handlers)
	b.mu.RUnlock()

	for _, h := range handlers {
		h := h
		go func() {
			defer func() {
				if r := recover(); r != nil {
					b.logger.Error("event handler panicked",
						"event_type", e.Type,
						"panic", r,
					)
				}
			}()
			h(handlerCtx, e)
		}()
	}
}
