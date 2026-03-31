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

func parseLanguages(norm string) []Language {
	matches := reLangToken.FindAllString(norm, -1)
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
