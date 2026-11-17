package sonarrimport

import (
	"testing"

	"github.com/beacon-stack/pilot/pkg/plugin"
)

// Regression: the import copied every quality in the Sonarr profile and
// ignored the allowed flag, so every imported profile allowed everything.
func TestMapProfile_KeepsOnlyAllowedQualities(t *testing.T) {
	p := sonarrProfile{
		Name:   "HD-1080p",
		Cutoff: 7,
		Items: []sonarrProfileItem{
			{Quality: sonarrProfileQuality{ID: 1, Name: "SDTV"}, Allowed: false},
			{Quality: sonarrProfileQuality{ID: 9, Name: "HDTV-1080p"}, Allowed: false},
			{Allowed: true, Items: []sonarrProfileItem{ // a group: WEB 1080p
				{Quality: sonarrProfileQuality{ID: 3, Name: "WEBDL-1080p"}, Allowed: true},
				{Quality: sonarrProfileQuality{ID: 15, Name: "WEBRip-1080p"}, Allowed: true},
			}},
			{Quality: sonarrProfileQuality{ID: 7, Name: "Bluray-1080p"}, Allowed: true},
		},
	}
	req := mapProfile(p)

	got := map[plugin.Source]bool{}
	for _, q := range req.Qualities {
		got[q.Source] = true
	}
	if len(req.Qualities) != 3 || !got[plugin.SourceWEBDL] || !got[plugin.SourceWEBRip] || !got[plugin.SourceBluRay] {
		t.Fatalf("qualities = %+v, want WEBDL, WEBRip and Bluray 1080p only", req.Qualities)
	}
	if got[plugin.SourceHDTV] {
		t.Fatal("a disallowed quality was imported")
	}
	if req.Cutoff.Source != plugin.SourceBluRay {
		t.Fatalf("cutoff = %+v, want Bluray-1080p", req.Cutoff)
	}
}
