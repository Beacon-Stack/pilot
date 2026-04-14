package renamer

import (
	"testing"

	"github.com/beacon-stack/pilot/pkg/plugin"
)

func TestApplyEpisodeFormat(t *testing.T) {
	series := Series{Title: "Family Guy", Year: 1999}
	episode := Episode{SeasonNumber: 9, EpisodeNumber: 8, Title: "New Kidney in Town"}
	quality := plugin.Quality{Name: "WEBDL-720p"}

	tests := []struct {
		name   string
		format string
		series Series
		ep     Episode
		qual   plugin.Quality
		colon  ColonReplacement
		want   string
	}{
		// Basic format — clean minimal
		{
			name:   "default format",
			format: DefaultEpisodeFormat,
			series: series, ep: episode, qual: quality, colon: ColonSpaceDash,
			want: "Family Guy - S09E08 - New Kidney in Town WEBDL-720p",
		},
		// Lowercase tokens (DB migration uses lowercase)
		{
			name:   "lowercase season/episode tokens",
			format: "{Series Title} - S{season:00}E{episode:00} - {Episode Title}",
			series: series, ep: episode, qual: quality, colon: ColonSpaceDash,
			want: "Family Guy - S09E08 - New Kidney in Town",
		},
		// Year token
		{
			name:   "year token",
			format: "{Series Title} ({Year}) - S{Season:00}E{Episode:00}",
			series: series, ep: episode, qual: quality, colon: ColonSpaceDash,
			want: "Family Guy (1999) - S09E08",
		},
		// Release Year token
		{
			name:   "release year token",
			format: "{Series Title} ({Release Year}) - S{Season:00}E{Episode:00}",
			series: series, ep: episode, qual: quality, colon: ColonSpaceDash,
			want: "Family Guy (1999) - S09E08",
		},
		// Anime format with absolute numbering
		{
			name:   "anime format",
			format: DefaultAnimeEpisodeFormat,
			series: series,
			ep:     Episode{SeasonNumber: 1, EpisodeNumber: 42, AbsoluteNumber: 42, Title: "Test"},
			qual:   quality, colon: ColonSpaceDash,
			want: "Family Guy - S01E42 - 042 - Test WEBDL-720p",
		},
		// Daily format with air date
		{
			name:   "daily format",
			format: DefaultDailyEpisodeFormat,
			series: series,
			ep:     Episode{Title: "Interview with Guest", AirDate: "2024-03-15"},
			qual:   quality, colon: ColonSpaceDash,
			want: "Family Guy - 2024-03-15 - Interview with Guest WEBDL-720p",
		},
		// Air-Date token (hyphenated)
		{
			name:   "air-date hyphenated token",
			format: "{Series Title} - {Air-Date} - {Episode Title}",
			series: series,
			ep:     Episode{Title: "Test", AirDate: "2024-01-01"},
			qual:   quality, colon: ColonSpaceDash,
			want: "Family Guy - 2024-01-01 - Test",
		},
		// Colon in raw title — preserved (use CleanTitle to strip)
		{
			name:   "colon in raw title preserved",
			format: "{Series Title} - S{Season:00}E{Episode:00} - {Episode Title}",
			series: Series{Title: "CSI: Vegas", Year: 2021},
			ep:     Episode{SeasonNumber: 1, EpisodeNumber: 1, Title: "Legacy: Part 1"},
			qual:   quality, colon: ColonDelete,
			want: "CSI: Vegas - S01E01 - Legacy: Part 1",
		},
		// CleanTitle applies colon strategy
		{
			name:   "clean title colon dash",
			format: "{Series CleanTitle} - S{Season:00}E{Episode:00}",
			series: Series{Title: "CSI: Vegas", Year: 2021},
			ep:     episode, qual: quality, colon: ColonDash,
			want: "CSI- Vegas - S09E08",
		},
		// Colon in title — space dash
		{
			name:   "colon space dash",
			format: "{Series CleanTitle} - S{Season:00}E{Episode:00}",
			series: Series{Title: "CSI: Vegas", Year: 2021},
			ep:     episode, qual: quality, colon: ColonSpaceDash,
			want: "CSI - Vegas - S09E08",
		},
		// Empty quality name
		{
			name:   "empty quality",
			format: "{Series Title} - S{Season:00}E{Episode:00} - {Episode Title} {Quality Full}",
			series: series, ep: episode, qual: plugin.Quality{}, colon: ColonSpaceDash,
			want: "Family Guy - S09E08 - New Kidney in Town",
		},
		// Zero year
		{
			name:   "zero year omitted",
			format: "{Series Title} ({Release Year})",
			series: Series{Title: "Unknown Show", Year: 0},
			ep:     episode, qual: quality, colon: ColonSpaceDash,
			want: "Unknown Show ()",
		},
		// Special characters in episode title
		{
			name:   "special chars in episode title",
			format: "{Series Title} - S{Season:00}E{Episode:00} - {Episode Title}",
			series: series,
			ep:     Episode{SeasonNumber: 1, EpisodeNumber: 1, Title: "Who Killed Sara?"},
			qual:   quality, colon: ColonSpaceDash,
			want: "Family Guy - S01E01 - Who Killed Sara?",
		},
		// Forward slash in title (must be removed)
		{
			name:   "slash in title removed",
			format: "{Series Title} - S{Season:00}E{Episode:00} - {Episode Title}",
			series: Series{Title: "He/She", Year: 2020},
			ep:     Episode{SeasonNumber: 1, EpisodeNumber: 1, Title: "Begin/End"},
			qual:   quality, colon: ColonSpaceDash,
			want: "HeShe - S01E01 - BeginEnd",
		},
		// High season/episode numbers
		{
			name:   "high numbers",
			format: "{Series Title} - S{Season:00}E{Episode:00}",
			series: series,
			ep:     Episode{SeasonNumber: 35, EpisodeNumber: 100},
			qual:   quality, colon: ColonSpaceDash,
			want: "Family Guy - S35E100",
		},
		// MediaInfo codec token
		{
			name:   "mediainfo codec",
			format: "{Series Title} [{MediaInfo VideoCodec}]",
			series: series, ep: episode,
			qual:  plugin.Quality{Codec: "x265"},
			colon: ColonSpaceDash,
			want:  "Family Guy [x265]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyEpisodeFormat(tt.format, tt.series, tt.ep, tt.qual, tt.colon)
			if got != tt.want {
				t.Errorf("ApplyEpisodeFormat(%q)\n  got  %q\n  want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestApplyFolderFormat(t *testing.T) {
	tests := []struct {
		name   string
		format string
		series Series
		want   string
	}{
		{
			name:   "default format",
			format: DefaultSeriesFolderFormat,
			series: Series{Title: "Breaking Bad", Year: 2008},
			want:   "Breaking Bad (2008)",
		},
		{
			name:   "year token alias",
			format: "{Series Title} ({Year})",
			series: Series{Title: "Breaking Bad", Year: 2008},
			want:   "Breaking Bad (2008)",
		},
		{
			name:   "clean title with colon",
			format: "{Series CleanTitle} ({Release Year})",
			series: Series{Title: "CSI: Vegas", Year: 2021},
			want:   "CSI - Vegas (2021)",
		},
		{
			name:   "original title",
			format: "{Original Title}",
			series: Series{OriginalTitle: "La Casa de Papel"},
			want:   "La Casa de Papel",
		},
		{
			name:   "slash in title",
			format: "{Series Title}",
			series: Series{Title: "His/Her Story"},
			want:   "HisHer Story",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyFolderFormat(tt.format, tt.series)
			if got != tt.want {
				t.Errorf("ApplyFolderFormat(%q)\n  got  %q\n  want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestApplySeasonFolderFormat(t *testing.T) {
	tests := []struct {
		name   string
		format string
		season int
		want   string
	}{
		{"default", DefaultSeasonFolderFormat, 1, "Season 01"},
		{"lowercase token", "Season {season:00}", 9, "Season 09"},
		{"uppercase token", "Season {Season:00}", 9, "Season 09"},
		{"double digit", "Season {Season:00}", 15, "Season 15"},
		{"bare number", "{Season:00}", 3, "03"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplySeasonFolderFormat(tt.format, tt.season)
			if got != tt.want {
				t.Errorf("ApplySeasonFolderFormat(%q, %d)\n  got  %q\n  want %q", tt.format, tt.season, got, tt.want)
			}
		})
	}
}

func TestDestPath(t *testing.T) {
	tests := []struct {
		name      string
		root      string
		epFmt     string
		seriesFmt string
		seasonFmt string
		series    Series
		ep        Episode
		quality   plugin.Quality
		colon     ColonReplacement
		ext       string
		want      string
	}{
		{
			name:      "full path clean minimal",
			root:      "/media/tv",
			epFmt:     "{Series Title} - S{Season:00}E{Episode:00} - {Episode Title}",
			seriesFmt: "{Series Title} ({Release Year})",
			seasonFmt: "Season {Season:00}",
			series:    Series{Title: "Family Guy", Year: 1999},
			ep:        Episode{SeasonNumber: 9, EpisodeNumber: 8, Title: "New Kidney in Town"},
			quality:   plugin.Quality{},
			colon:     ColonSpaceDash,
			ext:       ".mkv",
			want:      "/media/tv/Family Guy (1999)/Season 09/Family Guy - S09E08 - New Kidney in Town.mkv",
		},
		{
			name:      "colon in series title",
			root:      "/media/tv",
			epFmt:     "{Series CleanTitle} - S{Season:00}E{Episode:00}",
			seriesFmt: "{Series CleanTitle} ({Release Year})",
			seasonFmt: "Season {Season:00}",
			series:    Series{Title: "Star Trek: Discovery", Year: 2017},
			ep:        Episode{SeasonNumber: 1, EpisodeNumber: 1},
			quality:   plugin.Quality{},
			colon:     ColonSpaceDash,
			ext:       ".mkv",
			want:      "/media/tv/Star Trek - Discovery (2017)/Season 01/Star Trek - Discovery - S01E01.mkv",
		},
		{
			name:      "DB default format (lowercase tokens)",
			root:      "/data/tv",
			epFmt:     "{Series Title} - S{season:00}E{episode:00} - {Episode Title} {Quality Full}",
			seriesFmt: "{Series Title} ({Year})",
			seasonFmt: "Season {season:00}",
			series:    Series{Title: "The Office", Year: 2005},
			ep:        Episode{SeasonNumber: 2, EpisodeNumber: 5, Title: "Halloween"},
			quality:   plugin.Quality{Name: "Bluray-1080p"},
			colon:     ColonSpaceDash,
			ext:       ".mkv",
			want:      "/data/tv/The Office (2005)/Season 02/The Office - S02E05 - Halloween Bluray-1080p.mkv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DestPath(tt.root, tt.epFmt, tt.seriesFmt, tt.seasonFmt, tt.series, tt.ep, tt.quality, tt.colon, tt.ext)
			if got != tt.want {
				t.Errorf("DestPath()\n  got  %q\n  want %q", got, tt.want)
			}
		})
	}
}

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		colon ColonReplacement
		want  string
	}{
		{"basic", "Normal Title", ColonDelete, "Normal Title"},
		{"colon delete", "CSI: Vegas", ColonDelete, "CSI Vegas"},
		{"colon dash", "CSI: Vegas", ColonDash, "CSI- Vegas"},
		{"colon space-dash", "CSI: Vegas", ColonSpaceDash, "CSI - Vegas"},
		{"colon no space space-dash", "Code:Breaker", ColonSpaceDash, "Code-Breaker"},
		{"multiple colons", "A: B: C", ColonSpaceDash, "A - B - C"},
		{"slash removed", "AC/DC", ColonDelete, "ACDC"},
		{"backslash removed", "Back\\Slash", ColonDelete, "BackSlash"},
		{"question mark removed", "Who?", ColonDelete, "Who"},
		{"asterisk removed", "Star*", ColonDelete, "Star"},
		{"angle brackets removed", "<Title>", ColonDelete, "Title"},
		{"pipe removed", "A|B", ColonDelete, "AB"},
		{"quotes removed", `Say "Hello"`, ColonDelete, "Say Hello"},
		{"multiple spaces collapsed", "Too   Many   Spaces", ColonDelete, "Too Many Spaces"},
		{"leading/trailing spaces trimmed", "  Padded  ", ColonDelete, "Padded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanTitle(tt.title, tt.colon)
			if got != tt.want {
				t.Errorf("CleanTitle(%q, %q)\n  got  %q\n  want %q", tt.title, tt.colon, got, tt.want)
			}
		})
	}
}
