package provider

import (
	"context"
	"database/sql"
	"testing"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// fakeStore is an in-memory Store for tests — no Postgres required.
type fakeStore struct {
	rows map[string]string
}

func newFakeStore() *fakeStore { return &fakeStore{rows: make(map[string]string)} }

func (f *fakeStore) GetSetting(_ context.Context, key string) (string, error) {
	v, ok := f.rows[key]
	if !ok {
		return "", sql.ErrNoRows
	}
	return v, nil
}

func (f *fakeStore) SetSetting(_ context.Context, arg db.SetSettingParams) error {
	f.rows[arg.Key] = arg.Value
	return nil
}

func (f *fakeStore) DeleteSetting(_ context.Context, key string) error {
	delete(f.rows, key)
	return nil
}

func TestResolver_EffectiveKey_NoOverride_FallsBackToDefault(t *testing.T) {
	// With no stored override and no baked default, effective key is empty.
	r := NewResolver(newFakeStore())
	key, source, err := r.EffectiveKey(context.Background(), TMDB)
	if err != nil {
		t.Fatal(err)
	}
	if source != SourceDefault {
		t.Errorf("source = %q; want %q", source, SourceDefault)
	}
	if key != "" {
		t.Errorf("key = %q; want empty (no baked default in tests)", key)
	}
}

func TestResolver_EffectiveKey_Override_WinsOverDefault(t *testing.T) {
	store := newFakeStore()
	r := NewResolver(store)

	if err := r.SetOverride(context.Background(), TMDB, "my-override-key"); err != nil {
		t.Fatal(err)
	}

	key, source, err := r.EffectiveKey(context.Background(), TMDB)
	if err != nil {
		t.Fatal(err)
	}
	if source != SourceOverride {
		t.Errorf("source = %q; want %q", source, SourceOverride)
	}
	if key != "my-override-key" {
		t.Errorf("key = %q; want my-override-key", key)
	}
}

func TestResolver_ClearOverride_RevertsToDefault(t *testing.T) {
	r := NewResolver(newFakeStore())
	ctx := context.Background()
	if err := r.SetOverride(ctx, TMDB, "will-be-cleared"); err != nil {
		t.Fatal(err)
	}

	has, err := r.HasOverride(ctx, TMDB)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Fatal("expected override to exist before clear")
	}

	if err := r.ClearOverride(ctx, TMDB); err != nil {
		t.Fatal(err)
	}

	has, err = r.HasOverride(ctx, TMDB)
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("expected override to be absent after clear")
	}

	_, source, err := r.EffectiveKey(ctx, TMDB)
	if err != nil {
		t.Fatal(err)
	}
	if source != SourceDefault {
		t.Errorf("source = %q after clear; want %q", source, SourceDefault)
	}
}

func TestResolver_SetOverride_TrimsWhitespace(t *testing.T) {
	r := NewResolver(newFakeStore())
	ctx := context.Background()
	if err := r.SetOverride(ctx, TMDB, "  key-with-whitespace\n"); err != nil {
		t.Fatal(err)
	}

	key, _, err := r.EffectiveKey(ctx, TMDB)
	if err != nil {
		t.Fatal(err)
	}
	if key != "key-with-whitespace" {
		t.Errorf("key = %q; want key-with-whitespace (whitespace trimmed)", key)
	}
}

func TestResolver_SetOverride_RejectsEmpty(t *testing.T) {
	r := NewResolver(newFakeStore())
	if err := r.SetOverride(context.Background(), TMDB, "   "); err == nil {
		t.Error("expected error when override value is empty after trim")
	}
}

func TestResolver_UnknownProvider_Rejected(t *testing.T) {
	r := NewResolver(newFakeStore())
	ctx := context.Background()

	if _, _, err := r.EffectiveKey(ctx, "bogus"); err == nil {
		t.Error("expected error for unknown provider on EffectiveKey")
	}
	if err := r.SetOverride(ctx, "bogus", "x"); err == nil {
		t.Error("expected error for unknown provider on SetOverride")
	}
	if err := r.ClearOverride(ctx, "bogus"); err == nil {
		t.Error("expected error for unknown provider on ClearOverride")
	}
}

func TestResolver_Preview_RedactsOverride(t *testing.T) {
	r := NewResolver(newFakeStore())
	ctx := context.Background()
	if err := r.SetOverride(ctx, TMDB, "abcdef1234567890xyz"); err != nil {
		t.Fatal(err)
	}
	preview, source, err := r.Preview(ctx, TMDB)
	if err != nil {
		t.Fatal(err)
	}
	if source != SourceOverride {
		t.Errorf("source = %q; want %q", source, SourceOverride)
	}
	want := "••••••••••••••••xyz"
	if preview != want {
		t.Errorf("preview = %q; want %q", preview, want)
	}
}

func TestRedact_ShortKeysFullyBulleted(t *testing.T) {
	cases := map[string]string{
		"":     "",
		"a":    "•",
		"ab":   "••",
		"abc":  "•••",
		"abcd": "•abcd"[:1] + "bcd", // this specific test is awkward — rewrite:
	}
	_ = cases
	// Explicit cases:
	if got := redact(""); got != "" {
		t.Errorf("redact empty = %q", got)
	}
	if got := redact("ab"); got != "••" {
		t.Errorf("redact(ab) = %q; want ••", got)
	}
	if got := redact("abc"); got != "•••" {
		t.Errorf("redact(abc) = %q; want •••", got)
	}
	if got := redact("abcd"); got != "•bcd" {
		t.Errorf("redact(abcd) = %q; want •bcd", got)
	}
}
