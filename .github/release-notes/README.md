# Release highlights

Release notes are written by `tools/releasenotes` when a `v*` tag is pushed:
one line per merged pull request, from its title, grouped into breaking
changes, new features, fixes and speed-ups, with CI, build, test and chore
changes folded under "Under the hood".

For a release worth a few words of its own, add `<tag>.md` here (for example
`v0.14.0.md`) before tagging. It goes at the top of the release notes, above
the generated list. Two or three sentences on what changed for people using
the app is plenty.

Good PR titles make good notes: `feat(search): stream results as indexers
answer` becomes "**Search:** Stream results as indexers answer".
