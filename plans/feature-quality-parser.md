# Feature: Quality Parser

**Status: DONE** (already implemented — verified 2026-03-30)

Screenarr's `internal/parser/video.go` + `audio.go` already extract resolution,
source, codec, HDR, audio codec, and audio channels. `parser.Parse()` returns
a `ParsedRelease` with all quality fields. Quality profile matching exists in
`internal/core/quality/profile.go` (`WantRelease`). No separate wrapper needed.

## Context

Luminarr has an 18KB quality parser (`internal/core/quality/parser.go`) that
uses compiled regexes to extract resolution, source, codec, HDR, and audio
from release titles. This feeds into custom format scoring and quality profile
matching. Screenarr removed this file entirely — the parser module has
`episode.go` for TV-specific parsing but no quality extraction.

## Assessment

Before implementing, check what Screenarr's `internal/parser/` already does.
Luminarr's parser package (`internal/parser/`) handles video quality
(resolution, source, codec, HDR) in `video.go`. The _separate_
`internal/core/quality/parser.go` in Luminarr is a higher-level layer that
takes parsed output and maps it to quality profile entries.

**Key question**: Does Screenarr's `internal/parser/video.go` already extract
Resolution, Source, Codec, and HDR? If yes, the missing piece is only the
quality profile matching layer, not the regex parsing itself.

## Files to Check

1. `internal/parser/video.go` — does it export Resolution, Source, Codec, HDR?
2. `internal/parser/parser.go` — does `Parse()` return a struct with quality fields?
3. `internal/core/quality/profile.go` — does it have scoring/matching logic?
4. `internal/core/quality/definitions.go` — quality definition mappings

## What to Port (if needed)

### `internal/core/quality/parser.go` (~500 lines)

Port from Luminarr. Key components:

**Compiled regex patterns** (package-level `var`):
- Resolution: 2160p/4K/UHD, 1080p, 720p, 576p, 480p
- Source: REMUX, BluRay, WEB-DL, WEBRip, HDTV, DVD variants, CAM, TS
- HDR: DolbyVision, HDR10+, HDR10, HLG
- Codec: x265/HEVC, x264/H.264, AV1, XviD
- Audio: TrueHD Atmos, TrueHD, DTS-X, DTS-HD MA, DTS, Atmos, EAC3, AC3, AAC

**Functions**:
- `ParseQuality(title string) plugin.Quality` — main entry point
- `ParseReleaseGroup(title string) string` — extract scene group name
- Internal helpers for each dimension

### `internal/core/quality/parser_test.go` (~hundreds of lines)

Port test cases covering:
- Standard releases: "Movie.2024.1080p.BluRay.x264-GROUP"
- Audio combinations: TrueHD Atmos, DTS-HD MA
- Edge cases: partial matches, ambiguous titles
- Release group extraction with bracket variants

## Verification

1. `go test ./internal/core/quality/ -v`
2. `make check` passes
3. Existing custom format and quality profile features still work
