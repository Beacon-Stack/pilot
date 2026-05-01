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
