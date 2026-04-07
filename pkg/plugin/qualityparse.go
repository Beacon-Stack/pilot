package plugin

import (
	"regexp"
	"strings"
)

var (
	resolutionRe = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p|4K)\b`)
	sourceRe     = regexp.MustCompile(`(?i)\b(BluRay|Blu-Ray|BDRip|BRRip|WEB-DL|WEBDL|WEBRip|WEB|HDTV|DVDRip|DVDScr|PDTV|SDTV|CAM|TS|TC|SCR|R5|AMZN|NF|DSNP|HMAX|ATVP)\b`)
	codecRe      = regexp.MustCompile(`(?i)\b(x264|x265|H\.?264|H\.?265|HEVC|AVC|XviD|DivX|VP9|AV1|MPEG2)\b`)
	hdrRe        = regexp.MustCompile(`(?i)\b(HDR10\+|HDR10|HDR|DV|Dolby\.?Vision|HLG)\b`)
)

// ParseQualityFromTitle extracts quality metadata from a release title string.
func ParseQualityFromTitle(title string) Quality {
	var q Quality

	if m := resolutionRe.FindString(title); m != "" {
		r := strings.ToLower(m)
		if r == "4k" {
			r = "2160p"
		}
		q.Resolution = Resolution(r)
	}

	if m := sourceRe.FindString(title); m != "" {
		q.Source = Source(normalizeSource(m))
	}

	if m := codecRe.FindString(title); m != "" {
		q.Codec = Codec(normalizeCodec(m))
	}

	if m := hdrRe.FindString(title); m != "" {
		q.HDR = HDRFormat(strings.ToUpper(m))
	}

	q.Name = buildQualityName(q)
	return q
}

func normalizeSource(s string) string {
	upper := strings.ToUpper(strings.ReplaceAll(s, "-", ""))
	switch upper {
	case "BLURAY", "BDMUX", "BDRIP", "BRRIP":
		return "Bluray"
	case "WEBDL", "WEB":
		return "WEBDL"
	case "WEBRIP":
		return "WEBRip"
	case "HDTV":
		return "HDTV"
	case "DVDRIP", "DVDSCR":
		return "DVD"
	default:
		return s
	}
}

func normalizeCodec(s string) string {
	upper := strings.ToUpper(strings.ReplaceAll(s, ".", ""))
	switch upper {
	case "X264", "H264", "AVC":
		return "x264"
	case "X265", "H265", "HEVC":
		return "x265"
	default:
		return s
	}
}

func buildQualityName(q Quality) string {
	parts := []string{}
	if q.Source != "" {
		parts = append(parts, string(q.Source))
	}
	if q.Resolution != "" {
		parts = append(parts, string(q.Resolution))
	}
	if q.Codec != "" {
		parts = append(parts, string(q.Codec))
	}
	if q.HDR != "" {
		parts = append(parts, string(q.HDR))
	}
	if len(parts) == 0 {
		return "Unknown"
	}
	return strings.Join(parts, " ")
}
