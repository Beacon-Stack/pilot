// Package updatecheck tells a Beacon app whether a newer image of it has
// been published.
//
// The same file lives in pilot, prism, haul and pulse
// (internal/updatecheck); change it in all four together.
//
// Beacon images are published from main: every push that passes CI
// pushes ghcr.io/beacon-stack/<app>:latest, built with the commit as its
// version. So for a commit version, "newer" means: the latest successful
// CI run on main is a later commit than the one running. GitHub's
// compare API says how many commits behind, and which. A release-tag
// version (v1.2.3) is compared with the latest GitHub release instead,
// and a build with no version (a local "dev" build) says it can't tell.
package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// UpdateStatus is the response of GET /api/v1/system/updates.
type UpdateStatus struct {
	UpdateAvailable bool           `json:"update_available"`
	CurrentVersion  string         `json:"current_version"`
	LatestVersion   string         `json:"latest_version"`
	ReleaseURL      string         `json:"release_url,omitempty"`   // compare or release page
	ReleaseNotes    string         `json:"release_notes,omitempty"` // a release's notes (plain text)
	PublishedAt     string         `json:"published_at,omitempty"`  // when the newer build was published
	Behind          int            `json:"behind,omitempty"`        // commits behind, for commit versions
	Commits         []UpdateCommit `json:"commits,omitempty"`       // the newer commits, newest first
	Message         string         `json:"message,omitempty"`       // why it can't tell, when it can't
}

// UpdateCommit is one change in a newer build.
type UpdateCommit struct {
	SHA     string `json:"sha"` // short
	Subject string `json:"subject"`
	URL     string `json:"url"`
}

// Checker checks one repository.
type Checker struct {
	Repo      string // "beacon-stack/prism"
	Workflow  string // the workflow that publishes images; default "ci.yml"
	Branch    string // default "main"
	UserAgent string // e.g. "Prism/<version>"
	Client    *http.Client
	BaseURL   string        // default "https://api.github.com"; tests override
	CacheFor  time.Duration // default 5 minutes; GitHub allows 60 unauthenticated calls an hour

	mu       sync.Mutex
	cached   *UpdateStatus
	cachedAt time.Time
	cachedOf string
}

// maxCommits caps the commit list shown for a build far behind.
const maxCommits = 30

var (
	shaRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	tagRe = regexp.MustCompile(`^v?\d+\.\d+(\.\d+)?([-+].*)?$`)
)

// Check compares current (the running build's version) with the newest
// published one. A result whose Message is set means it couldn't tell;
// errors are for GitHub being unreachable or refusing.
func (c *Checker) Check(ctx context.Context, current string) (UpdateStatus, error) {
	c.mu.Lock()
	if c.cached != nil && c.cachedOf == current && time.Since(c.cachedAt) < c.cacheFor() {
		r := *c.cached
		c.mu.Unlock()
		return r, nil
	}
	c.mu.Unlock()

	var (
		r   UpdateStatus
		err error
	)
	v := strings.ToLower(strings.TrimSpace(current))
	switch {
	case shaRe.MatchString(v):
		r, err = c.checkCommit(ctx, v)
	case tagRe.MatchString(v):
		r, err = c.checkRelease(ctx, current)
	default:
		r = UpdateStatus{
			CurrentVersion: current,
			Message: fmt.Sprintf("This build has no version to compare (a local or development build), "+
				"so %s can't tell whether it's out of date.", appName(c.Repo)),
		}
	}
	if err != nil {
		return UpdateStatus{}, err
	}
	c.mu.Lock()
	c.cached, c.cachedAt, c.cachedOf = &r, time.Now(), current
	c.mu.Unlock()
	return r, nil
}

func (c *Checker) checkCommit(ctx context.Context, current string) (UpdateStatus, error) {
	r := UpdateStatus{CurrentVersion: short(current)}

	var runs struct {
		WorkflowRuns []struct {
			HeadSHA   string `json:"head_sha"`
			UpdatedAt string `json:"updated_at"`
		} `json:"workflow_runs"`
	}
	path := fmt.Sprintf("/repos/%s/actions/workflows/%s/runs?branch=%s&event=push&status=success&per_page=1",
		c.Repo, c.workflow(), c.branch())
	if err := c.get(ctx, path, &runs); err != nil {
		return UpdateStatus{}, err
	}
	if len(runs.WorkflowRuns) == 0 {
		r.LatestVersion = r.CurrentVersion
		r.Message = "No published build was found on GitHub."
		return r, nil
	}
	latest := runs.WorkflowRuns[0]
	r.LatestVersion = short(latest.HeadSHA)
	if strings.HasPrefix(latest.HeadSHA, current) || strings.HasPrefix(current, latest.HeadSHA) {
		return r, nil // running the newest build
	}

	var cmp struct {
		Status  string `json:"status"` // ahead | behind | identical | diverged
		AheadBy int    `json:"ahead_by"`
		HTMLURL string `json:"html_url"`
		Commits []struct {
			SHA     string `json:"sha"`
			HTMLURL string `json:"html_url"`
			Commit  struct {
				Message string `json:"message"`
			} `json:"commit"`
		} `json:"commits"`
	}
	err := c.get(ctx, fmt.Sprintf("/repos/%s/compare/%s...%s", c.Repo, current, latest.HeadSHA), &cmp)
	if errors.Is(err, errNotFound) {
		// GitHub doesn't know the running commit: built from a fork or
		// an unpushed checkout. The published build is still newer news.
		r.Message = "The running build's commit isn't on GitHub, so how far behind it is can't be worked out."
		r.UpdateAvailable = true
		r.PublishedAt = latest.UpdatedAt
		return r, nil
	}
	if err != nil {
		return UpdateStatus{}, err
	}
	if cmp.AheadBy == 0 || cmp.Status == "behind" || cmp.Status == "identical" {
		return r, nil // running something at least as new as the published build
	}
	r.UpdateAvailable = true
	r.Behind = cmp.AheadBy
	r.ReleaseURL = cmp.HTMLURL
	r.PublishedAt = latest.UpdatedAt
	// The API lists commits oldest first.
	for i := len(cmp.Commits) - 1; i >= 0 && len(r.Commits) < maxCommits; i-- {
		cm := cmp.Commits[i]
		subject, _, _ := strings.Cut(cm.Commit.Message, "\n")
		r.Commits = append(r.Commits, UpdateCommit{SHA: short(cm.SHA), Subject: subject, URL: cm.HTMLURL})
	}
	return r, nil
}

func (c *Checker) checkRelease(ctx context.Context, current string) (UpdateStatus, error) {
	r := UpdateStatus{CurrentVersion: current, LatestVersion: current}
	var rel struct {
		TagName     string `json:"tag_name"`
		HTMLURL     string `json:"html_url"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
	}
	err := c.get(ctx, fmt.Sprintf("/repos/%s/releases/latest", c.Repo), &rel)
	if errors.Is(err, errNotFound) {
		r.Message = "No releases are published on GitHub yet."
		return r, nil
	}
	if err != nil {
		return UpdateStatus{}, err
	}
	r.LatestVersion = rel.TagName
	if strings.TrimPrefix(rel.TagName, "v") != strings.TrimPrefix(current, "v") {
		r.UpdateAvailable = true
		r.ReleaseURL = rel.HTMLURL
		r.ReleaseNotes = rel.Body
		r.PublishedAt = rel.PublishedAt
	}
	return r, nil
}

var errNotFound = errors.New("not found")

func (c *Checker) get(ctx context.Context, path string, out any) error {
	base := c.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reaching GitHub: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return errNotFound
	case (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) &&
		resp.Header.Get("X-RateLimit-Remaining") == "0":
		return errors.New("GitHub's hourly limit for update checks from this address is used up; try again later")
	case resp.StatusCode != http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("GitHub returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("reading GitHub's answer: %w", err)
	}
	return nil
}

func (c *Checker) workflow() string {
	if c.Workflow == "" {
		return "ci.yml"
	}
	return c.Workflow
}

func (c *Checker) branch() string {
	if c.Branch == "" {
		return "main"
	}
	return c.Branch
}

func (c *Checker) cacheFor() time.Duration {
	if c.CacheFor == 0 {
		return 5 * time.Minute
	}
	return c.CacheFor
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// appName turns "beacon-stack/prism" into "Prism".
func appName(repo string) string {
	_, name, _ := strings.Cut(repo, "/")
	if name == "" {
		return "this app"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
