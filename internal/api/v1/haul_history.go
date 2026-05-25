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
	"os"
	"path/filepath"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/importer"
	db "github.com/beacon-stack/pilot/internal/db/generated"
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

type reimportGrabInput struct {
	GrabID string `path:"grab_id" doc:"Grab history UUID"`
}

type grabFileStatusInput struct {
	GrabID string `path:"grab_id" doc:"Grab UUID"`
}

type grabFileStatusOutput struct {
	Body struct {
		// Exists is true when a real file was found at the path Haul
		// reports. False means: no info_hash, no Haul record, or
		// os.Stat failed (file missing on disk).
		Exists bool `json:"exists"`
		// Reason explains why exists is false. One of: "no_info_hash",
		// "no_haul_client", "haul_unreachable", "not_in_haul_history",
		// "file_missing_on_disk", or empty when Exists is true.
		Reason string `json:"reason,omitempty"`
		// Path is the resolved path Haul claims has the file. May be
		// set even when Exists is false (the file SHOULD live there
		// but doesn't).
		Path string `json:"path,omitempty"`
		// Size is the file size in bytes when Exists is true.
		Size int64 `json:"size,omitempty"`
	}
}

// validateReimportGrab returns an HTTP error string if the grab is
// ineligible for re-import, or "" when it can proceed. Extracted so
// tests can pin the eligibility rules without spinning up the full
// Haul + importer stack.
//
// Rules:
//   - status must be "completed"; in-progress / failed / removed grabs
//     have nothing on disk to re-import (or shouldn't be re-imported)
//   - info_hash must be present; without it we can't locate the file
//     in Haul's history index
func validateReimportGrab(grab db.GrabHistory) string {
	if grab.DownloadStatus != "completed" {
		return fmt.Sprintf("grab status is %q — only completed grabs can be re-imported", grab.DownloadStatus)
	}
	if grab.InfoHash == nil || *grab.InfoHash == "" {
		return "grab has no info_hash — Pilot never received it back from the download client, so we can't locate the file"
	}
	return ""
}

// RegisterHaulHistoryRoutes wires the haul-history endpoints.
func RegisterHaulHistoryRoutes(api huma.API, q db.Querier, downloaderSvc *downloader.Service, importerSvc *importer.Service) {
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

	huma.Register(api, huma.Operation{
		OperationID: "grab-file-status",
		Method:      http.MethodGet,
		Path:        "/api/v1/grabs/{grab_id}/file-status",
		Summary:     "Check whether a grab's downloaded file actually exists on disk",
		Description: "Resolves grab → info_hash → Haul history record → expected file path → os.Stat. The frontend uses this to decide whether to render an Import button (file present) or an Investigate link (file missing or Haul lost track) on a `completed`-but-not-imported grab. Returns 404 if the grab itself doesn't exist; otherwise always returns 200 with `exists` + a `reason` describing the failure mode when it's false.",
		Tags:        []string{"Haul"},
	}, func(ctx context.Context, input *grabFileStatusInput) (*grabFileStatusOutput, error) {
		out := &grabFileStatusOutput{}
		grab, err := q.GetGrabByID(ctx, input.GrabID)
		if err != nil {
			return nil, huma.Error404NotFound("grab not found")
		}
		if grab.InfoHash == nil || *grab.InfoHash == "" {
			out.Body.Exists = false
			out.Body.Reason = "no_info_hash"
			return out, nil
		}
		client, clientErr := firstHaulClient(ctx, downloaderSvc)
		if clientErr != nil || client == nil {
			out.Body.Exists = false
			out.Body.Reason = "no_haul_client"
			return out, nil //nolint:nilerr // intentional: surface as exists=false to caller
		}
		rec, lookupErr := client.LookupHistoryByHash(ctx, *grab.InfoHash)
		if lookupErr != nil {
			// Degrade gracefully — report exists=false with a reason
			// rather than 500ing. The UI then renders Investigate
			// instead of Import, which is still the right action.
			out.Body.Exists = false
			out.Body.Reason = "haul_unreachable"
			return out, nil //nolint:nilerr // intentional: surface as exists=false to caller
		}
		if rec == nil {
			out.Body.Exists = false
			out.Body.Reason = "not_in_haul_history"
			return out, nil
		}
		// Haul reports SavePath + Name; the file (or directory) lives at
		// the join. We stat the join — for multi-file torrents that's a
		// dir, which still reports as Exists. UI distinguishes only on
		// existence, not file vs dir.
		path := filepath.Join(rec.SavePath, rec.Name)
		out.Body.Path = path
		fi, statErr := os.Stat(path)
		if statErr != nil {
			out.Body.Exists = false
			out.Body.Reason = "file_missing_on_disk"
			return out, nil //nolint:nilerr // intentional: surface as exists=false to caller
		}
		out.Body.Exists = true
		out.Body.Size = fi.Size()
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "reimport-grab",
		Method:      http.MethodPost,
		Path:        "/api/v1/grabs/{grab_id}/reimport",
		Summary:     "Re-run the import pipeline for an existing grab whose file is on disk but not linked",
		Description: "Looks up the grab in grab_history, finds the matching Haul record by info_hash (NOT requester metadata), and runs the import pipeline. Useful for orphaned grabs — older grabs that predate Phase 1-4's metadata SDK and whose files Haul has but Pilot couldn't auto-import (e.g. anime absolute-numbered episodes before the importer fix). Differs from /api/v1/import/from-haul: that endpoint reads series_id from Haul's requester metadata; this one reads it from Pilot's grab_history, so it works on grabs Haul knows nothing series-wise about.",
		Tags:        []string{"Haul"},
	}, func(ctx context.Context, input *reimportGrabInput) (*importFromHaulOutput, error) {
		grab, err := q.GetGrabByID(ctx, input.GrabID)
		if err != nil {
			return nil, huma.Error404NotFound("grab not found")
		}
		if msg := validateReimportGrab(grab); msg != "" {
			return nil, huma.Error409Conflict(msg)
		}

		client, err := firstHaulClient(ctx, downloaderSvc)
		if err != nil {
			return nil, huma.Error503ServiceUnavailable(err.Error())
		}
		if client == nil {
			return nil, huma.Error503ServiceUnavailable("no Haul download client configured")
		}
		rec, err := client.LookupHistoryByHash(ctx, *grab.InfoHash)
		if err != nil {
			return nil, huma.Error502BadGateway(err.Error())
		}
		if rec == nil {
			return nil, huma.Error404NotFound("info_hash not found in Haul history — the file may have been removed")
		}

		contentPath := filepath.Join(rec.SavePath, rec.Name)
		if err := importerSvc.ImportFromHaulRecord(ctx, grab.SeriesID, contentPath, plugin.Quality{}); err != nil {
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
