package main

import (
	"strings"
	"testing"
)

var history = []Commit{ // newest first, as git log gives them
	{SHA: "b8aecd9e2f", Subject: "Merge pull request #93 from Beacon-Stack/ci/drop-develop-tag", Body: "ci: one tag per meaning, drop :develop\n"},
	{SHA: "2757ad90ec", Subject: "Merge pull request #90 from Beacon-Stack/perf/search-remaining", Body: "\nsearch: TVDB/IMDb ids, one blocklist query per search\n"},
	{SHA: "9d2aa31459", Subject: "Merge pull request #88 from Beacon-Stack/perf/interactive-search", Body: "perf(search): stream interactive results.\n"},
	{SHA: "aaaaaaa111", Subject: "fix(ui): toasts no longer cover form buttons", Body: ""},
	{SHA: "bbbbbbb222", Subject: "Merge remote-tracking branch 'origin/main' into feat/x", Body: ""},
	{SHA: "ccccccc333", Subject: "Squashed 'web/ui/src/shared/' content from commit a5195d4", Body: ""},
	{SHA: "ddddddd444", Subject: "feat(vpn)!: region picker replaces SERVER_REGIONS", Body: ""},
	{SHA: "eeeeeee555", Subject: "Merge pull request #80 from Beacon-Stack/feat/tmdb-keys", Body: "feat(tmdb-keys): rotate keys from Settings"},
	{SHA: "fffffff666", Subject: "Rename Luminarr → Beacon Prism", Body: ""},
	{SHA: "1111111777", Subject: "Merge pull request #79 from Beacon-Stack/empty", Body: ""},
	{SHA: "2222222888", Subject: "feat: port database from Postgres to SQLite (#12)", Body: ""},
}

func TestParse(t *testing.T) {
	got := Parse(history)
	type want struct {
		group, scope, text string
		pr                 int
		sha                string
	}
	exp := []want{
		{groupInternal, "", "One tag per meaning, drop :develop", 93, ""},
		{groupOther, "Search", "TVDB/IMDb ids, one blocklist query per search", 90, ""},
		{groupPerf, "Search", "Stream interactive results", 88, ""},
		{groupFixes, "UI", "Toasts no longer cover form buttons", 0, "aaaaaaa"},
		{groupBreaking, "VPN", "Region picker replaces SERVER_REGIONS", 0, "ddddddd"},
		{groupFeatures, "TMDB Keys", "Rotate keys from Settings", 80, ""},
		{groupOther, "", "Rename Luminarr → Beacon Prism", 0, "fffffff"},
		{groupFeatures, "", "Port database from Postgres to SQLite", 12, ""},
	}
	if len(got) != len(exp) {
		t.Fatalf("got %d entries, want %d (merges of branches, subtree squashes and title-less merges are skipped): %+v", len(got), len(exp), got)
	}
	for i, w := range exp {
		g := got[i]
		if g.Group != w.group || g.Scope != w.scope || g.Text != w.text || g.PR != w.pr || g.SHA != w.sha {
			t.Errorf("entry %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestRender(t *testing.T) {
	out := Render(Notes{
		App: "Pilot", Repo: "beacon-stack/pilot", Image: "ghcr.io/beacon-stack/pilot",
		Version: "0.14.0", Tag: "v0.14.0", PrevTag: "v0.13.0",
		Highlights: "Faster searches, and IDs for every show.\n",
		Entries:    Parse(history),
	})
	for _, want := range []string{
		"Faster searches, and IDs for every show.\n\n## Breaking changes",
		"- **VPN:** Region picker replaces SERVER_REGIONS ([ddddddd](https://github.com/beacon-stack/pilot/commit/ddddddd))",
		"- **TMDB Keys:** Rotate keys from Settings ([#80](https://github.com/beacon-stack/pilot/pull/80))",
		"- Port database from Postgres to SQLite ([#12](https://github.com/beacon-stack/pilot/pull/12))",
		"## Fixes\n\n- **UI:** Toasts no longer cover form buttons",
		"## Faster\n\n- **Search:** Stream interactive results ([#88]",
		"## Other changes",
		"<details>\n<summary>Under the hood (1)</summary>",
		"`ghcr.io/beacon-stack/pilot:0.14.0`, `ghcr.io/beacon-stack/pilot:0.14` and `ghcr.io/beacon-stack/pilot:latest`",
		"docker compose pull pilot\ndocker compose up -d pilot",
		"**Full changelog:** https://github.com/beacon-stack/pilot/compare/v0.13.0...v0.14.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("notes missing %q\n---\n%s", want, out)
		}
	}
	// Group order: breaking, features, fixes, faster, other, under the hood.
	order := []string{"## Breaking changes", "## New features", "## Fixes", "## Faster", "## Other changes", "Under the hood"}
	last := -1
	for _, h := range order {
		i := strings.Index(out, h)
		if i < last {
			t.Errorf("%q out of order", h)
		}
		last = i
	}
}

func TestRender_FirstReleaseAndNothingNew(t *testing.T) {
	out := Render(Notes{App: "Haul", Repo: "beacon-stack/haul", Image: "ghcr.io/beacon-stack/haul", Version: "0.1.0", Tag: "v0.1.0"})
	if !strings.Contains(out, "No changes since the previous release.") {
		t.Error("empty release should say so")
	}
	if strings.Contains(out, "Full changelog") {
		t.Error("a first release has nothing to compare with")
	}
}

func TestScopeName(t *testing.T) {
	for in, want := range map[string]string{"vpn": "VPN", "library-scan": "Library Scan", "ui": "UI", "api,db": "API DB", "": ""} {
		if got := scopeName(in); got != want {
			t.Errorf("scopeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRender_FirstReleaseFoldsTheHistory(t *testing.T) {
	out := Render(Notes{
		App: "Haul", Repo: "beacon-stack/haul", Image: "ghcr.io/beacon-stack/haul",
		Version: "0.1.0", Tag: "v0.1.0", Highlights: "The first release of Haul.", Entries: Parse(history),
	})
	if !strings.HasPrefix(out, "The first release of Haul.\n\n<details>\n<summary>Everything in this first release (8 changes)</summary>") {
		t.Errorf("first release should open with the highlights, then the folded history:\n%s", out)
	}
	if !strings.Contains(out, "\n### New features") || strings.Contains(out, "\n## New features") {
		t.Error("inside the fold, groups are subheadings")
	}
	if strings.Count(out, "<details>") != 1 {
		t.Error("one fold for the whole history, not nested ones")
	}
}
