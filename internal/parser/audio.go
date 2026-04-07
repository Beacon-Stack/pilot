package parser

import (
	"regexp"

	"github.com/beacon-stack/pilot/pkg/plugin"
)

var (
	// Audio codec — most specific first.
	reTrueHDAtmos = regexp.MustCompile(`(?i)truehd[\s._-]?atmos|truehd\b.*\batmos`)
	reTrueHD      = regexp.MustCompile(`(?i)\btruehd\b`)
	reDTSX        = regexp.MustCompile(`(?i)\bdts[\s._-]?x\b`)
	reDTSHDMA     = regexp.MustCompile(`(?i)\bdts[\s._-]?hd[\s._-]?(?:ma|master[\s._-]?audio)\b`)
	reDTSHD       = regexp.MustCompile(`(?i)\bdts[\s._-]?hd\b`)
	reDTS         = regexp.MustCompile(`(?i)\bdts\b`)
	reAtmos       = regexp.MustCompile(`(?i)\batmos\b`)
	reEAC3        = regexp.MustCompile(`(?i)(?:\bddp|\bdd\+|\bddplus\b|\beac[\s._-]?3\b|\be[\s._-]ac[\s._-]?3\b)`)
	reAC3         = regexp.MustCompile(`(?i)(?:\bdd\d|\bdd\b|\bac[\s._-]?3\b)`)
	reAAC         = regexp.MustCompile(`(?i)\baac`)
	reFLAC        = regexp.MustCompile(`(?i)\bflac\b`)
	rePCM         = regexp.MustCompile(`(?i)\bl?pcm\b`)
	reMP3         = regexp.MustCompile(`(?i)\bmp3\b`)
	reOpus        = regexp.MustCompile(`(?i)\bopus\b`)

	// Audio channels — uses [^\d] instead of \b for leading anchor to handle
	// combined tokens like DD5.1 (normalized to "DD5 1").
	reCh71 = regexp.MustCompile(`(?i)(?:(?:[^\d]|^)7[\s.]1(?:\b|$)|\b8ch\b)`)
	reCh51 = regexp.MustCompile(`(?i)(?:(?:[^\d]|^)5[\s.]1(?:\b|$)|\b6ch\b)`)
	reCh20 = regexp.MustCompile(`(?i)(?:(?:[^\d]|^)2[\s.]0(?:\b|$)|\bstereo\b|\b2ch\b)`)
	reCh10 = regexp.MustCompile(`(?i)(?:(?:[^\d]|^)1[\s.]0(?:\b|$)|\bmono\b|\b1ch\b)`)
)

func parseAudioCodec(norm string) plugin.AudioCodec {
	switch {
	case reTrueHDAtmos.MatchString(norm):
		return plugin.AudioCodecTrueHDAtmos
	case reTrueHD.MatchString(norm):
		return plugin.AudioCodecTrueHD
	case reDTSX.MatchString(norm):
		return plugin.AudioCodecDTSX
	case reDTSHDMA.MatchString(norm):
		return plugin.AudioCodecDTSHDMA
	case reDTSHD.MatchString(norm):
		return plugin.AudioCodecDTSHD
	case reDTS.MatchString(norm):
		return plugin.AudioCodecDTS
	case reEAC3.MatchString(norm):
		if reAtmos.MatchString(norm) {
			return plugin.AudioCodecEAC3Atmos
		}
		return plugin.AudioCodecEAC3
	case reAtmos.MatchString(norm):
		return plugin.AudioCodecEAC3Atmos
	case reAC3.MatchString(norm):
		return plugin.AudioCodecAC3
	case reAAC.MatchString(norm):
		return plugin.AudioCodecAAC
	case reFLAC.MatchString(norm):
		return plugin.AudioCodecFLAC
	case rePCM.MatchString(norm):
		return plugin.AudioCodecPCM
	case reMP3.MatchString(norm):
		return plugin.AudioCodecMP3
	case reOpus.MatchString(norm):
		return plugin.AudioCodecOpus
	default:
		return plugin.AudioCodecUnknown
	}
}

func parseAudioChannels(norm string) plugin.AudioChannels {
	switch {
	case reCh71.MatchString(norm):
		return plugin.AudioChannels71
	case reCh51.MatchString(norm):
		return plugin.AudioChannels51
	case reCh20.MatchString(norm):
		return plugin.AudioChannels20
	case reCh10.MatchString(norm):
		return plugin.AudioChannels10
	default:
		return plugin.AudioChannelsUnknown
	}
}
