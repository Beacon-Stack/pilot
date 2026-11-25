// Package availability works out where each episode and series stands
// from the viewer's side: ready to watch on the media server, downloaded
// but not there yet, downloading, or still wanted.
package availability

import (
	"context"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/beacon-stack/pilot/internal/core/queue"
	"github.com/beacon-stack/pilot/internal/core/show"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// State is where an episode, or a series as a whole, stands.
type State string

const (
	// Available: the media server has it, so it can be watched.
	Available State = "available"
	// Downloading: a download for it is in the queue.
	Downloading State = "downloading"
	// Downloaded: the file is on disk but the media server doesn't have
	// it (yet), or there is no media server to ask.
	Downloaded State = "downloaded"
	// Wanted: aired and monitored, with nothing downloaded or on its way.
	Wanted State = "wanted"
	// Upcoming: not aired yet.
	Upcoming State = "upcoming"
	// Unmonitored: nothing downloaded and Pilot isn't looking for it.
	Unmonitored State = "unmonitored"
)

// Facts are the things an episode's State is decided from.
type Facts struct {
	HasFile       bool
	Monitored     bool
	Aired         bool
	InMediaServer bool
	Downloading   bool
}

// Decide gives an episode's state. The media server wins: an episode
// that can be watched is available however it got there, including from
// a folder Pilot can't see.
func Decide(f Facts) State {
	switch {
	case f.InMediaServer:
		return Available
	case f.Downloading:
		return Downloading
	case f.HasFile:
		return Downloaded
	case !f.Aired:
		return Upcoming
	case f.Monitored:
		return Wanted
	default:
		return Unmonitored
	}
}

// Summarise gives a series one state from its episodes' states: the most
// active thing happening. A series is Available only when nothing is
// downloading, waiting to reach the media server, or wanted.
func Summarise(counts map[State]int) State {
	for _, s := range []State{Downloading, Downloaded, Wanted} {
		if counts[s] > 0 {
			return s
		}
	}
	switch {
	case counts[Available] > 0:
		return Available
	case counts[Upcoming] > 0:
		return Upcoming
	default:
		return Unmonitored
	}
}

// Info is an episode's state, with how far along a download is.
type Info struct {
	State    State
	Progress float64 // 0..1, meaningful while Downloading
}

// SeriesInfo is a series' state and how many of its episodes can be
// watched.
type SeriesInfo struct {
	State     State
	Available int
	// Progress is the furthest-along download, while State is Downloading.
	Progress float64
}

// ListerSource supplies the media servers that can list their episodes.
type ListerSource interface {
	ShowListers(ctx context.Context) ([]plugin.ShowLister, error)
}

// QueueSource supplies the downloads in progress.
type QueueSource interface {
	GetQueue(ctx context.Context) ([]queue.Item, error)
}

// EpisodeSource supplies a series' episodes.
type EpisodeSource interface {
	EpisodesOfSeries(ctx context.Context, seriesID string) ([]show.Episode, error)
}

// episodeSet is the episodes a media server holds for one show.
type episodeSet map[plugin.EpisodeNumber]struct{}

// Service keeps a copy of what the media servers hold, refreshed on a
// schedule, and answers from it without calling out.
type Service struct {
	servers  ListerSource
	queue    QueueSource
	episodes EpisodeSource
	logger   *slog.Logger
	now      func() time.Time

	mu      sync.RWMutex
	byTMDB  map[int]episodeSet
	byTVDB  map[int]episodeSet
	byTitle map[string]episodeSet // "normalised title|year", for shows with no IDs
}

// NewService creates a Service. Until the first Refresh it knows of no
// media server contents, so nothing is reported as available.
func NewService(servers ListerSource, q QueueSource, episodes EpisodeSource, logger *slog.Logger) *Service {
	return &Service{servers: servers, queue: q, episodes: episodes, logger: logger, now: time.Now}
}

// Refresh re-reads what every media server holds. If a server can't be
// reached its last known contents are kept: a server that is down for a
// minute shouldn't turn the whole library back to "downloaded".
func (s *Service) Refresh(ctx context.Context) error {
	listers, err := s.servers.ShowListers(ctx)
	if err != nil {
		return err
	}
	byTMDB := make(map[int]episodeSet)
	byTVDB := make(map[int]episodeSet)
	byTitle := make(map[string]episodeSet)
	for _, l := range listers {
		items, err := l.ListShowItems(ctx)
		if err != nil {
			return err
		}
		for _, it := range items {
			// The same show on two servers, or split across two
			// libraries, shares one set.
			var set episodeSet
			if it.TMDBID > 0 {
				set = byTMDB[it.TMDBID]
			}
			if set == nil && it.TVDBID > 0 {
				set = byTVDB[it.TVDBID]
			}
			if set == nil {
				set = make(episodeSet, len(it.Episodes))
			}
			for _, e := range it.Episodes {
				set[e] = struct{}{}
			}
			if it.TMDBID > 0 {
				byTMDB[it.TMDBID] = set
			}
			if it.TVDBID > 0 {
				byTVDB[it.TVDBID] = set
			}
			if it.TMDBID == 0 && it.TVDBID == 0 && it.Title != "" && it.Year > 0 {
				byTitle[titleKey(it.Title, it.Year)] = set
			}
		}
	}
	s.mu.Lock()
	s.byTMDB, s.byTVDB, s.byTitle = byTMDB, byTVDB, byTitle
	s.mu.Unlock()
	return nil
}

// held returns the episodes a media server holds for a series, or nil.
// A series is matched by TMDB ID, then TVDB ID; a show the server has no
// IDs for is matched by exact title and year.
func (s *Service) held(sr show.Series) episodeSet {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if set, ok := s.byTMDB[sr.TMDBID]; ok && sr.TMDBID > 0 {
		return set
	}
	if set, ok := s.byTVDB[sr.TVDBID]; ok && sr.TVDBID > 0 {
		return set
	}
	if sr.Year > 0 {
		return s.byTitle[titleKey(sr.Title, sr.Year)]
	}
	return nil
}

// downloads is the queue, arranged for looking up one series' episodes.
type downloads struct {
	byEpisode map[string]float64         // episode ID → progress
	bySeason  map[string]map[int]float64 // series ID → season → progress (season packs)
	bySeries  map[string]float64         // series ID → progress (packs with no season recorded)
}

func (s *Service) downloads(ctx context.Context) downloads {
	d := downloads{
		byEpisode: make(map[string]float64),
		bySeason:  make(map[string]map[int]float64),
		bySeries:  make(map[string]float64),
	}
	if s.queue == nil {
		return d
	}
	items, err := s.queue.GetQueue(ctx)
	if err != nil && s.logger != nil {
		s.logger.Warn("availability: reading queue", "error", err)
	}
	for _, it := range items {
		if it.Status == "failed" || it.Status == "removed" || it.Status == "completed" {
			continue
		}
		p := 0.0
		if it.Size > 0 {
			p = min(1, float64(it.DownloadedBytes)/float64(it.Size))
		}
		switch {
		case it.EpisodeID != "":
			d.byEpisode[it.EpisodeID] = max(d.byEpisode[it.EpisodeID], p)
		case it.SeasonNumber >= 0:
			if d.bySeason[it.SeriesID] == nil {
				d.bySeason[it.SeriesID] = make(map[int]float64)
			}
			d.bySeason[it.SeriesID][it.SeasonNumber] = max(d.bySeason[it.SeriesID][it.SeasonNumber], p)
		default:
			d.bySeries[it.SeriesID] = max(d.bySeries[it.SeriesID], p)
		}
	}
	return d
}

// progressFor reports whether a download covers the episode, and how far
// along it is. A season pack covers the episodes of its season that have
// no file yet.
func (d downloads) progressFor(e show.Episode) (float64, bool) {
	if p, ok := d.byEpisode[e.ID]; ok {
		return p, true
	}
	if e.HasFile {
		return 0, false
	}
	if p, ok := d.bySeason[e.SeriesID][e.SeasonNumber]; ok {
		return p, true
	}
	p, ok := d.bySeries[e.SeriesID]
	return p, ok
}

func (s *Service) decide(e show.Episode, held episodeSet, d downloads, today string) Info {
	_, inServer := held[plugin.EpisodeNumber{Season: e.SeasonNumber, Episode: e.EpisodeNumber}]
	p, downloading := d.progressFor(e)
	state := Decide(Facts{
		HasFile:       e.HasFile,
		Monitored:     e.Monitored,
		Aired:         e.AirDate != "" && e.AirDate <= today,
		InMediaServer: inServer,
		Downloading:   downloading,
	})
	info := Info{State: state}
	if state == Downloading {
		info.Progress = p
	}
	return info
}

// ForEpisodes returns the state of each of a series' episodes, keyed by
// episode ID. A nil Service returns nothing, so callers needn't check
// whether one is set up.
func (s *Service) ForEpisodes(ctx context.Context, sr show.Series, episodes []show.Episode) map[string]Info {
	if s == nil {
		return nil
	}
	held := s.held(sr)
	d := s.downloads(ctx)
	today := s.now().Format("2006-01-02")
	out := make(map[string]Info, len(episodes))
	for _, e := range episodes {
		out[e.ID] = s.decide(e, held, d, today)
	}
	return out
}

// ForSeries returns each series' state and how many of its episodes can
// be watched, keyed by series ID. A nil Service returns nothing.
func (s *Service) ForSeries(ctx context.Context, series []show.Series) map[string]SeriesInfo {
	if s == nil {
		return nil
	}
	d := s.downloads(ctx)
	today := s.now().Format("2006-01-02")
	out := make(map[string]SeriesInfo, len(series))
	for _, sr := range series {
		episodes, err := s.episodes.EpisodesOfSeries(ctx, sr.ID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("availability: listing episodes", "series", sr.Title, "error", err)
			}
			continue
		}
		held := s.held(sr)
		counts := make(map[State]int)
		progress := 0.0
		for _, e := range episodes {
			info := s.decide(e, held, d, today)
			counts[info.State]++
			progress = max(progress, info.Progress)
		}
		info := SeriesInfo{State: Summarise(counts), Available: counts[Available]}
		if info.State == Downloading {
			info.Progress = progress
		}
		out[sr.ID] = info
	}
	return out
}

var notTitleChar = regexp.MustCompile(`[^a-z0-9\s]`)

func titleKey(title string, year int) string {
	t := notTitleChar.ReplaceAllString(strings.ToLower(title), "")
	return strings.Join(strings.Fields(t), " ") + "|" + strconv.Itoa(year)
}
