package parser

import "regexp"

type editionRule struct {
	name string
	re   *regexp.Regexp
}

// Edition rules — order matters: specific multi-word patterns before broad single-word.
var editionRules = []editionRule{
	{name: "Director's Cut", re: regexp.MustCompile(`(?i)\bdirector'?s[\s._-]?(?:cut|edition)\b`)},
	{name: "Extended", re: regexp.MustCompile(`(?i)\bextended[\s._-]?(?:cut|edition|version)\b`)},
	{name: "Theatrical", re: regexp.MustCompile(`(?i)\btheatrical[\s._-]?(?:cut|edition|release)\b`)},
	{name: "Unrated", re: regexp.MustCompile(`(?i)\b(?:unrated[\s._-]?(?:cut|edition)?|uncensored)\b`)},
	{name: "Ultimate", re: regexp.MustCompile(`(?i)\bultimate[\s._-]?(?:cut|edition|collector'?s?)\b`)},
	{name: "Special Edition", re: regexp.MustCompile(`(?i)\bspecial[\s._-]?edition\b`)},
	{name: "Criterion", re: regexp.MustCompile(`(?i)\bcriterion[\s._-]?(?:collection)?\b`)},
	{name: "IMAX", re: regexp.MustCompile(`(?i)\bimax[\s._-]?(?:edition)?\b`)},
	{name: "Final Cut", re: regexp.MustCompile(`(?i)\bfinal[\s._-]?cut\b`)},
	{name: "Open Matte", re: regexp.MustCompile(`(?i)\bopen[\s._-]?matte\b`)},
	{name: "Rogue Cut", re: regexp.MustCompile(`(?i)\brogue[\s._-]?cut\b`)},
	{name: "Black and Chrome", re: regexp.MustCompile(`(?i)\bblack[\s._-]?and[\s._-]?chrome\b`)},
	{name: "Remastered", re: regexp.MustCompile(`(?i)\b(?:4k[\s._-]?)?(?:digitally[\s._-]?)?remastered\b`)},
	{name: "Anniversary", re: regexp.MustCompile(`(?i)\b(?:\d+(?:st|nd|rd|th)[\s._-]?)?anniversary[\s._-]?(?:edition)?\b`)},
	// Single-word fallbacks (check last).
	{name: "Extended", re: regexp.MustCompile(`(?i)\bextended\b`)},
	{name: "Theatrical", re: regexp.MustCompile(`(?i)\btheatrical\b`)},
	{name: "Redux", re: regexp.MustCompile(`(?i)\bredux\b`)},
}

func parseEdition(norm string) string {
	for _, rule := range editionRules {
		if rule.re.MatchString(norm) {
			return rule.name
		}
	}
	return ""
}
