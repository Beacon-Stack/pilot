# Phase D design

**Status: IN PROGRESS** (2026-11-17). Follows `plans/sonarr-parity.md`
Phases A–C. Each section is one or two PRs (backend, then UI), merged
before the next starts, under the same rules as the parity plan.

## Order

| # | Section | Size | Why this position |
|---|---------|------|-------------------|
| D1 | Series bulk editor | S | Small and self-contained; needed before libraries get large. |
| D2 | Manual import + reviewable library import | M | Biggest day-to-day gap: there is no way to import files Pilot didn't grab, and the library scan adds series from folder names with no review. |
| D3 | Custom formats | L | Unblocks the hidden Custom Formats page (A1) and the CF score fields quality profiles already carry. Touches the Pulse contract, so it goes after the safer sections. |
| D4 | More integrations | M | Independent plugins; can be split and done in any order. |
| D5 | Scene numbering (TheXEM) | L | Highest risk: an external service and a second numbering scheme in every search, RSS and import path. Last, behind a per-series switch. |

## D1. Series bulk editor

**Backend:** `PUT /api/v1/series/bulk` with `{ids: [...], monitored?, quality_profile_id?, library_id?, series_type?}`. Only fields that are present change.
- Validated as a whole first (every id exists, the profile and library exist), then applied in one transaction, so it's all or nothing.
- `DELETE /api/v1/series/bulk` with `{ids, delete_files?: false}`. Delete files goes through the recycle bin (C4).
- Changing `library_id` only re-points the series. Moving files on disk is out of scope, and the UI says so.

**UI:** a select mode on the Series page (checkboxes, select all in the current filter) and a bottom action bar: monitor or unmonitor, quality profile, library, series type, delete. A confirmation names the count.

**Tests:** atomicity (one bad id → nothing changes), only the given fields change, delete with files uses the recycle bin, auth list.

## D2. Manual import and reviewable library import

**Manual import (Sonarr's "Manual Import"):**
- `POST /api/v1/manual-import/scan {path, series_id?}` walks a folder (or takes one file) and returns a candidate per video file:
  - path, size;
  - the matched series (from the folder or file name, or the given `series_id`);
  - season, episodes;
  - quality and languages parsed from the name;
  - rejections, each with a reason: sample, not a video, unknown series, unknown episode, the existing file is equal or better, already imported.
- Nothing is changed by a scan.
- `POST /api/v1/manual-import {items: [{path, series_id, episode_ids, quality}], mode: "copy"|"move"}` imports each item through `importer.AttachFile` (so renaming, free space, recycle bin, replaced-file handling and events all apply).
  - `copy` hardlinks or copies (the source is kept, as for downloads); `move` removes the source afterwards.
  - The result is per item.
- Paths are server paths chosen with the existing `/api/v1/fs/browse`. Remote path mappings (C2) do not apply: the user picks a local folder.

**Reviewable library import:**
- **Today:** the 6-hourly library scan adds a series for every unknown folder name it can match on TMDB, without asking. A wrong match adds the wrong show and attaches files to it.
- **Change:** the scan stops auto-adding. Unknown folders are recorded as *unmapped folders* (new table: path, library, best TMDB guess, first seen).
- **New page, Library > Unmapped folders:** lists them with the guess. The user confirms (adds the series, then scans its files) or picks a different show, or ignores the folder.
- Files of series already in Pilot are still matched automatically, as today.

**UI:**
- a Manual Import page (Activity > Manual Import, plus a button on each series): folder picker → review table where series, episodes and quality are editable per row and rejected rows are unchecked → Import;
- the Unmapped folders page.

**Tests:**
- scan output for each rejection reason;
- import goes through `AttachFile`, so replacement, recycling and free space are covered;
- `move` removes the source only after a successful import;
- path traversal is impossible (paths are absolute and must exist);
- the scan never auto-adds;
- confirm adds and links.

## D3. Custom formats

**Model (Sonarr v4):**
- A custom format is a name plus *specifications*, each with `negate` and `required` flags.
- **Spec kinds:**
  - release title (regex);
  - language;
  - source;
  - resolution;
  - release group (regex);
  - size range;
  - indexer flag (freeleech, internal);
  - release type (single, multi, season pack).
- **Matching:** a release matches a format when every *required* spec matches and, among the non-required specs, at least one of each kind matches.

**Storage:**
- `custom_formats` (id, name, specs_json, include_in_rename);
- `quality_profile_format_scores` (profile_id, format_id, score).

  Scores live in their own table, **not** in `qualities_json`, which is the Pulse/Prism contract and must not change. Pulse-managed profiles keep their scores across syncs: `SyncQualityProfiles` must leave this table alone. A test pins that.

**Decisions:**
- the CF score is the sum of matched formats' scores in the series' profile;
- a release below the profile's `min_custom_format_score` is tagged `below_custom_format_score` (tagged, not dropped, like every other filter);
- ranking: CF score becomes the tier right after quality (before revision), as in Sonarr;
- upgrades: a release of the same quality with a higher CF score is an upgrade until `upgrade_until_cf_score`;
- `decision.Evaluate` gains the CF score; the file on disk stores its score at import.

**Also in D3:**
- restore the Custom Formats page (hidden in A1) and its presets;
- add per-format scores to the quality profile editor (B1b);
- the Sonarr import brings custom formats and scores across;
- the `{Custom Formats}` naming token.

**Tests:**
- spec matching (each kind, negate, required);
- score sum;
- the min-score tag;
- ranking with CF scores (the dead-torrent ranking suite must still pass unchanged);
- upgrades by CF score;
- Pulse sync keeps scores.

## D4. More integrations

These are plugins behind the existing registries. Each plugin gets a `secret_leak_test.go` (the redact pattern from the security work) and a sanitizer, and its errors go through `redact.Error`.

- **Download clients:** rTorrent (XML-RPC), Flood, Aria2, Synology Download Station.
- **Notifications:** Apprise, Pushbullet, Matrix, Signal (signal-cli REST), Notifiarr.
- **Import lists:** another Sonarr instance (series list), AniList, MyAnimeList, Simkl, IMDb list.

Order inside D4 follows how often each one is requested, one PR per registry.

## D5. Scene numbering (TheXEM)

**Problem:** some shows are released with scene numbering that differs from the metadata provider's (TMDB) order. Common with anime and with specials moved into seasons. Pilot then searches for, grabs and imports the wrong episode.

**Design:**
- **TVDB ID:** stored on the series, fetched from TMDB `external_ids` at add and refresh.
- **XEM data:** a daily job fetches TheXEM `map/all?origin=tvdb` for series that have it, and stores rows `(series_id, scene_season, scene_episode) → (tvdb_season, tvdb_episode)`.
- **TVDB vs TMDB order:** where TVDB and TMDB orders differ (detected per series by comparing episode counts per season), the series is flagged and mapping stays off, because a wrong mapping is worse than none. The user can turn it on per series.
- **Where it applies:**
  - search queries are sent with scene numbers;
  - RSS, search results and import parse scene numbers and map them back before episode lookup.

  One `numbering.Mapper` interface serves all three paths, so they can't drift.
- **Switch:** per-series "Use scene numbering", on by default only when XEM has data and the orders agree.

**Tests:**
- mapping both ways;
- unmapped episodes pass through;
- RSS, search and import all use the mapper;
- XEM fetch failures leave existing mappings and never block a search.

## Not in Phase D

- Moving a series' files when its library changes (D1 only re-points it).
- Multiple quality-profile languages per series (B4 kept a global preference because profiles are the Pulse contract; revisit with D3's separate-table approach if wanted).
