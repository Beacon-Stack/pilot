package show

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	db "github.com/beacon-stack/pilot/internal/db/generated"
	"github.com/beacon-stack/pilot/internal/parser"
	"github.com/beacon-stack/pilot/pkg/plugin"
)

// renameQuality is the quality a rename renders into {Quality Full}.
//
// The stored quality is used when it has a name (importer rows). Rows
// from the library scan have no name and no source (ffprobe cannot see
// the source), so the current filename is parsed and its fields win over
// the probed ones; the probed ones fill whatever the filename lacks.
func renameQuality(f db.EpisodeFile) plugin.Quality {
	var q plugin.Quality
	_ = json.Unmarshal([]byte(f.QualityJson), &q)
	if q.Name != "" {
		return q
	}
	p := parser.Parse(filepath.Base(f.Path))
	if known(string(p.Source)) {
		q.Source = p.Source
	}
	if known(string(p.Resolution)) {
		q.Resolution = p.Resolution
	}
	if known(string(p.Codec)) {
		q.Codec = p.Codec
	}
	if known(string(p.HDR)) {
		q.HDR = p.HDR
	}
	q.Name = parser.QualityName(q)
	return q
}

func known(v string) bool { return v != "" && v != "unknown" && v != "none" }

var subtitleExts = map[string]bool{
	".srt": true, ".ass": true, ".ssa": true, ".sub": true,
	".idx": true, ".vtt": true, ".sup": true,
}

// rarbgSubRe matches the numbered names in a RARBG "Subs/<release>/" folder
// ("2_English.srt").
var rarbgSubRe = regexp.MustCompile(`^\d+_`)

// subtitleMoves lists the subtitle files that belong to the episode file at
// oldPath and where they go when it is renamed to newPath. Two layouts:
//
//   - next to the video, named after it: "<base>.srt", "<base>.en.srt"
//     → "<newbase>.srt", "<newbase>.en.srt"
//   - a RARBG-style "Subs/<base>/2_English.srt" folder
//     → "<newbase>.English.srt" ("<newbase>.English.2.srt" for the next
//     one with the same label), next to the renamed video
//
// It only reads the disk. Targets that collide with each other get a
// numeric suffix; targets that already exist are left to the caller.
func subtitleMoves(oldPath, newPath string) []RenameMove {
	oldDir, newDir := filepath.Dir(oldPath), filepath.Dir(newPath)
	oldBase := strings.TrimSuffix(filepath.Base(oldPath), filepath.Ext(oldPath))
	newBase := strings.TrimSuffix(filepath.Base(newPath), filepath.Ext(newPath))

	var moves []RenameMove
	taken := map[string]bool{}
	add := func(src, label, ext string) {
		name := newBase + label + ext
		for n := 2; taken[name]; n++ {
			name = fmt.Sprintf("%s%s.%d%s", newBase, label, n, ext)
		}
		taken[name] = true
		dst := filepath.Join(newDir, name)
		if dst != src {
			moves = append(moves, RenameMove{OldPath: src, NewPath: dst})
		}
	}

	if entries, err := os.ReadDir(oldDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if e.IsDir() || !subtitleExts[ext] {
				continue
			}
			stem := strings.TrimSuffix(name, filepath.Ext(name))
			if stem != oldBase && !strings.HasPrefix(stem, oldBase+".") {
				continue
			}
			add(filepath.Join(oldDir, name), strings.TrimPrefix(stem, oldBase), filepath.Ext(name))
		}
	}

	subsDir := filepath.Join(oldDir, "Subs", oldBase)
	if entries, err := os.ReadDir(subsDir); err == nil {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() && subtitleExts[strings.ToLower(filepath.Ext(e.Name()))] {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			label := rarbgSubRe.ReplaceAllString(strings.TrimSuffix(name, filepath.Ext(name)), "")
			if label != "" {
				label = "." + label
			}
			add(filepath.Join(subsDir, name), label, filepath.Ext(name))
		}
	}
	return moves
}
