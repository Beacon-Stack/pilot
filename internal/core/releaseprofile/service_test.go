package releaseprofile

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beacon-stack/pilot/internal/config"
	dbpkg "github.com/beacon-stack/pilot/internal/db"
	db "github.com/beacon-stack/pilot/internal/db/generated"
)

func TestMatcher(t *testing.T) {
	m := NewMatcher([]Profile{
		{Name: "No junk", Enabled: true, Ignored: []string{"CAM", "/\\bHC\\b/"}},
		{Name: "Only these groups", Enabled: true, Required: []string{"-NTb", "-FLUX"}},
		{Name: "One indexer only", Enabled: true, IndexerID: "idx-2", Ignored: []string{"x265"}},
		{Name: "Disabled", Enabled: false, Ignored: []string{"1080p"}},
	})
	cases := []struct {
		title, indexer string
		want           bool
		reasonHas      string
	}{
		{"Show.S01E01.1080p.WEB-DL.x264-NTb", "idx-1", true, ""},
		{"Show.S01E01.1080p.web-dl.x264-ntb", "idx-1", true, ""}, // case-insensitive
		{"Show.S01E01.1080p.WEB-DL.x264-OTHER", "idx-1", false, "none of the required terms"},
		{"Show.S01E01.CAM.x264-NTb", "idx-1", false, `"CAM"`},
		{"Show.S01E01.1080p.HC.WEB-DL-NTb", "idx-1", false, `/\bHC\b/`}, // regex
		{"Show.S01E01.1080p.HCTV.WEB-DL-NTb", "idx-1", true, ""},        // regex word boundary
		{"Show.S01E01.1080p.x265-FLUX", "idx-1", true, ""},              // indexer-scoped rule doesn't apply
		{"Show.S01E01.1080p.x265-FLUX", "idx-2", false, "One indexer only"},
	}
	for _, c := range cases {
		ok, reason := m.Check(c.title, c.indexer)
		if ok != c.want || !strings.Contains(reason, c.reasonHas) {
			t.Errorf("Check(%q, %q) = (%v, %q), want (%v, ...%s...)", c.title, c.indexer, ok, reason, c.want, c.reasonHas)
		}
	}
	var nilMatcher *Matcher
	if ok, _ := nilMatcher.Check("anything", ""); !ok {
		t.Fatal("a nil matcher must accept everything")
	}
}

func newService(t *testing.T) (*Service, *db.Queries, func(string, ...any)) {
	t.Helper()
	d, err := dbpkg.Open(config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "pilot.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := dbpkg.Migrate(d.SQL); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.SQL.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	q := db.New(d.SQL)
	return NewService(q), q, exec
}

func TestService_CRUDAndMatcher(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()

	p, err := svc.Create(ctx, Request{Name: "  No CAM  ", Enabled: true, Ignored: []string{" cam ", "CAM", ""}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "No CAM" || len(p.Ignored) != 1 || p.Ignored[0] != "cam" || p.Required == nil {
		t.Fatalf("not normalized: %+v", p)
	}

	m, err := LoadMatcher(ctx, svc.q)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := m.Check("Show.S01E01.CAM-GRP", ""); ok {
		t.Fatal("stored profile not applied by the matcher")
	}

	p, err = svc.Update(ctx, p.ID, Request{Name: "No CAM", Enabled: false, Ignored: []string{"cam"}})
	if err != nil || p.Enabled {
		t.Fatalf("update: %+v %v", p, err)
	}
	if m, _ = LoadMatcher(ctx, svc.q); func() bool { ok, _ := m.Check("Show.S01E01.CAM-GRP", ""); return !ok }() {
		t.Fatal("disabled profile still applied")
	}

	if err := svc.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if _, err := svc.Update(ctx, "nope", Request{Name: "x", Ignored: []string{"a"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	if err := svc.Delete(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestService_Validation(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	for name, req := range map[string]Request{
		"no name":   {Ignored: []string{"cam"}},
		"no terms":  {Name: "Empty", Required: []string{" "}},
		"bad regex": {Name: "Bad", Ignored: []string{"/([/"}},
	} {
		var verr *ValidationError
		if _, err := svc.Create(ctx, req); !errors.As(err, &verr) {
			t.Errorf("%s: err = %v, want a ValidationError", name, err)
		}
	}
}

// Deleting an indexer deletes the profiles scoped to it, so they don't
// linger as rules for an indexer that no longer exists.
func TestService_IndexerDeleteCascades(t *testing.T) {
	svc, _, exec := newService(t)
	ctx := context.Background()
	exec(`INSERT INTO indexer_configs (id, name, kind, created_at, updated_at) VALUES ('idx', 'X', 'torznab', 'now', 'now')`)
	p, err := svc.Create(ctx, Request{Name: "Scoped", Enabled: true, Ignored: []string{"cam"}, IndexerID: "idx"})
	if err != nil {
		t.Fatal(err)
	}
	exec(`DELETE FROM indexer_configs WHERE id = 'idx'`)
	if _, err := svc.Get(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("scoped profile survived its indexer: %v", err)
	}
}
