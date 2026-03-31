package parser

import "strings"

// Parse extracts all metadata from a release name or filename.
// It is a pure function with no external dependencies.
func Parse(input string) ParsedRelease {
	p := ParsedRelease{RawTitle: input}

	// Normalize for quality/audio/edition/markers (dots/underscores → spaces).
	norm := strings.NewReplacer(".", " ", "_", " ").Replace(input)

	// Video quality
	p.Source = parseSource(norm)
	p.Resolution = parseResolution(norm, p.Source)
	p.HDR = parseHDR(norm)
	p.Codec = parseCodec(norm)

	// Audio
	p.AudioCodec = parseAudioCodec(norm)
	p.AudioChannels = parseAudioChannels(norm)

	// Edition
	p.Edition = parseEdition(norm)

	// Release group (operates on raw input, not normalized)
	p.ReleaseGroup = parseReleaseGroup(input)

	// Languages
	p.Languages = parseLanguages(norm)

	// Revision + markers
	p.Revision = parseRevision(norm)
	parseMarkers(norm, &p)

	// Quality name label
	p.QualityName = buildQualityName(p.Resolution, p.Source, p.Codec, p.HDR)

	// Episode info — must run before title extraction so episode tokens
	// can be stripped from the title input.
	p.EpisodeInfo = ParseEpisodeInfo(input)

	// Title + year: strip episode tokens first so they don't pollute the show name.
	titleInput := stripEpisodeTokens(input)
	normalized := normalize(titleInput)
	p.Title, p.Year = extractTitle(normalized)

	return p
}
