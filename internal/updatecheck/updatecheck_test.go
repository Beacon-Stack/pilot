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

// releasesAPI fakes GitHub's latest-release and release-list calls.
func releasesAPI(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/repos/beacon-stack/prism/releases/latest":
		_, _ = w.Write([]byte(`{"tag_name":"v1.3.0","html_url":"https://r/1.3.0","body":"## New features\n- three","published_at":"2026-11-03T00:00:00Z"}`))
	case "/repos/beacon-stack/prism/releases":
		_, _ = w.Write([]byte(`[
		  {"tag_name":"v1.4.0-rc.1","name":"v1.4.0-rc.1","body":"rc","prerelease":true},
		  {"tag_name":"v1.3.0","name":"v1.3.0","html_url":"https://r/1.3.0","body":"## New features\n- three","published_at":"2026-11-03T00:00:00Z"},
		  {"tag_name":"v1.2.0","name":"","html_url":"https://r/1.2.0","body":"- two","published_at":"2026-11-02T00:00:00Z"},
		  {"tag_name":"v1.2.1-draft","draft":true,"body":"secret draft"},
		  {"tag_name":"v1.1.0","name":"v1.1.0","body":"- one"}]`))
	default:
		return false
	}
	return true
}

func TestCheck_ReleaseTags(t *testing.T) {
	c, _ := github(t, func(w http.ResponseWriter, r *http.Request) {
		if !releasesAPI(w, r) {
			t.Errorf("path %s", r.URL.Path)
		}
	})
	r, _ := c.Check(context.Background(), "v1.1.0")
	if !r.UpdateAvailable || r.LatestVersion != "v1.3.0" || !strings.Contains(r.ReleaseNotes, "three") {
		t.Errorf("result = %+v", r)
	}
	// Two releases behind: the notes of both, newest first; no
	// pre-release, no draft, nothing it already has.
	if len(r.Releases) != 2 || r.Releases[0].Tag != "v1.3.0" || r.Releases[1].Tag != "v1.2.0" {
		t.Errorf("releases = %+v, want v1.3.0 then v1.2.0", r.Releases)
	}
	if r.Releases[1].Name != "v1.2.0" {
		t.Errorf("a release without a name is named by its tag, got %q", r.Releases[1].Name)
	}
	c.cached = nil
	if r, _ := c.Check(context.Background(), "1.3.0"); r.UpdateAvailable {
		t.Errorf("same version without the v: %+v", r)
	}
	c.cached = nil
	if r, _ := c.Check(context.Background(), "v1.4.0"); r.UpdateAvailable {
		t.Errorf("a build newer than the latest release is not behind: %+v", r)
	}
}

func TestReleases_ListsPublishedNewestFirstAndCaches(t *testing.T) {
	c, calls := github(t, func(w http.ResponseWriter, r *http.Request) { releasesAPI(w, r) })
	rels, err := c.Releases(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	var tags []string
	for _, r := range rels {
		tags = append(tags, r.Tag)
	}
	if strings.Join(tags, ",") != "v1.4.0-rc.1,v1.3.0,v1.2.0,v1.1.0" {
		t.Errorf("tags = %v (drafts must never be listed)", tags)
	}
	if !rels[0].Prerelease {
		t.Error("pre-releases are listed, and marked")
	}
	two, _ := c.Releases(context.Background(), 2)
	if len(two) != 2 || calls.Load() != 1 {
		t.Errorf("limit %d, %d calls; want 2 releases from the cache", len(two), calls.Load())
	}
}

func TestReleases_NoneIsEmptyNotAnError(t *testing.T) {
	c, _ := github(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	rels, err := c.Releases(context.Background(), 5)
	if err != nil || len(rels) != 0 {
		t.Errorf("%v, %v", rels, err)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"v0.13.0", "0.13.0", 0},
		{"v0.13.1", "v0.13.0", 1},
		{"v0.9.0", "v0.13.0", -1},
		{"v1.0.0", "v0.99.9", 1},
		{"v0.14.0-rc.1", "v0.14.0", -1},
		{"v0.14.0-rc.2", "v0.14.0-rc.1", 1},
		{"v0.14", "v0.14.0", 0},
	} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
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
