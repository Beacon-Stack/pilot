// Package notifications connects Pilot to the shared Beacon notifications
// (pulse/pkg/notify): its channels and routing live in notify's store, and
// Pilot's internal events are turned into the shared events here.
package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/beacon-stack/pulse/pkg/notify"

	"github.com/beacon-stack/pilot/internal/core/health"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
)

// Deps is what Setup needs from Pilot.
type Deps struct {
	DB  *sql.DB
	Q   db.Querier
	Bus *events.Bus
	// Hub returns Pulse when Pilot is connected to it, nil otherwise.
	Hub func() notify.Hub
	Log *slog.Logger
}

// Setup opens the store, brings over channels set up before the shared
// notifications existed (once), and starts turning Pilot's events into
// notifications. Run the returned outbox's Run in the background.
func Setup(ctx context.Context, d Deps) (*notify.Store, *notify.Outbox, error) {
	store, err := notify.NewStore(ctx, d.DB)
	if err != nil {
		return nil, nil, err
	}
	importOld(ctx, store, d.Q, d.Log)
	out := &notify.Outbox{App: "Pilot", Store: store, Hub: d.Hub, Log: d.Log}
	t := &translator{q: d.Q, out: out}
	d.Bus.Subscribe(t.handle)
	return store, out, nil
}

func importOld(ctx context.Context, store *notify.Store, q db.Querier, log *slog.Logger) {
	rows, err := q.ListNotificationConfigs(ctx)
	if err != nil {
		log.Warn("notifications: couldn't read the old channels to bring them over", "error", err)
		return
	}
	old := make([]notify.LegacyChannel, 0, len(rows))
	for _, r := range rows {
		old = append(old, notify.LegacyChannel{Name: r.Name, Kind: r.Kind, Enabled: r.Enabled, Settings: []byte(r.Settings), Events: decodeList(r.OnEvents)})
	}
	n, skipped, err := store.ImportLegacy(ctx, old)
	if err != nil {
		log.Warn("notifications: bringing over old channels failed", "error", err)
		return
	}
	if n > 0 {
		log.Info("notifications: brought over existing channels", "count", n)
	}
	for _, s := range skipped {
		log.Warn("notifications: a custom-script channel was not brought over; scripts are no longer supported", "channel", s)
	}
}

func decodeList(s string) []string {
	var out []string
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

type publisher interface {
	Publish(notify.Event)
}

// translator turns Pilot's events into the shared notification events.
type translator struct {
	q   db.Querier
	out publisher

	mu     sync.Mutex
	failed map[string]time.Time // grab IDs already told about, so retries don't repeat it
}

const rememberFailures = 3 * 24 * time.Hour // longer than Pilot's ~2 days of retries

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return strings.TrimSpace(s)
}

// series looks up the show behind an event for its name, poster and link.
func (t *translator) series(ctx context.Context, e events.Event) (db.Series, bool) {
	id := e.ShowID
	if id == "" {
		id = str(e.Data, "series_id")
	}
	if id == "" {
		if gid := str(e.Data, "grab_id"); gid != "" {
			if g, err := t.q.GetGrabByID(ctx, gid); err == nil {
				id = g.SeriesID
			}
		}
	}
	if id == "" {
		return db.Series{}, false
	}
	s, err := t.q.GetSeries(ctx, id)
	return s, err == nil
}

func dress(ev *notify.Event, s db.Series, ok bool) {
	if !ok {
		return
	}
	if s.PosterUrl != nil {
		ev.ImageURL = *s.PosterUrl
	}
	if s.TmdbID > 0 {
		ev.URL = fmt.Sprintf("https://www.themoviedb.org/tv/%d", s.TmdbID)
	}
}

func orName(s db.Series, ok bool, fallback string) string {
	if ok && s.Title != "" {
		return s.Title
	}
	if fallback != "" {
		return fallback
	}
	return "a series"
}

// firstFailure reports whether this is the first failure seen for a grab.
func (t *translator) firstFailure(grabID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failed == nil {
		t.failed = map[string]time.Time{}
	}
	now := time.Now()
	for id, at := range t.failed {
		if now.Sub(at) > rememberFailures {
			delete(t.failed, id)
		}
	}
	if _, seen := t.failed[grabID]; seen {
		return false
	}
	t.failed[grabID] = now
	return true
}

func (t *translator) handle(ctx context.Context, e events.Event) {
	ev, ok := t.translate(ctx, e)
	if ok {
		t.out.Publish(ev)
	}
}

func (t *translator) translate(ctx context.Context, e events.Event) (notify.Event, bool) {
	var ev notify.Event
	switch e.Type {
	case events.TypeShowAdded:
		s, ok := t.series(ctx, e)
		ev = notify.Event{Type: notify.TypeAdded, Level: notify.LevelInfo, Title: "Added: " + orName(s, ok, str(e.Data, "title"))}
		dress(&ev, s, ok)

	case events.TypeEpisodeGrabbed:
		release := str(e.Data, "title")
		if release == "" {
			// The download client's copy of the same grab; the indexer's
			// one (with the release name) is the one to announce.
			return ev, false
		}
		s, ok := t.series(ctx, e)
		ev = notify.Event{Type: notify.TypeGrabbed, Level: notify.LevelInfo, Title: "Grabbed: " + orName(s, ok, release), Message: release}
		dress(&ev, s, ok)

	case events.TypeDownloadDone:
		release := str(e.Data, "title")
		s, ok := t.series(ctx, e)
		ev = notify.Event{Type: notify.TypeDownloaded, Level: notify.LevelInfo, Title: "Downloaded: " + orName(s, ok, release), Message: release}
		dress(&ev, s, ok)

	case events.TypeImportComplete:
		s, ok := t.series(ctx, e)
		ev = notify.Event{Type: notify.TypeImported, Level: notify.LevelSuccess, Title: "Imported: " + orName(s, ok, str(e.Data, "series_title"))}
		if gid := str(e.Data, "grab_id"); gid != "" {
			if g, err := t.q.GetGrabByID(ctx, gid); err == nil {
				ev.Message = g.ReleaseTitle
			}
		}
		if manual, _ := e.Data["manual"].(bool); manual {
			ev.Message = strings.TrimSpace(ev.Message + "\nImported by hand.")
		}
		if q := str(e.Data, "quality"); q != "" {
			ev.Fields = map[string]string{"Quality": q}
		}
		dress(&ev, s, ok)

	case events.TypeImportFailed:
		s, ok := t.series(ctx, e)
		reason := str(e.Data, "reason")
		gid := str(e.Data, "grab_id")
		name := orName(s, ok, str(e.Data, "series_title"))
		switch {
		case strings.HasPrefix(reason, "gave up after"):
			ev = notify.Event{Type: notify.TypeImportFailed, Level: notify.LevelError, Title: "Import failed: " + name, Message: "Pilot " + reason + "."}
		case gid == "" || t.firstFailure(gid):
			ev = notify.Event{Type: notify.TypeImportFailed, Level: notify.LevelWarning, Title: "Import failed: " + name,
				Message: strings.TrimSpace(reason + "\nPilot will keep trying for about two days.")}
		default:
			return ev, false // a retry that failed again; already told
		}
		dress(&ev, s, ok)

	case events.TypeGrabFailed, events.TypeGrabStalled, events.TypeGrabStalledGaveUp:
		s, ok := t.series(ctx, e)
		release := str(e.Data, "release_title")
		title, level := "Download failed: ", notify.LevelError
		if e.Type == events.TypeGrabStalled {
			title, level = "Download stalled: ", notify.LevelWarning
		} else if e.Type == events.TypeGrabStalledGaveUp {
			title = "Gave up on a stalled download: "
		}
		msg := release
		if r := str(e.Data, "reason"); r != "" {
			msg = strings.TrimSpace(msg + "\n" + r)
		}
		ev = notify.Event{Type: notify.TypeDownloadProblem, Level: level, Title: title + orName(s, ok, release), Message: msg}
		dress(&ev, s, ok)

	default:
		return ev, false
	}
	return ev, true
}

var digits = regexp.MustCompile(`[0-9]+(\.[0-9]+)?`)

// HealthCheck adapts Pilot's health report for notify.HealthWatcher. The
// key ignores numbers, so "2.0 GB free" becoming "1.9 GB free" is the
// same problem, not a new one.
func HealthCheck(report func(ctx context.Context) []health.Issue) func(ctx context.Context) []notify.Problem {
	return func(ctx context.Context) []notify.Problem {
		var out []notify.Problem
		for _, i := range report(ctx) {
			level := notify.LevelWarning
			if i.Level == health.LevelError {
				level = notify.LevelError
			}
			out = append(out, notify.Problem{Key: i.Source + ":" + digits.ReplaceAllString(i.Message, "#"), Message: i.Message, Level: level})
		}
		return out
	}
}
