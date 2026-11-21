package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Usage (run by release.yml before GoReleaser):
//
//	go run ./tools/releasenotes -app Pilot -tag v0.14.0 > notes.md
//
// The previous tag is found with git describe; pass -prev to override.
// .github/release-notes/<tag>.md, if it exists, is put first.
func main() {
	app := flag.String("app", "", "display name, e.g. Pilot")
	tag := flag.String("tag", "", "the tag being released, e.g. v0.14.0")
	prev := flag.String("prev", "", "previous tag (default: the nearest older tag)")
	flag.Parse()
	if *app == "" || *tag == "" {
		fmt.Fprintln(os.Stderr, "usage: releasenotes -app <Name> -tag <vX.Y.Z> [-prev <tag>]")
		os.Exit(2)
	}
	lower := strings.ToLower(*app)

	from := *prev
	if from == "" {
		// The nearest tag reachable from the commit before this one.
		if out, err := git("describe", "--tags", "--abbrev=0", "--match", "v*", *tag+"^"); err == nil {
			from = strings.TrimSpace(out)
		}
	}
	rangeSpec := *tag
	if from != "" {
		rangeSpec = from + ".." + *tag
	}
	// %x1f between fields, %x1e between commits.
	out, err := git("log", "--first-parent", "--format=%H%x1f%s%x1f%b%x1e", rangeSpec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "git log:", err)
		os.Exit(1)
	}
	var commits []Commit
	for _, rec := range strings.Split(out, "\x1e") {
		f := strings.Split(strings.TrimLeft(rec, "\n"), "\x1f")
		if len(f) < 3 || f[0] == "" {
			continue
		}
		commits = append(commits, Commit{SHA: f[0], Subject: f[1], Body: f[2]})
	}

	var highlights string
	if b, err := os.ReadFile(".github/release-notes/" + *tag + ".md"); err == nil {
		highlights = string(b)
	}

	fmt.Print(Render(Notes{
		App:        *app,
		Repo:       "beacon-stack/" + lower,
		Image:      "ghcr.io/beacon-stack/" + lower,
		Version:    strings.TrimPrefix(*tag, "v"),
		Tag:        *tag,
		PrevTag:    from,
		Highlights: highlights,
		Entries:    Parse(commits),
	}))
}

func git(args ...string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", args...).Output()
	return string(out), err
}
