package show

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	db "github.com/beacon-stack/pilot/internal/db/generated"
)

// Rename passed an empty quality, so "{Quality Full}" rendered "" and every
// rename stripped the quality the importer had just put in the name.
func TestRenameQuality_UsesStoredName(t *testing.T) {
	f := db.EpisodeFile{
		Path:        "/tv/Slow Horses (2022)/Season 01/Slow Horses - S01E01 - Failure's Contagious WEBRip-1080p x265.mp4",
		QualityJson: `{"resolution":"1080p","source":"webrip","codec":"x265","hdr":"none","name":"WEBRip-1080p x265","revision":1}`,
	}
	if got := renameQuality(f).Name; got != "WEBRip-1080p x265" {
		t.Fatalf("Name = %q, want %q", got, "WEBRip-1080p x265")
	}
}

// Library-scan rows have no name or source (ffprobe can't see the source):
// the filename supplies them, and its resolution beats the probed one
// (a cropped 1080p encode probes as 720p).
func TestRenameQuality_ScanRowParsesFilename(t *testing.T) {
	f := db.EpisodeFile{
		Path:        "/tv/Pluribus (2025)/Pluribus (2025) - S01E02 - Pirate Lady (1080p ATVP WEB-DL x265 Ghost).mkv",
		QualityJson: `{"resolution":"720p","source":"","codec":"x265","hdr":"none","name":""}`,
	}
	if got := renameQuality(f).Name; got != "WEBDL-1080p x265" {
		t.Fatalf("Name = %q, want %q", got, "WEBDL-1080p x265")
	}
}

// A clean name with no quality in it falls back to what was probed.
func TestRenameQuality_CleanNameKeepsProbedFields(t *testing.T) {
	f := db.EpisodeFile{
		Path:        "/tv/Silo (2023)/Season 03/Silo - S03E09 - Farewell.mkv",
		QualityJson: `{"resolution":"1080p","source":"","codec":"x265","hdr":"none","name":""}`,
	}
	if got := renameQuality(f).Name; got != "1080p x265" {
		t.Fatalf("Name = %q, want %q", got, "1080p x265")
	}
}

func TestRenameQuality_NothingKnown(t *testing.T) {
	f := db.EpisodeFile{Path: "/tv/Show/Season 01/Show - S01E01.mkv", QualityJson: `{}`}
	if got := renameQuality(f).Name; got != "" {
		t.Fatalf("Name = %q, want empty", got)
	}
}

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSubtitleMoves_SameDirectory(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "Old", "Season 01", "Show.S01E01.1080p.WEB.x265.mkv")
	touch(t, old)
	touch(t, filepath.Join(dir, "Old", "Season 01", "Show.S01E01.1080p.WEB.x265.srt"))
	touch(t, filepath.Join(dir, "Old", "Season 01", "Show.S01E01.1080p.WEB.x265.en.forced.srt"))
	touch(t, filepath.Join(dir, "Old", "Season 01", "Show.S01E02.1080p.WEB.x265.srt")) // another episode
	touch(t, filepath.Join(dir, "Old", "Season 01", "Show.S01E01.1080p.WEB.x265.nfo")) // not a subtitle

	newPath := filepath.Join(dir, "Show (2020)", "Season 01", "Show - S01E01 - Pilot WEBDL-1080p x265.mkv")
	got := subtitleMoves(old, newPath)
	nd := filepath.Dir(newPath)
	od := filepath.Dir(old)
	want := []RenameMove{
		{filepath.Join(od, "Show.S01E01.1080p.WEB.x265.en.forced.srt"), filepath.Join(nd, "Show - S01E01 - Pilot WEBDL-1080p x265.en.forced.srt")},
		{filepath.Join(od, "Show.S01E01.1080p.WEB.x265.srt"), filepath.Join(nd, "Show - S01E01 - Pilot WEBDL-1080p x265.srt")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

// The layout on the NAS for Raised by Wolves S01 (RARBG).
func TestSubtitleMoves_RarbgSubsFolder(t *testing.T) {
	dir := t.TempDir()
	base := "Raised.By.Wolves.2020.S01E01.1080p.BluRay.x265-RARBG[eztv.re]"
	old := filepath.Join(dir, "Season 01", base+".mp4")
	touch(t, old)
	touch(t, filepath.Join(dir, "Season 01", "Subs", base, "2_English.srt"))
	touch(t, filepath.Join(dir, "Season 01", "Subs", base, "3_English.srt"))
	touch(t, filepath.Join(dir, "Season 01", "Subs", "Raised.By.Wolves.2020.S01E02.1080p.BluRay.x265-RARBG[eztv.re]", "2_English.srt"))

	newPath := filepath.Join(dir, "Season 01", "Raised by Wolves - S01E01 - Raised by Wolves Bluray-1080p x265.mp4")
	got := subtitleMoves(old, newPath)
	subs := filepath.Join(dir, "Season 01", "Subs", base)
	nb := filepath.Join(dir, "Season 01", "Raised by Wolves - S01E01 - Raised by Wolves Bluray-1080p x265")
	want := []RenameMove{
		{filepath.Join(subs, "2_English.srt"), nb + ".English.srt"},
		{filepath.Join(subs, "3_English.srt"), nb + ".English.2.srt"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestSubtitleMoves_NoSubtitles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "a.mkv")
	touch(t, old)
	if got := subtitleMoves(old, filepath.Join(dir, "b.mkv")); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}
