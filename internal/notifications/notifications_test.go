package notifications

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/beacon-stack/pulse/pkg/notify"
	_ "modernc.org/sqlite"

	"github.com/beacon-stack/pilot/internal/core/health"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
)

type fakeQ struct {
	db.Querier
	configs []db.NotificationConfig
}

func (fakeQ) GetSeries(_ context.Context, id string) (db.Series, error) {
	if id != "s1" {
		return db.Series{}, sql.ErrNoRows
	}
	poster := "https://image.tmdb.org/t/p/w342/andor.jpg"
	return db.Series{ID: "s1", Title: "Andor", TmdbID: 83867, PosterUrl: &poster}, nil
}

func (fakeQ) GetGrabByID(_ context.Context, id string) (db.GrabHistory, error) {
	if id != "g1" {
		return db.GrabHistory{}, errors.New("no grab")
	}
	return db.GrabHistory{ID: "g1", SeriesID: "s1", ReleaseTitle: "Andor.S02E03.2160p.WEB-DL"}, nil
}

func (q fakeQ) ListNotificationConfigs(context.Context) ([]db.NotificationConfig, error) {
	return q.configs, nil
}

type recorder struct{ got []notify.Event }

func (r *recorder) Publish(e notify.Event) { r.got = append(r.got, e) }

func TestTranslate(t *testing.T) {
	ctx := context.Background()
	rec := &recorder{}
	tr := &translator{q: fakeQ{}, out: rec}

	// Imported: series name, release, quality, poster and link.
	tr.handle(ctx, events.Event{Type: events.TypeImportComplete, ShowID: "s1", Data: map[string]any{"grab_id": "g1", "quality": "2160p WEBDL"}})
	e := rec.got[0]
	if e.Type != notify.TypeImported || e.Title != "Imported: Andor" || e.Message != "Andor.S02E03.2160p.WEB-DL" ||
		e.Fields["Quality"] != "2160p WEBDL" || e.URL != "https://www.themoviedb.org/tv/83867" || e.ImageURL == "" || e.Level != notify.LevelSuccess {
		t.Errorf("imported %+v", e)
	}

	// Grabbed: the indexer's event (with the release name) is announced;
	// the download client's copy of the same grab is not.
	rec.got = nil
	tr.handle(ctx, events.Event{Type: events.TypeEpisodeGrabbed, ShowID: "s1", Data: map[string]any{"title": "Andor.S02E04.1080p"}})
	tr.handle(ctx, events.Event{Type: events.TypeEpisodeGrabbed, Data: map[string]any{"client": "Haul", "client_item_id": "abc"}})
	if len(rec.got) != 1 || rec.got[0].Title != "Grabbed: Andor" || rec.got[0].Message != "Andor.S02E04.1080p" {
		t.Errorf("grabbed %+v", rec.got)
	}

	// Import failures: the first warns, retries stay quiet, giving up is an error.
	rec.got = nil
	fail := func(reason string) {
		tr.handle(ctx, events.Event{Type: events.TypeImportFailed, Data: map[string]any{"grab_id": "g1", "reason": reason, "series_title": "Andor"}})
	}
	fail("no video file found")
	fail("no video file found")
	fail("gave up after 10 import attempts: no video file found")
	if len(rec.got) != 2 {
		t.Fatalf("import failures %+v", rec.got)
	}
	if rec.got[0].Level != notify.LevelWarning || rec.got[0].Title != "Import failed: Andor" {
		t.Errorf("first failure %+v", rec.got[0])
	}
	if rec.got[1].Level != notify.LevelError || rec.got[1].Message != "Pilot gave up after 10 import attempts: no video file found." {
		t.Errorf("gave up %+v", rec.got[1])
	}

	// Stalled downloads are download problems.
	rec.got = nil
	tr.handle(ctx, events.Event{Type: events.TypeGrabStalled, Data: map[string]any{"series_id": "s1", "release_title": "Andor.S02E05", "reason": "no peers for 2h"}})
	if len(rec.got) != 1 || rec.got[0].Type != notify.TypeDownloadProblem || rec.got[0].Title != "Download stalled: Andor" || rec.got[0].Level != notify.LevelWarning {
		t.Errorf("stalled %+v", rec.got)
	}

	// Internal chatter isn't a notification.
	rec.got = nil
	tr.handle(ctx, events.Event{Type: events.TypeTaskStarted})
	tr.handle(ctx, events.Event{Type: events.TypeShowUpdated})
	if len(rec.got) != 0 {
		t.Errorf("internal events produced %+v", rec.got)
	}
}

func TestSetup_BringsOverOldChannels(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "pilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	q := fakeQ{configs: []db.NotificationConfig{
		{Name: "Discord", Kind: "discord", Enabled: true, Settings: `{"webhook_url":"https://discord.com/api/webhooks/1/x"}`, OnEvents: `["import_complete","download_done"]`},
		{Name: "Script", Kind: "command", Enabled: true, Settings: `{"path":"/x.sh"}`, OnEvents: `[]`},
	}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, out, err := Setup(ctx, Deps{DB: sqlDB, Q: q, Bus: events.New(log), Log: log})
	if err != nil || out == nil {
		t.Fatal(err)
	}
	list, _ := store.List(ctx)
	if len(list) != 1 || list[0].Name != "Discord" || len(list[0].Events) != 2 {
		t.Errorf("brought over %+v", list)
	}
	if r := out.Route(ctx); r.ViaPulse || r.PulseConnected {
		t.Errorf("no Pulse: route %+v", r)
	}
}

func TestHealthCheck_SameProblemDespiteChangingNumbers(t *testing.T) {
	check := HealthCheck(func(context.Context) []health.Issue {
		return []health.Issue{{Source: health.SourceLibrary, Level: health.LevelError, Message: `Library "TV" has 2.0 GB free, below its minimum of 5 GB.`}}
	})
	later := HealthCheck(func(context.Context) []health.Issue {
		return []health.Issue{{Source: health.SourceLibrary, Level: health.LevelError, Message: `Library "TV" has 1.4 GB free, below its minimum of 5 GB.`}}
	})
	a, b := check(context.Background())[0], later(context.Background())[0]
	if a.Key != b.Key || a.Level != notify.LevelError || b.Message == a.Message {
		t.Errorf("keys %q vs %q, level %s", a.Key, b.Key, a.Level)
	}
}
