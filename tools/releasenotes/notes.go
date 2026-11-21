// Command releasenotes writes the notes for a GitHub release from the
// git history between two tags.
//
// The same tool lives in pilot, prism, haul and pulse
// (tools/releasenotes); change it in all four together.
//
// One entry per merged pull request, from the PR's title (which the
// merge commit carries), so a PR of ten commits is one line. Commits
// pushed straight to main get an entry each. Entries are grouped by the
// conventional-commit type of their title: breaking changes, new
// features, fixes and speed-ups up top, everything else (CI, build,
// tests, chores, docs, refactors) folded away under "Under the hood".
// A hand-written highlights file, if present, goes first; an "Upgrading"
// section with the image tags goes last.
package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Commit is one first-parent commit between the two tags.
type Commit struct {
	SHA     string
	Subject string
	Body    string
}

// Entry is one line of the notes.
type Entry struct {
	Group    string
	Scope    string
	Text     string
	PR       int    // 0 for a commit pushed straight to main
	SHA      string // short; for direct commits
	Breaking bool
}

var (
	mergePRRe = regexp.MustCompile(`^Merge pull request #(\d+) from `)
	// A squash-merged PR: GitHub appends " (#12)" to the title.
	squashPRRe = regexp.MustCompile(`\s*\(#(\d+)\)$`)
	typedRe    = regexp.MustCompile(`^([a-zA-Z]+)(?:\(([^)]+)\))?(!)?:\s*(.+)$`)
	// Merges of branches into each other, subtree squashes: not changes.
	noiseRe = regexp.MustCompile(`^(Merge (branch|remote-tracking branch|commit|tag) |Squashed '|Merge pull request #\d+ from \S+$)`)
)

// Groups in the order they appear in the notes.
const (
	groupBreaking = "Breaking changes"
	groupFeatures = "New features"
	groupFixes    = "Fixes"
	groupPerf     = "Faster"
	groupOther    = "Other changes"
	groupInternal = "Under the hood"
)

var groupOrder = []string{groupBreaking, groupFeatures, groupFixes, groupPerf, groupOther, groupInternal}

var typeGroups = map[string]string{
	"feat": groupFeatures, "feature": groupFeatures,
	"fix": groupFixes, "bugfix": groupFixes, "security": groupFixes,
	"perf": groupPerf,
	"ci":   groupInternal, "build": groupInternal, "chore": groupInternal, "test": groupInternal,
	"tests": groupInternal, "docs": groupInternal, "refactor": groupInternal, "style": groupInternal,
	"deps": groupInternal, "revert": groupOther,
}

// Words that read better in capitals than title case.
var acronyms = map[string]string{
	"api": "API", "ui": "UI", "vpn": "VPN", "ci": "CI", "tmdb": "TMDB", "db": "DB",
	"rss": "RSS", "sse": "SSE", "ux": "UX", "json": "JSON", "url": "URL", "dns": "DNS",
}

// Parse turns first-parent commits (newest first) into entries.
func Parse(commits []Commit) []Entry {
	var out []Entry
	for _, c := range commits {
		title, pr := c.Subject, 0
		if m := mergePRRe.FindStringSubmatch(c.Subject); m != nil {
			pr, _ = strconv.Atoi(m[1])
			title = firstLine(c.Body)
			if title == "" {
				continue // a merge with no PR title tells a reader nothing
			}
		} else if noiseRe.MatchString(c.Subject) {
			continue
		} else if m := squashPRRe.FindStringSubmatch(title); m != nil {
			pr, _ = strconv.Atoi(m[1])
			title = strings.TrimSpace(squashPRRe.ReplaceAllString(title, ""))
		}
		e := Entry{PR: pr, Group: groupOther, Text: title}
		if pr == 0 {
			e.SHA = short(c.SHA)
		}
		if m := typedRe.FindStringSubmatch(title); m != nil {
			typ := strings.ToLower(m[1])
			e.Scope = scopeName(m[2])
			if g, ok := typeGroups[typ]; ok {
				e.Group = g
			} else if e.Scope == "" {
				// "search: …" isn't a change type; read it as the topic.
				e.Scope = scopeName(m[1])
			}
			e.Breaking = m[3] == "!" || strings.Contains(c.Body, "BREAKING CHANGE")
			e.Text = m[4]
		}
		if e.Breaking {
			e.Group = groupBreaking
		}
		e.Text = capitalize(strings.TrimSuffix(strings.TrimSpace(e.Text), "."))
		out = append(out, e)
	}
	return out
}

// Notes holds everything Render needs.
type Notes struct {
	App        string // "Pilot"
	Repo       string // "beacon-stack/pilot"
	Image      string // "ghcr.io/beacon-stack/pilot"
	Version    string // "0.14.0"
	Tag        string // "v0.14.0"
	PrevTag    string // "" for a first release
	Highlights string // hand-written markdown, optional
	Entries    []Entry
}

// Render writes the release notes as GitHub markdown.
func Render(n Notes) string {
	var b strings.Builder
	if h := strings.TrimSpace(n.Highlights); h != "" {
		b.WriteString(h + "\n\n")
	}

	first := n.PrevTag == ""
	if first && len(n.Entries) > 0 {
		// A first release would list the whole history: fold it away and
		// let the highlights speak.
		fmt.Fprintf(&b, "<details>\n<summary>Everything in this first release (%d changes)</summary>\n\n", len(n.Entries))
	}

	byGroup := map[string][]Entry{}
	for _, e := range n.Entries {
		byGroup[e.Group] = append(byGroup[e.Group], e)
	}
	userFacing := 0
	for _, g := range groupOrder {
		entries := byGroup[g]
		if len(entries) == 0 {
			continue
		}
		// Grouped by scope, so related changes sit together.
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].Scope < entries[j].Scope })
		if g == groupInternal && !first {
			fmt.Fprintf(&b, "<details>\n<summary>%s (%d)</summary>\n\n", g, len(entries))
		} else if first {
			userFacing += len(entries)
			fmt.Fprintf(&b, "### %s\n\n", g)
		} else {
			userFacing += len(entries)
			fmt.Fprintf(&b, "## %s\n\n", g)
		}
		for _, e := range entries {
			b.WriteString("- " + n.line(e) + "\n")
		}
		if g == groupInternal && !first {
			b.WriteString("\n</details>\n")
		}
		b.WriteString("\n")
	}
	if first && len(n.Entries) > 0 {
		b.WriteString("</details>\n\n")
	}
	if len(n.Entries) == 0 || userFacing == 0 && byGroup[groupInternal] == nil {
		b.WriteString("No changes since the previous release.\n\n")
	}

	b.WriteString("## Upgrading\n\n")
	fmt.Fprintf(&b, "Docker images for amd64 and arm64: `%s:%s`, `%s:%s` and `%s:latest`.\n\n",
		n.Image, n.Version, n.Image, minor(n.Version), n.Image)
	app := strings.ToLower(n.App)
	fmt.Fprintf(&b, "```sh\ndocker compose pull %s\ndocker compose up -d %s\n```\n\n", app, app)
	b.WriteString("Binaries are attached below.\n")
	if n.PrevTag != "" {
		fmt.Fprintf(&b, "\n**Full changelog:** https://github.com/%s/compare/%s...%s\n", n.Repo, n.PrevTag, n.Tag)
	}
	return b.String()
}

func (n Notes) line(e Entry) string {
	s := e.Text
	if e.Scope != "" {
		s = "**" + e.Scope + ":** " + s
	}
	switch {
	case e.PR > 0:
		s += fmt.Sprintf(" ([#%d](https://github.com/%s/pull/%d))", e.PR, n.Repo, e.PR)
	case e.SHA != "":
		s += fmt.Sprintf(" ([%s](https://github.com/%s/commit/%s))", e.SHA, n.Repo, e.SHA)
	}
	return s
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

func scopeName(scope string) string {
	if scope == "" {
		return ""
	}
	parts := strings.FieldsFunc(scope, func(r rune) bool { return r == '-' || r == '_' || r == ',' || r == ' ' })
	for i, p := range parts {
		if a, ok := acronyms[strings.ToLower(p)]; ok {
			parts[i] = a
		} else {
			parts[i] = capitalize(p)
		}
	}
	return strings.Join(parts, " ")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// minor turns "0.14.2" into "0.14".
func minor(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}
