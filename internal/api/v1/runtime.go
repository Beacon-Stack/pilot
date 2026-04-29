package v1

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// runtimeBody is the response shape for /api/v1/system/runtime.
// Surfaces Go-internal stats Docker stats can't see — heap usage,
// goroutine count, GC activity — and basic host metadata. Pulse's
// dashboard pairs this with container CPU/mem/net from Docker for a
// full health picture.
type runtimeBody struct {
	Goroutines    int    `json:"goroutines"`
	HeapAlloc     uint64 `json:"heap_alloc_bytes"`
	HeapInUse     uint64 `json:"heap_in_use_bytes"`
	HeapObjects   uint64 `json:"heap_objects"`
	NumGC         uint32 `json:"num_gc"`
	LastGCPauseNs uint64 `json:"last_gc_pause_ns"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	GoVersion     string `json:"go_version"`
	GoOS          string `json:"goos"`
	GoArch        string `json:"goarch"`
	NumCPU        int    `json:"num_cpu"`
	Hostname      string `json:"hostname"`
}

type runtimeOutput struct {
	Body *runtimeBody
}

// RegisterRuntimeRoutes registers the Go-runtime stats endpoint.
func RegisterRuntimeRoutes(api huma.API, startTime time.Time) {
	huma.Register(api, huma.Operation{
		OperationID: "system-runtime",
		Method:      http.MethodGet,
		Path:        "/api/v1/system/runtime",
		Summary:     "Go runtime statistics and host metadata",
		Tags:        []string{"System"},
	}, func(_ context.Context, _ *struct{}) (*runtimeOutput, error) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		var lastPause uint64
		if m.NumGC > 0 {
			lastPause = m.PauseNs[(m.NumGC+255)%256]
		}
		host, _ := os.Hostname()
		return &runtimeOutput{Body: &runtimeBody{
			Goroutines:    runtime.NumGoroutine(),
			HeapAlloc:     m.HeapAlloc,
			HeapInUse:     m.HeapInuse,
			HeapObjects:   m.HeapObjects,
			NumGC:         m.NumGC,
			LastGCPauseNs: lastPause,
			UptimeSeconds: int64(time.Since(startTime).Seconds()),
			GoVersion:     runtime.Version(),
			GoOS:          runtime.GOOS,
			GoArch:        runtime.GOARCH,
			NumCPU:        runtime.NumCPU(),
			Hostname:      host,
		}}, nil
	})
}
