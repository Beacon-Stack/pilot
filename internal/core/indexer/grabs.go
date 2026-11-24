package indexer

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/beacon-stack/pilot/internal/core/dbutil"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/events"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// GrabRequest carries the fields needed to record a grab history entry.
type GrabRequest struct {
	SeriesID     string
	EpisodeID    string // optional; empty for season-pack grabs
	SeasonNumber int    // optional; 0 when not known
	Release      plugin.Release
	IndexerID    string
	// Source is "interactive" (user-initiated via UI) or "auto_search"
	// (triggered by scheduled episode search). Controls whether the stall
	// watcher triggers automatic re-search on failure.
	Source string
}

// GrabHistory returns the grab history for a series, newest first.
func (s *Service) GrabHistory(ctx context.Context, seriesID string) ([]db.GrabHistory, error) {
	return s.q.ListGrabHistoryBySeries(ctx, seriesID)
}

// CreateGrab records a grab history entry linking the release to the series
// and optionally to a specific episode. Pass empty strings for fields that are
// not applicable (e.g. season-pack grabs have no episodeID).
func (s *Service) CreateGrab(ctx context.Context, req GrabRequest) (db.GrabHistory, error) {
	var seasonNumber *int64
	if req.SeasonNumber > 0 {
		sn := int64(req.SeasonNumber)
		seasonNumber = &sn
	}

	source := req.Source
	if source == "" {
		source = "interactive"
	}

	now := time.Now().UTC().Format(time.RFC3339)
	row, err := s.q.CreateGrabHistory(ctx, db.CreateGrabHistoryParams{
		ID:                uuid.New().String(),
		SeriesID:          req.SeriesID,
		EpisodeID:         dbutil.NullableString(req.EpisodeID),
		SeasonNumber:      seasonNumber,
		IndexerID:         dbutil.NullableString(req.IndexerID),
		ReleaseGuid:       req.Release.GUID,
		ReleaseTitle:      req.Release.Title,
		ReleaseSource:     string(req.Release.Quality.Source),
		ReleaseResolution: string(req.Release.Quality.Resolution),
		ReleaseCodec:      string(req.Release.Quality.Codec),
		ReleaseHdr:        string(req.Release.Quality.HDR),
		Protocol:          string(req.Release.Protocol),
		Size:              req.Release.Size,
		DownloadClientID:  nil,
		ClientItemID:      nil,
		DownloadStatus:    "queued",
		ScoreBreakdown:    nil,
		GrabbedAt:         now,
		Source:            source,
		InfoHash:          nil, // populated later when Haul reports it via UpdateGrabInfoHash
	})
	if err != nil {
		return db.GrabHistory{}, fmt.Errorf("recording grab history: %w", err)
	}

	if s.bus != nil {
		s.bus.Publish(ctx, events.Event{
			Type:   events.TypeEpisodeGrabbed,
			ShowID: req.SeriesID,
			Data:   map[string]any{"title": req.Release.Title},
		})
	}

	return row, nil
}

// UpdateGrabDownloadClient records the download client and client-assigned item ID
// on an existing grab history entry. Called after a release is successfully submitted
// to a download client.
func (s *Service) UpdateGrabDownloadClient(ctx context.Context, arg db.UpdateGrabDownloadClientParams) error {
	if err := s.q.UpdateGrabDownloadClient(ctx, arg); err != nil {
		return fmt.Errorf("updating grab download client: %w", err)
	}
	return nil
}

// UpdateGrabStatus sets the download_status on a grab row. Used when
// the grab handler can't make forward progress (e.g. download client
// rejects the release because the indexer response had no download
// URL) — without this, the row stays at "queued" forever and shows
// up in the search guardrail as "already grabbed", suppressing the
// retry the user wants.
func (s *Service) UpdateGrabStatus(ctx context.Context, grabID, status string) error {
	if err := s.q.UpdateGrabStatus(ctx, db.UpdateGrabStatusParams{
		DownloadStatus:  status,
		DownloadedBytes: 0,
		ID:              grabID,
	}); err != nil {
		return fmt.Errorf("updating grab status: %w", err)
	}
	return nil
}

// UpdateGrabInfoHash records the BitTorrent info_hash on a grab row. The
// stall watcher uses this to correlate Haul's stall events back to the
// grab that initiated them.
func (s *Service) UpdateGrabInfoHash(ctx context.Context, grabID, infoHash string) error {
	if err := s.q.UpdateGrabInfoHash(ctx, db.UpdateGrabInfoHashParams{
		InfoHash: dbutil.NullableString(infoHash),
		ID:       grabID,
	}); err != nil {
		return fmt.Errorf("updating grab info_hash: %w", err)
	}
	return nil
}

// GetGrabByInfoHash returns the most-recent grab for the given
// info_hash. Returns sql.ErrNoRows when none matches — callers map
// that to 404 at the API edge. Used by the /grabs/by-hash/{hash}/research
// endpoint to find the original grab so it can blocklist the dead
// release and retry.
func (s *Service) GetGrabByInfoHash(ctx context.Context, infoHash string) (db.GrabHistory, error) {
	return s.q.GetGrabByInfoHash(ctx, dbutil.NullableString(infoHash))
}
