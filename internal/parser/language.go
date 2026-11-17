package parser

import (
	"regexp"
	"strings"
)

var langTokens = map[string]Language{
	"english":    LangEnglish,
	"eng":        LangEnglish,
	"french":     LangFrench,
	"fre":        LangFrench,
	"fra":        LangFrench,
	"vff":        LangFrench,
	"vfq":        LangFrench,
	"truefrench": LangFrench,
	"german":     LangGerman,
	"ger":        LangGerman,
	"deu":        LangGerman,
	"spanish":    LangSpanish,
	"spa":        LangSpanish,
	"esp":        LangSpanish,
	"italian":    LangItalian,
	"ita":        LangItalian,
	"portuguese": LangPortuguese,
	"por":        LangPortuguese,
	"ptbr":       LangPortuguese,
	"russian":    LangRussian,
	"rus":        LangRussian,
	"japanese":   LangJapanese,
	"jpn":        LangJapanese,
	"korean":     LangKorean,
	"kor":        LangKorean,
	"chinese":    LangChinese,
	"chi":        LangChinese,
	"mandarin":   LangChinese,
	"cantonese":  LangChinese,
	"hindi":      LangHindi,
	"hin":        LangHindi,
	"arabic":     LangArabic,
	"ara":        LangArabic,
	"dutch":      LangDutch,
	"nld":        LangDutch,
	"nordic":     LangNordic,
	"swedish":    LangSwedish,
	"swe":        LangSwedish,
	"danish":     LangDanish,
	"dan":        LangDanish,
	"finnish":    LangFinnish,
	"fin":        LangFinnish,
	"norwegian":  LangNorwegian,
	"nor":        LangNorwegian,
	"polish":     LangPolish,
	"pol":        LangPolish,
	"turkish":    LangTurkish,
	"tur":        LangTurkish,
	"multi":      LangMulti,
}

// reLangToken matches language tags as standalone words in the normalized string.
// Built once at init from the langTokens map.
var reLangToken *regexp.Regexp

func init() {
	tokens := make([]string, 0, len(langTokens))
	seen := map[string]bool{}
	for k := range langTokens {
		if !seen[k] {
			tokens = append(tokens, regexp.QuoteMeta(k))
			seen[k] = true
		}
	}
	reLangToken = regexp.MustCompile(`(?i)\b(?:` + strings.Join(tokens, "|") + `)\b`)
}

// reLangTailStart finds where the show name ends: the first episode,
// season, year, absolute-episode or resolution marker. Language tags are
// only read after it, so a show named "Dan Da Dan" or "The French
// Dispatch" isn't tagged Danish or French.
var reLangTailStart = regexp.MustCompile(
	`(?i)\bS\d{1,2}(?:E\d{1,3})?\b` +
		`|\b\d{1,2}x\d{2,3}\b` +
		`|\bSeason \d` +
		`|\b(?:19|20)\d{2}\b` +
		`|\s-\s*\d{2,4}\b` +
		`|\b\d{3,4}p\b`)

// languageTail returns the part of norm after the show name. With no
// marker the whole string is returned.
func languageTail(norm string) string {
	if loc := reLangTailStart.FindStringIndex(norm); loc != nil {
		return norm[loc[0]:]
	}
	return norm
}

func parseLanguages(norm string) []Language {
	matches := reLangToken.FindAllString(languageTail(norm), -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[Language]bool{}
	var langs []Language
	for _, m := range matches {
		if lang, ok := langTokens[strings.ToLower(m)]; ok && !seen[lang] {
			langs = append(langs, lang)
			seen[lang] = true
		}
	}
	return langs
}
