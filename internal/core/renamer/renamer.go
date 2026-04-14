// Package renamer applies naming format templates to produce filesystem-safe
// filenames for imported TV episode files.
package renamer

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/beacon-stack/pilot/pkg/plugin"
)

// Default format constants used when a library has no explicit format configured.
const (
	DefaultEpisodeFormat      = "{Series Title} - S{Season:00}E{Episode:00} - {Episode Title} {Quality Full}"
	DefaultDailyEpisodeFormat = "{Series Title} - {Air Date} - {Episode Title} {Quality Full}"
	DefaultAnimeEpisodeFormat = "{Series Title} - S{Season:00}E{Episode:00} - {Absolute Episode:000} - {Episode Title} {Quality Full}"
	DefaultSeriesFolderFormat = "{Series Title} ({Release Year})"
	DefaultSeasonFolderFormat = "Season {Season:00}"
)

// ColonReplacement controls how colons in series/episode titles are handled
// when producing filesystem-safe filenames.
type ColonReplacement string

const (
	// ColonDelete removes colons: "Title: Subtitle" → "Title Subtitle"
	ColonDelete ColonReplacement = "delete"
	// ColonDash replaces colons with a dash: "Title: Subtitle" → "Title- Subtitle"
	ColonDash ColonReplacement = "dash"
	// ColonSpaceDash replaces ": " with " - ": "Title: Subtitle" → "Title - Subtitle"
	ColonSpaceDash ColonReplacement = "space-dash"
)

// Series holds the series-level metadata the renamer needs.
type Series struct {
	Title         string
	OriginalTitle string
	Year          int
}

// Episode holds the episode-level metadata the renamer needs.
type Episode struct {
	SeasonNumber   int
	EpisodeNumber  int
	AbsoluteNumber int
	Title          string
	AirDate        string // "2024-01-15"
}

// ApplyEpisodeFormat returns the formatted filename (without extension) for the
// given series, episode, quality, and format string.
//
// Supported tokens:
//
//	{Series Title}           → series.Title
//	{Series CleanTitle}      → filesystem-safe series title
//	{Original Title}         → series.OriginalTitle
//	{Release Year}           → series.Year
//	{Season:00}              → zero-padded season number (e.g. "01")
//	{Episode:00}             → zero-padded episode number (e.g. "05")
//	{Absolute Episode:000}   → zero-padded absolute number (e.g. "005")
//	{Episode Title}          → episode.Title
//	{Air Date}               → episode.AirDate
//	{Quality Full}           → quality.Name
//	{MediaInfo VideoCodec}   → quality.Codec
func ApplyEpisodeFormat(format string, series Series, episode Episode, quality plugin.Quality, colon ColonReplacement) string {
	r := strings.NewReplacer(
		"{Series Title}", series.Title,
		"{Series CleanTitle}", CleanTitle(series.Title, colon),
		"{Original Title}", series.OriginalTitle,
		"{Release Year}", yearStr(series.Year),
		"{Season:00}", fmt.Sprintf("%02d", episode.SeasonNumber),
		"{season:00}", fmt.Sprintf("%02d", episode.SeasonNumber),
		"{Episode:00}", fmt.Sprintf("%02d", episode.EpisodeNumber),
		"{episode:00}", fmt.Sprintf("%02d", episode.EpisodeNumber),
		"{Absolute Episode:000}", fmt.Sprintf("%03d", episode.AbsoluteNumber),
		"{Episode Title}", episode.Title,
		"{Air Date}", episode.AirDate,
		"{Air-Date}", episode.AirDate,
		"{Quality Full}", quality.Name,
		"{MediaInfo VideoCodec}", string(quality.Codec),
		"{Year}", yearStr(series.Year),
	)
	return sanitize(r.Replace(format))
}

// ApplyFolderFormat returns the series root folder name.
//
// Supported tokens: {Series Title}, {Series CleanTitle}, {Original Title}, {Release Year}.
func ApplyFolderFormat(format string, series Series) string {
	r := strings.NewReplacer(
		"{Series Title}", series.Title,
		"{Series CleanTitle}", CleanTitle(series.Title, ColonSpaceDash),
		"{Original Title}", series.OriginalTitle,
		"{Release Year}", yearStr(series.Year),
		"{Year}", yearStr(series.Year),
	)
	return sanitize(r.Replace(format))
}

// ApplySeasonFolderFormat returns the season sub-folder name.
//
// Supported token: {Season:00}.
func ApplySeasonFolderFormat(format string, seasonNumber int) string {
	r := strings.NewReplacer(
		"{Season:00}", fmt.Sprintf("%02d", seasonNumber),
		"{season:00}", fmt.Sprintf("%02d", seasonNumber),
	)
	return sanitize(r.Replace(format))
}

// DestPath returns the absolute destination path for an imported episode file.
//
//	libraryRoot / ApplyFolderFormat(...) / ApplySeasonFolderFormat(...) / ApplyEpisodeFormat(...) + ext
func DestPath(
	libraryRoot, episodeFormat, seriesFolderFormat, seasonFolderFormat string,
	series Series, episode Episode,
	quality plugin.Quality, colon ColonReplacement,
	ext string,
) string {
	seriesDir := ApplyFolderFormat(seriesFolderFormat, series)
	seasonDir := ApplySeasonFolderFormat(seasonFolderFormat, episode.SeasonNumber)
	filename := ApplyEpisodeFormat(episodeFormat, series, episode, quality, colon) + ext
	return filepath.Join(libraryRoot, seriesDir, seasonDir, filename)
}

// CleanTitle strips characters that are problematic on common filesystems
// while preserving readability. The colon strategy controls how colons
// in the title are handled.
func CleanTitle(title string, colon ColonReplacement) string {
	switch colon {
	case ColonDash:
		title = strings.ReplaceAll(title, ":", "-")
	case ColonSpaceDash:
		title = strings.ReplaceAll(title, ": ", " - ")
		title = strings.ReplaceAll(title, ":", "-")
	default: // ColonDelete
		title = strings.ReplaceAll(title, ":", " ")
	}
	title = invalidCharsRe.ReplaceAllString(title, "")
	title = multiSpaceRe.ReplaceAllString(title, " ")
	return strings.TrimSpace(title)
}

// sanitize makes a string safe to use as a filename: removes path separators
// and collapses whitespace. Does not strip colons or other title chars so
// that the full {Series Title} variable retains its value; use CleanTitle for that.
func sanitize(s string) string {
	s = strings.NewReplacer("/", "", "\x00", "").Replace(s)
	s = multiSpaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func yearStr(y int) string {
	if y == 0 {
		return ""
	}
	return fmt.Sprintf("%d", y)
}

var (
	invalidCharsRe = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	multiSpaceRe   = regexp.MustCompile(`\s{2,}`)
)
