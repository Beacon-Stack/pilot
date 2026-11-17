package language

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/beacon-stack/pilot/internal/config"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/parser"
)

func TestAcceptsTitle(t *testing.T) {
	english := Preference{Languages: []string{"english"}}
	german := Preference{Languages: []string{"german"}}
	swedish := Preference{Languages: []string{"swedish"}}
	cases := []struct {
		name  string
		pref  Preference
		title string
		want  bool
	}{
		{"any language", Default(), "Show.S01E01.FRENCH.1080p.WEB-DL-GRP", true},
		{"untagged counts as english", english, "Show.S01E01.1080p.WEB-DL-GRP", true},
		{"tagged other language", english, "Show.S01E01.FRENCH.1080p.WEB-DL-GRP", false},
		{"truefrench", english, "Show.S01E01.TRUEFRENCH.1080p.WEB-DL-GRP", false},
		{"wanted language tagged", german, "Show.S01E01.GERMAN.1080p.WEB-DL-GRP", true},
		{"untagged is not german", german, "Show.S01E01.1080p.WEB-DL-GRP", false},
		{"multi matches anything", german, "Show.S01E01.MULTi.1080p.WEB-DL-GRP", true},
		{"one of several tags", english, "Show.S01E01.German.ENG.1080p.WEB-DL-GRP", true},
		{"nordic covers swedish", swedish, "Show.S01E01.NORDiC.1080p.WEB-DL-GRP", true},
		// The show name is not read for language tags.
		{"show name looks danish", english, "Dan.Da.Dan.S01E01.1080p.WEB-DL-GRP", true},
		{"show name looks french", english, "The.French.Dispatch.2021.1080p.BluRay-GRP", true},
		{"season pack", english, "Dan.Da.Dan.S01.1080p.WEB-DL-GRP", true},
		{"anime absolute", english, "[SubsPlease] Dan Da Dan - 05 (1080p)", true},
	}
	for _, c := range cases {
		if got := c.pref.AcceptsTitle(c.title); got != c.want {
			t.Errorf("%s: AcceptsTitle(%q) = %v, want %v (parsed %v)",
				c.name, c.title, got, c.want, parser.Parse(c.title).Languages)
		}
	}
}

func TestValidate(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default invalid: %v", err)
	}
	if err := (Preference{Languages: []string{"english", "nordic"}}).Validate(); err != nil {
		t.Fatalf("valid preference rejected: %v", err)
	}
	for name, p := range map[string]Preference{
		"unknown":   {Languages: []string{"klingon"}},
		"multi":     {Languages: []string{"multi"}},
		"duplicate": {Languages: []string{"english", "english"}},
	} {
		var verr *ValidationError
		if err := p.Validate(); !errors.As(err, &verr) {
			t.Errorf("%s: err = %v, want ValidationError", name, err)
		}
	}
}

func TestStore_DefaultAndRoundTrip(t *testing.T) {
	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pilot.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	s := NewStore(db.New(d.SQL))
	ctx := context.Background()

	got, err := s.Get(ctx)
	if err != nil || !reflect.DeepEqual(got, Default()) {
		t.Fatalf("unsaved preference = %+v, %v; want the default", got, err)
	}
	want := Preference{Languages: []string{"english", "german"}}
	if err := s.Set(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.Get(ctx); !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	if err := s.Set(ctx, Preference{Languages: []string{"klingon"}}); err == nil {
		t.Fatal("an invalid preference was saved")
	}
}
