package v1

import (
	"database/sql"
	"strings"
	"testing"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// TestValidateReimportGrab covers the eligibility rules for the
// re-import endpoint. Pinning these so a refactor that loosens (e.g.
// allowing in-progress grabs) or tightens (e.g. requiring completed_at)
// the gate has to update the test alongside the code.
func TestValidateReimportGrab(t *testing.T) {
	cases := []struct {
		name      string
		grab      db.GrabHistory
		wantError bool
		// errContains is a substring expected in the error message;
		// asserts the user sees a useful reason, not a generic 409.
		errContains string
	}{
		{
			name: "completed with info_hash is eligible",
			grab: db.GrabHistory{
				DownloadStatus: "completed",
				InfoHash:       sql.NullString{String: "abc123", Valid: true},
			},
			wantError: false,
		},
		{
			name: "in-progress grab is rejected",
			grab: db.GrabHistory{
				DownloadStatus: "downloading",
				InfoHash:       sql.NullString{String: "abc123", Valid: true},
			},
			wantError:   true,
			errContains: "downloading",
		},
		{
			name: "queued grab is rejected",
			grab: db.GrabHistory{
				DownloadStatus: "queued",
				InfoHash:       sql.NullString{String: "abc123", Valid: true},
			},
			wantError:   true,
			errContains: "queued",
		},
		{
			name: "failed grab is rejected",
			grab: db.GrabHistory{
				DownloadStatus: "failed",
				InfoHash:       sql.NullString{String: "abc123", Valid: true},
			},
			wantError:   true,
			errContains: "failed",
		},
		{
			name: "completed but missing info_hash is rejected",
			grab: db.GrabHistory{
				DownloadStatus: "completed",
				InfoHash:       sql.NullString{Valid: false},
			},
			wantError:   true,
			errContains: "info_hash",
		},
		{
			name: "completed but empty info_hash string is rejected",
			grab: db.GrabHistory{
				DownloadStatus: "completed",
				InfoHash:       sql.NullString{String: "", Valid: true},
			},
			wantError:   true,
			errContains: "info_hash",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validateReimportGrab(tc.grab)
			if (got != "") != tc.wantError {
				t.Fatalf("got error=%q, wantError=%v", got, tc.wantError)
			}
			if tc.errContains != "" && !strings.Contains(got, tc.errContains) {
				t.Errorf("error message %q does not contain %q", got, tc.errContains)
			}
		})
	}
}
