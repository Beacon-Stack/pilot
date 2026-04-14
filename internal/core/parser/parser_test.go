package parser

import "testing"

func TestNormalizeTitle(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Breaking Bad", "breakingbad"},
		{"Breaking.Bad", "breakingbad"},
		{"breaking_bad", "breakingbad"},
		{"Breaking-Bad!", "breakingbad"},
		{"Breaking Bad (2008)", "breakingbad2008"},
		{"", ""},
		{"   ", ""},
		{"S.H.I.E.L.D.", "shield"},
	}
	for _, tc := range cases {
		if got := NormalizeTitle(tc.in); got != tc.want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTitleMatches_ExactAndPunctuation(t *testing.T) {
	accept := []struct{ series, release string }{
		{"Breaking Bad", "Breaking Bad"},
		{"Breaking Bad", "Breaking.Bad"},
		{"Breaking Bad", "breaking bad"},
		{"Breaking Bad", "Breaking-Bad"},
		{"Raised by Wolves", "Raised.by.Wolves"},
		{"Mr. Robot", "Mr Robot"},
	}
	for _, tc := range accept {
		if !TitleMatches(tc.series, tc.release) {
			t.Errorf("TitleMatches(%q, %q) = false, want true", tc.series, tc.release)
		}
	}
}

func TestTitleMatches_TrailingYearAllowed(t *testing.T) {
	// Indexers routinely embed the premiere year in release names.
	if !TitleMatches("Breaking Bad", "Breaking Bad 2008") {
		t.Error("trailing year should match")
	}
	if !TitleMatches("Breaking Bad", "Breaking.Bad.2008") {
		t.Error("trailing year after dot separator should match")
	}
	if !TitleMatches("The Office", "The Office 1999") {
		t.Error("trailing year with 'The' prefix should match")
	}
}

func TestTitleMatches_RejectsUnrelated(t *testing.T) {
	// These are the regression cases. Each is a release that would have
	// slipped past the old filter because it shares a season number and
	// parses as a season pack, but is the wrong show.
	reject := []struct{ series, release string }{
		{"Breaking Bad", "Breaking Bad Bulgaria"},
		{"Breaking Bad", "The Breaking Bad"},
		{"The Office", "The Office US"}, // strict per user directive
		{"The Office", "The Office UK"},
		{"Bad", "Breaking Bad"}, // "Bad" must not match "Breaking Bad"
		{"Breaking Bad", "Breaking"},
		{"Breaking Bad", "Totally Unrelated"},
		{"Breaking Bad", ""},
		{"", "Breaking Bad"},
	}
	for _, tc := range reject {
		if TitleMatches(tc.series, tc.release) {
			t.Errorf("TitleMatches(%q, %q) = true, want false", tc.series, tc.release)
		}
	}
}

func TestTitleMatches_YearNot4Digits(t *testing.T) {
	// Anything past a 4-digit-year suffix must be rejected.
	if TitleMatches("Breaking Bad", "Breaking Bad 20081") {
		t.Error("5-digit suffix should not count as a year")
	}
	if TitleMatches("Breaking Bad", "Breaking Bad 99") {
		t.Error("2-digit suffix should not count as a year")
	}
}
