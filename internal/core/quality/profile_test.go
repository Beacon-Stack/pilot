package quality_test

import (
	"testing"

	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func qSD() plugin.Quality {
	return plugin.Quality{Resolution: plugin.ResolutionSD, Source: plugin.SourceDVD, Codec: plugin.CodecXVID}
}

func q720p() plugin.Quality {
	return plugin.Quality{Resolution: plugin.Resolution720p, Source: plugin.SourceWEBDL, Codec: plugin.CodecX264}
}

func q1080pWEB() plugin.Quality {
	return plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceWEBDL, Codec: plugin.CodecX264}
}

func q1080pBluRay() plugin.Quality {
	return plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceBluRay, Codec: plugin.CodecX265}
}

func q1080pRemux() plugin.Quality {
	return plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceRemux, Codec: plugin.CodecX265}
}

func ptr(q plugin.Quality) *plugin.Quality { return &q }

// standardProfile builds a typical HD profile: accepts 720p WEB through
// 1080p Remux, cutoff at 1080p WEB-DL, upgrades allowed up to 1080p Remux.
func standardProfile(upgradeAllowed bool, upgradeUntil *plugin.Quality) *quality.Profile {
	return &quality.Profile{
		ID:     "hd",
		Name:   "HD",
		Cutoff: q1080pWEB(),
		Qualities: []plugin.Quality{
			q1080pRemux(),
			q1080pBluRay(),
			q1080pWEB(),
			q720p(),
		},
		UpgradeAllowed: upgradeAllowed,
		UpgradeUntil:   upgradeUntil,
	}
}

// ── IsUpgrade tests ───────────────────────────────────────────────────────────

func TestIsUpgrade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		profile        *quality.Profile
		releaseQuality plugin.Quality
		currentQuality plugin.Quality
		want           bool
	}{
		{
			name:           "strictly better, no ceiling → upgrade",
			profile:        standardProfile(true, nil),
			releaseQuality: q1080pBluRay(),
			currentQuality: q1080pWEB(),
			want:           true,
		},
		{
			name:           "equal quality → not an upgrade",
			profile:        standardProfile(true, nil),
			releaseQuality: q1080pWEB(),
			currentQuality: q1080pWEB(),
			want:           false,
		},
		{
			name:           "worse quality → not an upgrade",
			profile:        standardProfile(true, nil),
			releaseQuality: q720p(),
			currentQuality: q1080pWEB(),
			want:           false,
		},
		{
			name:           "better, current below ceiling → upgrade",
			profile:        standardProfile(true, ptr(q1080pRemux())),
			releaseQuality: q1080pBluRay(),
			currentQuality: q1080pWEB(),
			want:           true,
		},
		{
			name:           "better, current at ceiling → no upgrade",
			profile:        standardProfile(true, ptr(q1080pRemux())),
			releaseQuality: q1080pRemux(),
			currentQuality: q1080pRemux(),
			want:           false,
		},
		{
			name:           "better, current above ceiling → no upgrade",
			profile:        standardProfile(true, ptr(q1080pBluRay())),
			releaseQuality: q1080pRemux(),
			currentQuality: q1080pRemux(),
			want:           false,
		},
		{
			name:           "current below ceiling by 2 levels → upgrade",
			profile:        standardProfile(true, ptr(q1080pRemux())),
			releaseQuality: q1080pWEB(),
			currentQuality: q720p(),
			want:           true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.profile.IsUpgrade(tc.releaseQuality, tc.currentQuality)
			if got != tc.want {
				t.Errorf("IsUpgrade() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ── AllowedQualities tests ────────────────────────────────────────────────────

func TestAllowedQualities(t *testing.T) {
	t.Parallel()

	p := standardProfile(false, nil)
	got := p.AllowedQualities()

	if len(got) != len(p.Qualities) {
		t.Fatalf("AllowedQualities() returned %d items, want %d", len(got), len(p.Qualities))
	}

	// Verify it's a copy, not a reference.
	got[0] = qSD()
	if p.Qualities[0].Score() == qSD().Score() {
		t.Error("AllowedQualities() returned a reference to internal slice — expected a copy")
	}
}
