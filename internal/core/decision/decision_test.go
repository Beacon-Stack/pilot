package decision

import (
	"testing"

	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

func q(res plugin.Resolution, src plugin.Source, rev int) plugin.Quality {
	return plugin.Quality{Resolution: res, Source: src, Revision: rev}
}

var (
	web720    = q(plugin.Resolution720p, plugin.SourceWEBDL, 1)
	web1080   = q(plugin.Resolution1080p, plugin.SourceWEBDL, 1)
	web1080p2 = q(plugin.Resolution1080p, plugin.SourceWEBDL, 2)
	bd1080    = q(plugin.Resolution1080p, plugin.SourceBluRay, 1)
	web2160   = q(plugin.Resolution2160p, plugin.SourceWEBDL, 1)
	hdtv480   = q(plugin.Resolution480p, plugin.SourceHDTV, 1)
)

func profile(cutoff plugin.Quality, upgrades bool, until *plugin.Quality) *quality.Profile {
	return &quality.Profile{Cutoff: cutoff, UpgradeAllowed: upgrades, UpgradeUntil: until}
}

func ptr(v plugin.Quality) *plugin.Quality { return &v }

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name    string
		cand    plugin.Quality
		current *plugin.Quality
		p       *quality.Profile
		wantOK  bool
		wantWhy string
	}{
		{"no file, meets profile", web1080, nil, profile(web1080, false, nil), true, ""},
		{"no file, below profile", web720, nil, profile(web1080, false, nil), false, ReasonBelowProfile},
		{"no file, nil profile accepts", hdtv480, nil, nil, true, ""},
		{"same quality is not an upgrade", web1080, ptr(web1080), profile(web720, true, nil), false, ReasonNotAnUpgrade},
		{"worse quality is not an upgrade", web720, ptr(web1080), profile(web720, true, nil), false, ReasonNotAnUpgrade},
		{"better, current below cutoff", web1080, ptr(web720), profile(web1080, false, nil), true, ""},
		{"better, cutoff met, upgrades off", bd1080, ptr(web1080), profile(web1080, false, nil), false, ReasonUpgradesDisabled},
		{"better, cutoff met, upgrades on", bd1080, ptr(web1080), profile(web1080, true, nil), true, ""},
		{"better, at upgrade-until ceiling", web2160, ptr(bd1080), profile(web1080, true, ptr(bd1080)), false, ReasonUpgradeLimit},
		{"better, below upgrade-until ceiling", bd1080, ptr(web1080), profile(web1080, true, ptr(web2160)), true, ""},
		{"proper of current quality", web1080p2, ptr(web1080), profile(web1080, false, nil), true, ""},
		{"proper already on disk", web1080p2, ptr(web1080p2), profile(web1080, false, nil), false, ReasonNotAnUpgrade},
		{"proper with a different codec is still a proper", plugin.Quality{Resolution: plugin.Resolution1080p, Source: plugin.SourceWEBDL, Codec: "x265", Revision: 2}, ptr(web1080), profile(web1080, false, nil), true, ""},
		{"unknown revision on disk counts as original", web1080p2, ptr(q(plugin.Resolution1080p, plugin.SourceWEBDL, 0)), profile(web1080, false, nil), true, ""},
		{"proper of a lower quality is not an upgrade", q(plugin.Resolution720p, plugin.SourceWEBDL, 2), ptr(web1080), profile(web720, true, nil), false, ReasonNotAnUpgrade},
		{"current quality unknown", bd1080, ptr(plugin.Quality{}), profile(web1080, true, nil), false, ReasonCurrentUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, why := Evaluate(c.cand, c.current, c.p)
			if ok != c.wantOK || why != c.wantWhy {
				t.Fatalf("Evaluate = (%v, %q), want (%v, %q)", ok, why, c.wantOK, c.wantWhy)
			}
		})
	}
}

func TestMeetsProfileMinimum_ResolutionOnly(t *testing.T) {
	p := profile(bd1080, false, nil)
	if !MeetsProfileMinimum(web1080, p) {
		t.Fatal("1080p WEB-DL should meet a 1080p Bluray cutoff (resolution-only gate until B1)")
	}
	if MeetsProfileMinimum(web720, p) {
		t.Fatal("720p should not meet a 1080p cutoff")
	}
	if !MeetsProfileMinimum(hdtv480, profile(plugin.Quality{}, false, nil)) {
		t.Fatal("an unset cutoff accepts everything")
	}
}

// TODO.md's headline case: a "Bluray only" profile used to accept 1080p
// WEB-DL, because the gate only compared resolution against the cutoff.
func TestMeetsProfileMinimum_AllowedListIsTheRule(t *testing.T) {
	blurayOnly := &quality.Profile{
		Cutoff:    bd1080,
		Qualities: []plugin.Quality{bd1080},
	}
	if MeetsProfileMinimum(web1080, blurayOnly) {
		t.Fatal("Bluray-only profile accepted 1080p WEB-DL")
	}
	if !MeetsProfileMinimum(bd1080, blurayOnly) {
		t.Fatal("Bluray-only profile rejected 1080p Bluray")
	}

	// A listed quality below the cutoff is acceptable (the cutoff is where
	// upgrading stops, not a floor), as in Sonarr.
	wide := &quality.Profile{Cutoff: web1080, Qualities: []plugin.Quality{web720, web1080}}
	if !MeetsProfileMinimum(web720, wide) {
		t.Fatal("listed 720p rejected because it is below the cutoff")
	}
	ok, why := Evaluate(web1080, &web720, wide)
	if !ok {
		t.Fatalf("upgrade from listed 720p to 1080p cutoff refused: %s", why)
	}
}
