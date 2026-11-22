package v1

import (
	"regexp"
	"strings"
)

// junkTitleRE matches what vandalised TMDB titles contain: phone numbers
// ("+1-888-805-5591") and web addresses. Seen on 2026-11-22: a real
// Netflix series listed as "Ticket+1-888-805-5591 or US +1-888-...".
var junkTitleRE = regexp.MustCompile(`\d[\d\s().-]{8,}\d|(?i)https?://|www\.|\.(com|net|org)\b`)

// displayTitle returns a title fit to show: the localized title, or the
// original-language one when the localized title has been vandalised.
// ok is false when both look like junk, so the entry should be skipped.
func displayTitle(title, original string) (string, bool) {
	if t := strings.TrimSpace(title); t != "" && !junkTitleRE.MatchString(t) {
		return t, true
	}
	if o := strings.TrimSpace(original); o != "" && !junkTitleRE.MatchString(o) {
		return o, true
	}
	return "", false
}
