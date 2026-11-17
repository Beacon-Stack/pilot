package quality_test

import (
	"testing"

	"github.com/beacon-stack/pilot/internal/core/quality"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

func qual(res plugin.Resolution, src plugin.Source, codec plugin.Codec) plugin.Quality {
	return plugin.Quality{Resolution: res, Source: src, Codec: codec}
}

// Allows matches on resolution + source (Sonarr's notion of a quality).
// The old isAllowed compared exact Score, codec included, and rejected
// almost every real release.
func TestProfileAllows(t *testing.T) {
	blurayOnly := &quality.Profile{Qualities: []plugin.Quality{
		qual(plugin.Resolution1080p, plugin.SourceBluRay, ""),
		qual(plugin.Resolution2160p, plugin.SourceBluRay, ""),
	}}
	webWithFallback := &quality.Profile{Qualities: []plugin.Quality{
		qual(plugin.Resolution1080p, plugin.SourceWEBDL, ""),
		qual(plugin.Resolution1080p, plugin.SourceWEBRip, ""),
	}}
	cases := []struct {
		name string
		p    *quality.Profile
		q    plugin.Quality
		want bool
	}{
		{"bluray-only accepts bluray x264", blurayOnly, qual(plugin.Resolution1080p, plugin.SourceBluRay, "x264"), true},
		{"bluray-only accepts bluray x265", blurayOnly, qual(plugin.Resolution1080p, plugin.SourceBluRay, "x265"), true},
		{"bluray-only rejects WEB-DL", blurayOnly, qual(plugin.Resolution1080p, plugin.SourceWEBDL, "x264"), false},
		{"bluray-only rejects 720p bluray", blurayOnly, qual(plugin.Resolution720p, plugin.SourceBluRay, ""), false},
		{"WEBRip accepted as listed fallback", webWithFallback, qual(plugin.Resolution1080p, plugin.SourceWEBRip, ""), true},
		{"HDTV not listed", webWithFallback, qual(plugin.Resolution1080p, plugin.SourceHDTV, ""), false},
		{"unknown source matches on resolution", webWithFallback, qual(plugin.Resolution1080p, "", ""), true},
		{"unknown source, wrong resolution", webWithFallback, qual(plugin.Resolution720p, "", ""), false},
		{"unknown resolution matches on source", webWithFallback, qual("", plugin.SourceWEBDL, ""), true},
		{"nothing known is not allowed", webWithFallback, plugin.Quality{}, false},
		{"explicit unknown values count as unknown", webWithFallback, qual(plugin.Resolution1080p, plugin.SourceUnknown, ""), true},
		{"empty list allows everything", &quality.Profile{}, qual(plugin.Resolution480p, plugin.SourceHDTV, ""), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.p.Allows(c.q); got != c.want {
				t.Fatalf("Allows(%+v) = %v, want %v", c.q, got, c.want)
			}
		})
	}
}
