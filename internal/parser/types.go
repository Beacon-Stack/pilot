// Package parser implements a comprehensive, unified release name parser.
// It extracts title, year, video quality, audio, edition, release group,
// language, and marker information from a single Parse() call.
package parser

import "github.com/screenarr/screenarr/pkg/plugin"

// Language identifies a detected language tag.
type Language string

const (
	LangEnglish    Language = "english"
	LangFrench     Language = "french"
	LangGerman     Language = "german"
	LangSpanish    Language = "spanish"
	LangItalian    Language = "italian"
	LangPortuguese Language = "portuguese"
	LangRussian    Language = "russian"
	LangJapanese   Language = "japanese"
	LangKorean     Language = "korean"
	LangChinese    Language = "chinese"
	LangHindi      Language = "hindi"
	LangArabic     Language = "arabic"
	LangDutch      Language = "dutch"
	LangNordic     Language = "nordic"
	LangSwedish    Language = "swedish"
	LangDanish     Language = "danish"
	LangFinnish    Language = "finnish"
	LangNorwegian  Language = "norwegian"
	LangPolish     Language = "polish"
	LangTurkish    Language = "turkish"
	LangMulti      Language = "multi"
)

// Revision tracks PROPER/REPACK/REAL status.
type Revision struct {
	Version int  // 1 = original, 2 = PROPER/REPACK, 3 = PROPER2, etc.
	IsReal  bool // REAL tag present
}

// ParsedRelease is the comprehensive result of parsing a release name.
type ParsedRelease struct {
	// Identity
	Title        string
	Year         int
	ReleaseGroup string

	// Episode info (TV-specific)
	EpisodeInfo EpisodeInfo

	// Video quality
	Resolution plugin.Resolution
	Source     plugin.Source
	Codec      plugin.Codec
	HDR        plugin.HDRFormat

	// Audio
	AudioCodec    plugin.AudioCodec
	AudioChannels plugin.AudioChannels

	// Edition
	Edition string // canonical name, e.g. "Director's Cut"

	// Languages
	Languages []Language

	// Revision
	Revision Revision

	// Markers / Flags
	IsHybrid       bool
	Is3D           bool
	IsHardcodedSub bool
	IsScene        bool
	IsSample       bool
	IsInternal     bool
	IsLimited      bool
	IsSubbed       bool
	IsDubbed       bool
	IsProper       bool
	IsRepack       bool

	// Source metadata
	RawTitle string // original input before normalization

	// Name is the human-readable quality label (e.g. "Bluray-1080p x265 HDR10").
	QualityName string
}

// Quality returns the backward-compatible plugin.Quality struct.
func (p ParsedRelease) Quality() plugin.Quality {
	return plugin.Quality{
		Resolution:    p.Resolution,
		Source:        p.Source,
		Codec:         p.Codec,
		HDR:           p.HDR,
		AudioCodec:    p.AudioCodec,
		AudioChannels: p.AudioChannels,
		Name:          p.QualityName,
	}
}
