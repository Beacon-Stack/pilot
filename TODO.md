# Pilot — TODO

## Quality System

- [x] **Quality groups in the profile gate** — done in plans/sonarr-parity.md B1. A profile's allowed list is now the rule, matched on resolution + source (`quality.Profile.Allows`, used by `decision.MeetsProfileMinimum`), so "Bluray only" rejects WEB-DL and "accept WEBRip as a fallback" means listing WEBRip. The editor is in Settings > Quality Profiles (B1b).

## Release Search & Ranking

- [ ] **Season-pack custom format bonus** — The indexer sort already has an Episode Count tier that lifts season packs above individual episodes within the same quality tier (`effectiveEpisodeCount` in `internal/core/indexer/service.go`). Sonarr + TRaSH Guides also recommend a "Season Pack" custom format that adds a flat score to any parsed season pack, which lets a pack edge ahead of an individual episode even across slightly different quality tiers. Pilot has no custom-format system yet, so the minimum-viable version is a hardcoded additive bonus: when `PackType == PackTypeSeason`, bump `QualityScore` by a small constant (say +10) before sorting. Tiny change to the comparator, no data model. Consider this after shipping full custom formats — then it becomes one of the built-in formats and the hardcoded bonus can be deleted.
