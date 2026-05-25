// Package stats provides library statistics and analytics.
package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// CollectionStats is a summary of the episode library.
type CollectionStats struct {
	TotalSeries   int64 `json:"total_series"`
	TotalEpisodes int64 `json:"total_episodes"`
	Monitored     int64 `json:"monitored"`
	WithFile      int64 `json:"with_file"`
	Missing       int64 `json:"missing"`
	NeedsUpgrade  int64 `json:"needs_upgrade"`
}

// StorageStat is the current total storage used by episode files.
type StorageStat struct {
	TotalBytes int64 `json:"total_bytes"`
	FileCount  int64 `json:"file_count"`
}

// QualityTier is a resolution+source group with a unique-series count.
type QualityTier struct {
	Resolution string `json:"resolution"`
	Source     string `json:"source"`
	Count      int64  `json:"count"`
}

// QualityBucket is a full quality breakdown (resolution+source+codec+hdr)
// with a unique-series count. Used by the "By Dimension" view.
type QualityBucket struct {
	Resolution string `json:"resolution"`
	Source     string `json:"source"`
	Codec      string `json:"codec"`
	HDR        string `json:"hdr"`
	Count      int64  `json:"count"`
}

// GrowthPoint is a point-in-time stats snapshot for trend charts.
type GrowthPoint struct {
	SnapshotAt    string `json:"snapshot_at"`
	TotalSeries   int64  `json:"total_series"`
	TotalEpisodes int64  `json:"total_episodes"`
	WithFile      int64  `json:"with_file"`
	TotalBytes    int64  `json:"total_bytes"`
}

// Service provides library statistics.
type Service struct {
	q db.Querier
}

// NewService creates a new statistics Service.
func NewService(q db.Querier) *Service {
	return &Service{q: q}
}

// Collection returns aggregate counts for the episode library.
func (s *Service) Collection(ctx context.Context) (CollectionStats, error) {
	totalSeries, err := s.q.CountSeries(ctx)
	if err != nil {
		return CollectionStats{}, fmt.Errorf("counting series: %w", err)
	}

	totalEpisodes, err := s.q.CountAllEpisodes(ctx)
	if err != nil {
		return CollectionStats{}, fmt.Errorf("counting episodes: %w", err)
	}

	monitored, err := s.q.CountMonitoredEpisodes(ctx)
	if err != nil {
		return CollectionStats{}, fmt.Errorf("counting monitored episodes: %w", err)
	}

	withFile, err := s.q.CountEpisodesWithFile(ctx)
	if err != nil {
		return CollectionStats{}, fmt.Errorf("counting episodes with file: %w", err)
	}

	missing, err := s.q.CountMissingEpisodes(ctx)
	if err != nil {
		return CollectionStats{}, fmt.Errorf("counting missing episodes: %w", err)
	}

	return CollectionStats{
		TotalSeries:   totalSeries,
		TotalEpisodes: totalEpisodes,
		Monitored:     monitored,
		WithFile:      withFile,
		Missing:       missing,
		NeedsUpgrade:  0, // not yet implemented
	}, nil
}

// Storage returns the current total bytes and file count from episode_files.
func (s *Service) Storage(ctx context.Context) (StorageStat, error) {
	rawBytes, err := s.q.SumEpisodeFileSize(ctx)
	if err != nil {
		return StorageStat{}, fmt.Errorf("summing episode file sizes: %w", err)
	}

	fileCount, err := s.q.CountEpisodeFiles(ctx)
	if err != nil {
		return StorageStat{}, fmt.Errorf("counting episode files: %w", err)
	}

	return StorageStat{
		TotalBytes: toInt64(rawBytes),
		FileCount:  fileCount,
	}, nil
}

// QualityTiers returns unique series counts grouped by resolution+source.
// A series with multiple files at the same tier is counted once. Mirrors
// the bar counts shown on the Stats page and the drilldown result count.
func (s *Service) QualityTiers(ctx context.Context) ([]QualityTier, error) {
	rows, err := s.q.ListEpisodeFileQualitiesWithSeriesIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing episode file qualities: %w", err)
	}

	type tierKey struct{ resolution, source string }
	tierSeries := make(map[tierKey]map[string]bool)

	for _, row := range rows {
		var q plugin.Quality
		if err := json.Unmarshal([]byte(row.QualityJson), &q); err != nil {
			continue
		}
		res := string(q.Resolution)
		if res == "" {
			res = "unknown"
		}
		src := string(q.Source)
		if src == "" {
			src = "unknown"
		}
		k := tierKey{res, src}
		if tierSeries[k] == nil {
			tierSeries[k] = make(map[string]bool)
		}
		tierSeries[k][row.SeriesID] = true
	}

	tiers := make([]QualityTier, 0, len(tierSeries))
	for k, series := range tierSeries {
		tiers = append(tiers, QualityTier{
			Resolution: k.resolution,
			Source:     k.source,
			Count:      int64(len(series)),
		})
	}
	return tiers, nil
}

// Quality returns unique series counts grouped by the full quality
// breakdown (resolution+source+codec+hdr). Powers the "By Dimension" view.
func (s *Service) Quality(ctx context.Context) ([]QualityBucket, error) {
	rows, err := s.q.ListEpisodeFileQualitiesWithSeriesIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing episode file qualities: %w", err)
	}

	type bucketKey struct {
		Resolution string
		Source     string
		Codec      string
		HDR        string
	}
	bucketSeries := make(map[bucketKey]map[string]bool)

	for _, row := range rows {
		var q plugin.Quality
		if err := json.Unmarshal([]byte(row.QualityJson), &q); err != nil {
			continue
		}
		res := string(q.Resolution)
		if res == "" {
			res = "unknown"
		}
		src := string(q.Source)
		if src == "" {
			src = "unknown"
		}
		codec := string(q.Codec)
		if codec == "" {
			codec = "unknown"
		}
		hdr := string(q.HDR)
		if hdr == "" {
			hdr = "none"
		}
		k := bucketKey{res, src, codec, hdr}
		if bucketSeries[k] == nil {
			bucketSeries[k] = make(map[string]bool)
		}
		bucketSeries[k][row.SeriesID] = true
	}

	buckets := make([]QualityBucket, 0, len(bucketSeries))
	for k, series := range bucketSeries {
		buckets = append(buckets, QualityBucket{
			Resolution: k.Resolution,
			Source:     k.Source,
			Codec:      k.Codec,
			HDR:        k.HDR,
			Count:      int64(len(series)),
		})
	}
	return buckets, nil
}

// SeriesIDsByQualityTier returns series IDs that have ANY file matching the
// given resolution and/or source. Mirrors QualityTiers' bucketing — empty
// filter values match any value.
func (s *Service) SeriesIDsByQualityTier(ctx context.Context, resolution, source string) ([]string, error) {
	rows, err := s.q.ListEpisodeFileQualitiesWithSeriesIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing episode file qualities: %w", err)
	}

	matched := make(map[string]bool)

	for _, row := range rows {
		var q plugin.Quality
		if err := json.Unmarshal([]byte(row.QualityJson), &q); err != nil {
			continue
		}
		res := string(q.Resolution)
		if res == "" {
			res = "unknown"
		}
		src := string(q.Source)
		if src == "" {
			src = "unknown"
		}
		if resolution != "" && res != resolution {
			continue
		}
		if source != "" && src != source {
			continue
		}
		matched[row.SeriesID] = true
	}

	ids := make([]string, 0, len(matched))
	for id := range matched {
		ids = append(ids, id)
	}
	return ids, nil
}

// Snapshot records a point-in-time stats snapshot.
func (s *Service) Snapshot(ctx context.Context) error {
	col, err := s.Collection(ctx)
	if err != nil {
		return fmt.Errorf("collecting stats for snapshot: %w", err)
	}

	stor, err := s.Storage(ctx)
	if err != nil {
		return fmt.Errorf("collecting storage for snapshot: %w", err)
	}

	return s.q.InsertStatsSnapshot(ctx, db.InsertStatsSnapshotParams{
		ID:                uuid.New().String(),
		TotalSeries:       col.TotalSeries,
		TotalEpisodes:     col.TotalEpisodes,
		MonitoredEpisodes: col.Monitored,
		WithFile:          col.WithFile,
		Missing:           col.Missing,
		TotalSizeBytes:    stor.TotalBytes,
		SnapshotAt:        time.Now().UTC().Format(time.RFC3339),
	})
}

// Growth returns recent stats snapshots oldest-first for trend charting.
func (s *Service) Growth(ctx context.Context) ([]GrowthPoint, error) {
	rows, err := s.q.ListStatsSnapshots(ctx, 90)
	if err != nil {
		return nil, fmt.Errorf("listing stats snapshots: %w", err)
	}

	// Rows come back newest-first; reverse for chronological order.
	points := make([]GrowthPoint, len(rows))
	for i, r := range rows {
		points[len(rows)-1-i] = GrowthPoint{
			SnapshotAt:    r.SnapshotAt,
			TotalSeries:   int64(r.TotalSeries),
			TotalEpisodes: int64(r.TotalEpisodes),
			WithFile:      int64(r.WithFile),
			TotalBytes:    r.TotalSizeBytes,
		}
	}
	return points, nil
}

// toInt64 converts the interface{} returned by COALESCE(SUM(...), 0) to int64.
// SQLite may return int64 or float64 depending on the driver; handle both.
func toInt64(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	case int:
		return int64(n)
	}
	return 0
}
