package v1

// haul_history.go — endpoints that proxy to Haul's history index so
// the Pilot UI can show "downloaded externally" badges and trigger
// re-imports for files Haul has but Pilot's library doesn't know
// about.
//
// The Pilot UI doesn't talk directly to Haul (CORS + auth: the
// browser can't carry Pilot's API key into Haul's domain). These
// endpoints sit in front, run as Pilot's process, and own the
// Haul-side credentials.

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/importer"
	"github.com/beacon-stack/pilot/pkg/plugin"
	"github.com/beacon-stack/pilot/plugins/downloaders/haul"
)

type seriesHaulHistoryInput struct {
	SeriesID string `path:"id" doc:"Series UUID"`
}

type seriesHaulHistoryOutput struct {
	Body struct {
		// Records are Haul history rows associated with this series
		// via the requester_series_id metadata. When empty, Haul
		// has no record of any download for this series.
		Records []haul.HistoryRecord `json:"records"`
	}
}

type importFromHaulInput struct {
	Body struct {
		InfoHash string `json:"info_hash" doc:"Haul info hash to import"`
	}
}

type importFromHaulOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

// RegisterHaulHistoryRoutes wires the two endpoints.
func RegisterHaulHistoryRoutes(api huma.API, downloaderSvc *downloader.Service, importerSvc *importer.Service) {
	huma.Register(api, huma.Operation{
		OperationID: "list-series-haul-history",
		Method:      http.MethodGet,
		Path:        "/api/v1/series/{id}/haul-history",
		Summary:     "List Haul torrent records associated with this series",
		Description: "Returns Haul's view of every torrent grabbed against this series, regardless of whether the corresponding files are linked in the Pilot library. Used by the per-episode \"Haul has it\" badge.",
		Tags:        []string{"Haul"},
	}, func(ctx context.Context, input *seriesHaulHistoryInput) (*seriesHaulHistoryOutput, error) {
		client, err := firstHaulClient(ctx, downloaderSvc)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		out := &seriesHaulHistoryOutput{}
		out.Body.Records = []haul.HistoryRecord{}
		if client == nil {
			// No Haul client configured — return empty so the UI
			// renders no badges. Not an error.
			return out, nil
		}
		records, err := client.LookupHistory(ctx, haul.HistoryFilter{
			Service:  "pilot",
			SeriesID: input.SeriesID,
			Limit:    200,
		})
		if err != nil {
			return nil, huma.Error502BadGateway(err.Error())
		}
		out.Body.Records = records
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "import-from-haul",
		Method:      http.MethodPost,
		Path:        "/api/v1/import/from-haul",
		Summary:     "Run the import pipeline against an existing Haul torrent",
		Description: "Looks up the Haul history record by info_hash, resolves its series via requester metadata, and runs the import pipeline against the file on disk. Used by \"Haul has it\" badges and the Activity-page \"downloaded but not in library\" rail.",
		Tags:        []string{"Haul"},
	}, func(ctx context.Context, input *importFromHaulInput) (*importFromHaulOutput, error) {
		hash := strings.TrimSpace(input.Body.InfoHash)
		if hash == "" {
			return nil, huma.Error400BadRequest("info_hash is required")
		}
		client, err := firstHaulClient(ctx, downloaderSvc)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		if client == nil {
			return nil, huma.Error503ServiceUnavailable("no Haul download client configured")
		}
		rec, err := client.LookupHistoryByHash(ctx, hash)
		if err != nil {
			return nil, huma.Error502BadGateway(err.Error())
		}
		if rec == nil {
			return nil, huma.Error404NotFound("info_hash not found in Haul history")
		}
		if rec.SeriesID == "" {
			return nil, huma.Error409Conflict(
				"Haul record has no series_id metadata — was the torrent grabbed via Pilot? Sideloaded torrents can't be auto-imported")
		}

		// Reconstruct the on-disk path. Haul's record has save_path
		// (the directory) and name (the torrent name); their join
		// is what the importer wants. For multi-file torrents the
		// importer walks the directory anyway.
		contentPath := filepath.Join(rec.SavePath, rec.Name)

		// Quality: the importer falls back to filename parsing when
		// the grab carries an empty Quality, so passing zero-value
		// here is correct.
		if err := importerSvc.ImportFromHaulRecord(ctx, rec.SeriesID, contentPath, plugin.Quality{}); err != nil {
			return nil, huma.Error500InternalServerError("import failed: " + err.Error())
		}

		out := &importFromHaulOutput{}
		out.Body.Status = "imported"
		return out, nil
	})
}

// firstHaulClient finds and returns the first enabled Haul download
// client. Returns (nil, nil) when no Haul client is configured —
// callers treat that as "feature disabled" rather than an error.
func firstHaulClient(ctx context.Context, downloaderSvc *downloader.Service) (*haul.Client, error) {
	configs, err := downloaderSvc.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing download clients: %w", err)
	}
	for _, cfg := range configs {
		if !cfg.Enabled || cfg.Kind != "haul" {
			continue
		}
		client, err := downloaderSvc.ClientFor(ctx, cfg.ID)
		if err != nil {
			continue
		}
		hc, ok := client.(*haul.Client)
		if !ok {
			continue
		}
		return hc, nil
	}
	return nil, nil
}
