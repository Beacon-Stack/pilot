package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	running = "1111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	newest  = "2222222bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// github fakes the two API calls a commit check makes.
func github(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*Checker, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Checker{Repo: "beacon-stack/prism", BaseURL: srv.URL}, &calls
}

func latestRun(w http.ResponseWriter, sha string) {
	_, _ = w.Write([]byte(`{"workflow_runs":[{"head_sha":"` + sha + `","updated_at":"2026-11-21T10:00:00Z"}]}`))
}

func TestCheck_AsksTheRightRepoAndWorkflow(t *testing.T) {
	var paths []string
	c, _ := github(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		latestRun(w, running)
	})
	if _, err := c.Check(context.Background(), running); err != nil {
		t.Fatal(err)
	}
	want := "/repos/beacon-stack/prism/actions/workflows/ci.yml/runs?branch=main&event=push&status=success&per_page=1"
	if len(paths) != 1 || paths[0] != want {
		t.Errorf("asked %v, want [%s]", paths, want)
	}
}

func TestCheck_UpToDate(t *testing.T) {
	c, calls := github(t, func(w http.ResponseWriter, r *http.Request) { latestRun(w, running) })
	r, err := c.Check(context.Background(), running[:7]) // short SHA works too
	if err != nil {
		t.Fatal(err)
	}
	if r.UpdateAvailable || r.LatestVersion != "1111111" || r.Message != "" {
		t.Errorf("result = %+v, want up to date", r)
	}
	if calls.Load() != 1 {
		t.Errorf("%d calls; running the newest build needs no compare", calls.Load())
	}
}

func TestCheck_BehindListsNewerCommitsNewestFirst(t *testing.T) {
	c, _ := github(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/compare/") {
			if !strings.HasSuffix(r.URL.Path, running+"..."+newest) {
				t.Errorf("compare path %s", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"status":"ahead","ahead_by":2,"html_url":"https://github.com/beacon-stack/prism/compare/x...y",
			  "commits":[
			    {"sha":"aaaaaaa1","html_url":"https://c/1","commit":{"message":"fix: older\n\nbody"}},
			    {"sha":"bbbbbbb2","html_url":"https://c/2","commit":{"message":"feat: newer"}}]}`))
			return
		}
		latestRun(w, newest)
	})
	r, err := c.Check(context.Background(), running)
	if err != nil {
		t.Fatal(err)
	}
	if !r.UpdateAvailable || r.Behind != 2 || r.LatestVersion != "2222222" || r.CurrentVersion != "1111111" {
		t.Errorf("result = %+v", r)
	}
	if len(r.Commits) != 2 || r.Commits[0].Subject != "feat: newer" || r.Commits[1].Subject != "fix: older" {
		t.Errorf("commits = %+v, want newest first, subject lines only", r.Commits)
	}
	if r.ReleaseURL == "" || r.PublishedAt == "" {
		t.Errorf("missing link or date: %+v", r)
	}
}

// Running something newer than the last published build (a build of an
// unmerged branch, say) is not "behind".
func TestCheck_AheadOfPublishedIsNotAnUpdate(t *testing.T) {
	c, _ := github(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/compare/") {
			_, _ = w.Write([]byte(`{"status":"behind","ahead_by":0,"behind_by":3}`))
			return
		}
		latestRun(w, newest)
	})
	r, _ := c.Check(context.Background(), running)
	if r.UpdateAvailable {
		t.Errorf("result = %+v, want no update", r)
	}
}

func TestCheck_UnknownCommitStillPointsAtTheNewBuild(t *testing.T) {
	c, _ := github(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/compare/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		latestRun(w, newest)
	})
	r, err := c.Check(context.Background(), running)
	if err != nil {
		t.Fatal(err)
	}
	if !r.UpdateAvailable || r.Message == "" {
		t.Errorf("result = %+v, want an update with an explanation", r)
	}
}

func TestCheck_DevBuildCantTell(t *testing.T) {
	c, calls := github(t, func(w http.ResponseWriter, r *http.Request) { t.Error("no call expected") })
	for _, v := range []string{"dev", "", "abc1234-dirty"} {
		r, err := c.Check(context.Background(), v)
		if err != nil || r.UpdateAvailable || !strings.Contains(r.Message, "Prism can't tell") {
			t.Errorf("%q: %+v, %v", v, r, err)
		}
	}
	if calls.Load() != 0 {
		t.Errorf("%d GitHub calls for builds with no version", calls.Load())
	}
}

func TestCheck_ReleaseTags(t *testing.T) {
	c, _ := github(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/beacon-stack/prism/releases/latest" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.3.0","html_url":"https://r","body":"notes","published_at":"2026-11-01T00:00:00Z"}`))
	})
	r, _ := c.Check(context.Background(), "v1.2.0")
	if !r.UpdateAvailable || r.LatestVersion != "v1.3.0" || r.ReleaseNotes != "notes" {
		t.Errorf("result = %+v", r)
	}
	c.cached = nil
	if r, _ := c.Check(context.Background(), "1.3.0"); r.UpdateAvailable {
		t.Errorf("same version without the v: %+v", r)
	}
}

func TestCheck_NoReleasesIsNotAnError(t *testing.T) {
	c, _ := github(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	r, err := c.Check(context.Background(), "v1.0.0")
	if err != nil || r.UpdateAvailable || r.Message == "" {
		t.Errorf("%+v, %v", r, err)
	}
}

func TestCheck_RateLimitSaysSo(t *testing.T) {
	c, _ := github(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	})
	_, err := c.Check(context.Background(), running)
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("err = %v, want a rate-limit explanation", err)
	}
}

func TestCheck_CachesBriefly(t *testing.T) {
	c, calls := github(t, func(w http.ResponseWriter, r *http.Request) { latestRun(w, running) })
	for i := 0; i < 3; i++ {
		_, _ = c.Check(context.Background(), running)
	}
	if calls.Load() != 1 {
		t.Errorf("%d GitHub calls for three clicks, want 1", calls.Load())
	}
}
