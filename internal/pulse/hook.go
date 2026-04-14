package pulse

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/beacon-stack/pilot/internal/core/downloader"
	"github.com/beacon-stack/pilot/internal/core/indexer"
)

// SyncHandler returns an http.HandlerFunc that triggers an immediate
// sync of indexers and download clients from the Pulse control plane.
// Called by Pulse when configs change.
//
// POST /api/v1/hooks/pulse/sync
func (i *Integration) SyncHandler(indexerSvc *indexer.Service, dlSvc *downloader.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"status":405}`, http.StatusMethodNotAllowed)
			return
		}

		go func() {
			if err := i.SyncIndexers(context.Background(), indexerSvc); err != nil {
				i.logger.Warn("pulse: push-triggered indexer sync failed", "error", err)
			}
			if err := i.SyncDownloadClients(context.Background(), dlSvc); err != nil {
				i.logger.Warn("pulse: push-triggered download client sync failed", "error", err)
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	}
}
