// Package health finds problems that stop Pilot from working: library
// folders that are missing or low on space, and download clients or
// indexers that are missing or unreachable. Like Sonarr's System > Status
// health list.
//
// Library checks are local and cheap, so they run on every Report.
// Connectivity checks call each enabled client and indexer, so they run on
// a schedule (and on demand) and Report returns the last result.
package health

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/indexer"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/diskspace"
)

// Level is how serious an issue is.
type Level string

const (
	// LevelWarning means something may go wrong soon or for some items.
	LevelWarning Level = "warning"
	// LevelError means something is broken now.
	LevelError Level = "error"
)

// Source groups issues by what they are about.
const (
	SourceLibrary        = "library"
	SourceDownloadClient = "download_client"
	SourceIndexer        = "indexer"
)

// Issue is one problem.
type Issue struct {
	Source  string `json:"source"`
	Level   Level  `json:"level"`
	Message string `json:"message"`
}

// Report is the current list of issues.
type Report struct {
	Issues []Issue
	// ConnectivityCheckedAt is when clients and indexers were last tested.
	ConnectivityCheckedAt time.Time
}

// Lister is the part of the downloader and indexer services health uses.
type Lister[T any] interface {
	List(ctx context.Context) ([]T, error)
	Test(ctx context.Context, id string) error
}

// testTimeout bounds one connectivity test.
const testTimeout = 15 * time.Second

// Service runs health checks.
type Service struct {
	q        db.Querier
	clients  Lister[downloader.Config]
	indexers Lister[indexer.Config]
	free     func(path string) (uint64, error)
	now      func() time.Time

	mu        sync.Mutex
	conn      []Issue
	checkedAt time.Time
	running   chan struct{} // closed when the running check ends; nil when idle
}

// NewService creates a Service.
func NewService(q db.Querier, clients Lister[downloader.Config], indexers Lister[indexer.Config]) *Service {
	return &Service{q: q, clients: clients, indexers: indexers, free: diskspace.Free, now: time.Now}
}

// Report returns library issues (checked now) plus the last connectivity
// result. If connectivity was never checked, it is checked first.
func (s *Service) Report(ctx context.Context) Report {
	s.mu.Lock()
	never := s.checkedAt.IsZero()
	s.mu.Unlock()
	if never {
		s.CheckConnectivity(ctx)
	}
	s.mu.Lock()
	conn := append([]Issue(nil), s.conn...)
	at := s.checkedAt
	s.mu.Unlock()

	issues := append(s.libraryIssues(ctx), conn...)
	sortIssues(issues)
	return Report{Issues: issues, ConnectivityCheckedAt: at}
}

// CheckConnectivity tests every enabled download client and indexer and
// stores the result. Concurrent calls share one run.
func (s *Service) CheckConnectivity(ctx context.Context) {
	s.mu.Lock()
	if s.running != nil {
		done := s.running
		s.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
		}
		return
	}
	done := make(chan struct{})
	s.running = done
	s.mu.Unlock()

	var issues []Issue
	issues = append(issues, checkAll(ctx, s.clients, SourceDownloadClient, "download client",
		func(c downloader.Config) (string, string, bool) { return c.ID, c.Name, c.Enabled },
		"No download client is enabled, so nothing can be downloaded.")...)
	issues = append(issues, checkAll(ctx, s.indexers, SourceIndexer, "indexer",
		func(c indexer.Config) (string, string, bool) { return c.ID, c.Name, c.Enabled },
		"No indexer is enabled, so Pilot can't find releases.")...)

	s.mu.Lock()
	s.conn = issues
	s.checkedAt = s.now()
	s.running = nil
	s.mu.Unlock()
	close(done)
}

// checkAll lists the configs, reports none enabled, and tests the
// enabled ones in parallel.
func checkAll[T any](ctx context.Context, l Lister[T], source, noun string,
	ident func(T) (id, name string, enabled bool), noneMsg string) []Issue {
	if l == nil {
		return nil
	}
	cfgs, err := l.List(ctx)
	if err != nil {
		return []Issue{{Source: source, Level: LevelError, Message: fmt.Sprintf("Could not list %ss: %v", noun, err)}}
	}
	type target struct{ id, name string }
	var enabled []target
	for _, c := range cfgs {
		if id, name, on := ident(c); on {
			enabled = append(enabled, target{id, name})
		}
	}
	if len(enabled) == 0 {
		return []Issue{{Source: source, Level: LevelError, Message: noneMsg}}
	}

	failed := make([]string, len(enabled))
	var wg sync.WaitGroup
	for i, t := range enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tctx, cancel := context.WithTimeout(ctx, testTimeout)
			defer cancel()
			if err := l.Test(tctx, t.id); err != nil {
				failed[i] = fmt.Sprintf("Can't reach %s %q: %v", noun, t.name, err)
			}
		}()
	}
	wg.Wait()

	var issues []Issue
	nFailed := 0
	for _, msg := range failed {
		if msg != "" {
			nFailed++
			issues = append(issues, Issue{Source: source, Level: LevelWarning, Message: msg})
		}
	}
	if nFailed == len(enabled) {
		// Every one is down: nothing works, so it's an error.
		for i := range issues {
			issues[i].Level = LevelError
		}
	}
	return issues
}

// libraryIssues checks every library's root folder and free space.
func (s *Service) libraryIssues(ctx context.Context) []Issue {
	libs, err := s.q.ListLibraries(ctx)
	if err != nil {
		return []Issue{{Source: SourceLibrary, Level: LevelError, Message: fmt.Sprintf("Could not list libraries: %v", err)}}
	}
	if len(libs) == 0 {
		return []Issue{{Source: SourceLibrary, Level: LevelWarning, Message: "No library is set up, so there is nowhere to import downloads."}}
	}
	var issues []Issue
	for _, lib := range libs {
		info, err := os.Stat(lib.RootPath)
		switch {
		case err != nil:
			issues = append(issues, Issue{Source: SourceLibrary, Level: LevelError,
				Message: fmt.Sprintf("Library %q: root folder %s is missing or unreadable (%v).", lib.Name, lib.RootPath, err)})
			continue
		case !info.IsDir():
			issues = append(issues, Issue{Source: SourceLibrary, Level: LevelError,
				Message: fmt.Sprintf("Library %q: root folder %s is not a folder.", lib.Name, lib.RootPath)})
			continue
		}
		if lib.MinFreeSpaceGb <= 0 {
			continue
		}
		avail, err := s.free(lib.RootPath)
		if err != nil {
			continue
		}
		if minFree := uint64(lib.MinFreeSpaceGb) * diskspace.GB; avail < minFree {
			issues = append(issues, Issue{Source: SourceLibrary, Level: LevelWarning,
				Message: fmt.Sprintf("Library %q has %.1f GB free, below its minimum of %d GB. Imports that need to copy files will wait until space is freed.",
					lib.Name, float64(avail)/diskspace.GB, lib.MinFreeSpaceGb)})
		}
	}
	return issues
}

// sortIssues puts errors first, then groups by source; message order
// within a group is kept.
func sortIssues(issues []Issue) {
	rank := func(l Level) int {
		if l == LevelError {
			return 0
		}
		return 1
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if ri, rj := rank(issues[i].Level), rank(issues[j].Level); ri != rj {
			return ri < rj
		}
		return issues[i].Source < issues[j].Source
	})
}
