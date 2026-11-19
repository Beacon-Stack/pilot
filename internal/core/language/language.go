// Package language implements the language preference: which audio
// languages a release may be in. Releases in no wanted language are tagged
// with FilterReason, so auto-search and RSS skip them and interactive
// search shows them grayed with an override.
//
// Languages come from the release title (internal/parser). As in Sonarr,
// a release with no language tag is assumed to be English, and a MULTI
// release matches any wanted language.
package language

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/parser"
)

// FilterReason tags a release in no wanted language.
const FilterReason = "unwanted_language"

// settingKey is where the preference is stored in the settings table.
const settingKey = "language_preference"

// Known lists the languages a preference can name, in display order.
// "multi" is not one: it is a property of a release, not a language.
var Known = []parser.Language{
	parser.LangEnglish, parser.LangFrench, parser.LangGerman, parser.LangSpanish,
	parser.LangItalian, parser.LangPortuguese, parser.LangDutch, parser.LangRussian,
	parser.LangPolish, parser.LangTurkish, parser.LangArabic, parser.LangHindi,
	parser.LangJapanese, parser.LangKorean, parser.LangChinese,
	parser.LangSwedish, parser.LangDanish, parser.LangNorwegian, parser.LangFinnish,
	parser.LangNordic,
}

// nordic languages all match a release tagged NORDIC, which carries
// several Nordic audio tracks.
var nordic = map[parser.Language]bool{
	parser.LangSwedish: true, parser.LangDanish: true, parser.LangNorwegian: true,
	parser.LangFinnish: true, parser.LangNordic: true,
}

// Preference is the saved language preference.
type Preference struct {
	// Languages are the wanted languages. Empty means any language.
	Languages []string `json:"languages"`
}

// Default is any language: the behavior before the preference existed.
func Default() Preference { return Preference{Languages: []string{}} }

// ValidationError explains why a preference can't be saved.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// Validate checks that every language is known and listed once.
func (p Preference) Validate() error {
	seen := map[string]bool{}
	for _, l := range p.Languages {
		if !isKnown(l) {
			return &ValidationError{fmt.Sprintf("unknown language %q", l)}
		}
		if seen[l] {
			return &ValidationError{fmt.Sprintf("language %q is listed twice", l)}
		}
		seen[l] = true
	}
	return nil
}

func isKnown(l string) bool {
	for _, k := range Known {
		if string(k) == l {
			return true
		}
	}
	return false
}

// Accepts reports whether a release with these parsed languages is
// wanted.
func (p Preference) Accepts(langs []parser.Language) bool {
	if len(p.Languages) == 0 {
		return true
	}
	if len(langs) == 0 {
		langs = []parser.Language{parser.LangEnglish}
	}
	for _, have := range langs {
		if have == parser.LangMulti {
			return true
		}
		for _, w := range p.Languages {
			want := parser.Language(w)
			if have == want || (have == parser.LangNordic && nordic[want]) {
				return true
			}
		}
	}
	return false
}

// AcceptsTitle parses the release title and reports whether it is wanted.
func (p Preference) AcceptsTitle(title string) bool {
	if len(p.Languages) == 0 {
		return true
	}
	return p.Accepts(parser.Parse(title).Languages)
}

// Store loads and saves the preference in the settings table.
type Store struct {
	q   db.Querier
	now func() time.Time
}

// NewStore creates a Store.
func NewStore(q db.Querier) *Store { return &Store{q: q, now: time.Now} }

// Get returns the saved preference, or Default() when none is saved.
func (s *Store) Get(ctx context.Context) (Preference, error) {
	raw, err := s.q.GetSetting(ctx, settingKey)
	if errors.Is(err, sql.ErrNoRows) {
		return Default(), nil
	}
	if err != nil {
		return Preference{}, err
	}
	p := Default()
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return Preference{}, fmt.Errorf("decoding language preference: %w", err)
	}
	if p.Languages == nil {
		p.Languages = []string{}
	}
	return p, nil
}

// Set validates and saves p.
func (s *Store) Set(ctx context.Context, p Preference) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Languages == nil {
		p.Languages = []string{}
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.q.SetSetting(ctx, db.SetSettingParams{
		Key:       settingKey,
		Value:     string(b),
		UpdatedAt: s.now().UTC().Format(time.RFC3339),
	})
}
