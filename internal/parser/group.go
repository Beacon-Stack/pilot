package parser

import (
	"regexp"
	"strings"
)

var (
	reFileExt      = regexp.MustCompile(`(?i)\.(mkv|mp4|avi|m4v|ts|wmv|mov|flv|webm)$`)
	reBracketGroup = regexp.MustCompile(`[\[\(]([A-Za-z0-9][A-Za-z0-9._-]*[A-Za-z0-9])[\]\)]\s*$`)
)

// knownCompoundSuffixes are strings that appear after a hyphen in multi-part
// quality tokens (WEB-DL, DTS-HD, etc.) and must not be mistaken for groups.
var knownCompoundSuffixes = map[string]bool{
	"dl":   true, // WEB-DL
	"rip":  true, // WEB-Rip
	"hd":   true, // RAW-HD, DTS-HD
	"x":    true, // DTS-X
	"disk": true, // BR-DISK
	"r":    true, // DVD-R
	"ray":  true, // Blu-Ray
}

// parseReleaseGroup extracts the release group from the raw (un-normalized) title.
func parseReleaseGroup(title string) string {
	s := reFileExt.ReplaceAllString(title, "")

	if m := reBracketGroup.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}

	for {
		idx := strings.LastIndex(s, "-")
		if idx < 0 {
			return ""
		}
		candidate := strings.TrimRight(s[idx+1:], " .")
		s = s[:idx]

		if candidate == "" {
			continue
		}
		if knownCompoundSuffixes[strings.ToLower(candidate)] {
			continue
		}
		if !isAlphanumeric(candidate) {
			continue
		}
		return candidate
	}
}

func isAlphanumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
