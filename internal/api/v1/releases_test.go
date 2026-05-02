package v1

import (
	"testing"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// TestIndexLatestGrabsByGUID covers the manual-search guardrail's
// most-recent-grab-per-GUID lookup. Pinning the GrabbedAt string
// comparison so a refactor to numeric/time.Time can't accidentally
// reverse the order.
func TestIndexLatestGrabsByGUID(t *testing.T) {
	cases := []struct {
		name string
		rows []db.GrabHistory
		want map[string]string // guid -> expected ID
	}{
		{
			name: "empty input returns empty map",
			rows: nil,
			want: map[string]string{},
		},
		{
			name: "single row indexes by guid",
			rows: []db.GrabHistory{
				{ID: "g1", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-29T12:00:00Z"},
			},
			want: map[string]string{"guid-a": "g1"},
		},
		{
			name: "two rows different guids both indexed",
			rows: []db.GrabHistory{
				{ID: "g1", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-29T12:00:00Z"},
				{ID: "g2", ReleaseGuid: "guid-b", GrabbedAt: "2026-04-30T12:00:00Z"},
			},
			want: map[string]string{"guid-a": "g1", "guid-b": "g2"},
		},
		{
			name: "two rows same guid keeps most recent",
			rows: []db.GrabHistory{
				{ID: "older", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-01T12:00:00Z"},
				{ID: "newer", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-29T12:00:00Z"},
			},
			want: map[string]string{"guid-a": "newer"},
		},
		{
			name: "order independent — newer first then older",
			rows: []db.GrabHistory{
				{ID: "newer", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-29T12:00:00Z"},
				{ID: "older", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-01T12:00:00Z"},
			},
			want: map[string]string{"guid-a": "newer"},
		},
		{
			// Headline regression case: a failed grab must not surface
			// "already grabbed" — the user clearly wants to try again.
			name: "failed-only grab is excluded",
			rows: []db.GrabHistory{
				{ID: "f1", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-29T12:00:00Z", DownloadStatus: "failed"},
			},
			want: map[string]string{},
		},
		{
			name: "stalled grab is excluded",
			rows: []db.GrabHistory{
				{ID: "s1", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-29T12:00:00Z", DownloadStatus: "stalled"},
			},
			want: map[string]string{},
		},
		{
			name: "removed grab is excluded",
			rows: []db.GrabHistory{
				{ID: "r1", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-29T12:00:00Z", DownloadStatus: "removed"},
			},
			want: map[string]string{},
		},
		{
			name: "completed grab is included",
			rows: []db.GrabHistory{
				{ID: "c1", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-29T12:00:00Z", DownloadStatus: "completed"},
			},
			want: map[string]string{"guid-a": "c1"},
		},
		{
			name: "in-progress (queued/downloading) grab is included",
			rows: []db.GrabHistory{
				{ID: "q1", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-29T12:00:00Z", DownloadStatus: "queued"},
				{ID: "d1", ReleaseGuid: "guid-b", GrabbedAt: "2026-04-29T12:00:00Z", DownloadStatus: "downloading"},
			},
			want: map[string]string{"guid-a": "q1", "guid-b": "d1"},
		},
		{
			// When a user retried after a failure, the older successful
			// grab is still the "did this complete?" signal we want to
			// surface — the failed retry is filtered out, so the older
			// completed wins.
			name: "completed older + failed newer keeps the completed",
			rows: []db.GrabHistory{
				{ID: "c-old", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-01T12:00:00Z", DownloadStatus: "completed"},
				{ID: "f-new", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-29T12:00:00Z", DownloadStatus: "failed"},
			},
			want: map[string]string{"guid-a": "c-old"},
		},
		{
			// The opposite: a successful retry after a failure. Newer
			// completed wins; failed is skipped.
			name: "failed older + completed newer keeps the completed",
			rows: []db.GrabHistory{
				{ID: "f-old", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-01T12:00:00Z", DownloadStatus: "failed"},
				{ID: "c-new", ReleaseGuid: "guid-a", GrabbedAt: "2026-04-29T12:00:00Z", DownloadStatus: "completed"},
			},
			want: map[string]string{"guid-a": "c-new"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := indexLatestGrabsByGUID(tc.rows)
			if len(got) != len(tc.want) {
				t.Fatalf("len: got %d, want %d (%+v)", len(got), len(tc.want), got)
			}
			for guid, wantID := range tc.want {
				row, ok := got[guid]
				if !ok {
					t.Errorf("missing guid %q", guid)
					continue
				}
				if row.ID != wantID {
					t.Errorf("guid %q: got id=%q, want id=%q", guid, row.ID, wantID)
				}
			}
		})
	}
}
