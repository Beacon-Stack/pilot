<p align="center">
  <h1 align="center">Pilot</h1>
  <p align="center">A self-hosted TV series manager for home servers and the Beacon media stack.</p>
</p>
<p align="center">
  <a href="https://github.com/beacon-stack/pilot/blob/main/LICENSE"><img src="https://img.shields.io/github/license/beacon-stack/pilot" alt="License"></a>
  <img src="https://img.shields.io/badge/go-1.25-00ADD8?logo=go&logoColor=white" alt="Go 1.25">
</p>
<p align="center">
  <a href="https://beaconstack.io">Website</a> ·
  <a href="https://github.com/beacon-stack/pilot/issues">Bug Reports</a>
</p>

---

Pilot is a self-hosted TV series manager with a React web UI and a REST API. It tracks a TV library, polls indexers for new episodes, grabs them through your download client, and files the finished downloads into season folders. It runs as a single Go binary, stores state in SQLite, and is configured from the UI or through environment variables.

Pilot stands on its own as a Sonarr alternative. It comes into its own as part of the [Beacon Stack](https://beaconstack.io) — alongside [Pulse](https://github.com/beacon-stack/pulse) (centralized indexers + quality profiles), [Prism](https://github.com/beacon-stack/prism) (movies), and [Haul](https://github.com/beacon-stack/haul) (BitTorrent client) — where Pulse-managed indexers, cross-service quality profiles, and stall-aware torrent blocklisting all light up.

## Features

**Library management**

- Full TMDB integration for search, metadata, posters, and episode tracking
- Per-series monitoring with configurable monitor types (all, future episodes, missing, none)
- Season and episode-level monitoring controls
- Season detail view with per-season episode counts and total size
- Multi-cour anime split — shows like Jujutsu Kaisen that TMDB serves as a single 59-episode "Season 1" are projected into their broadcast cours (Season 1/2/3) using the [Anime-Lists](https://github.com/Anime-Lists/anime-lists) mapping, with cour boundaries inferred from air-date gaps when TMDB and AniDB disagree on season layout
- Wanted page covering missing episodes and cutoff-unmet upgrades
- Calendar view of upcoming and recently aired episodes
- Library stats with breakdowns by quality, genre, network, and storage trends

**Release handling**

- Quality profiles with resolution, source, codec, and HDR dimensions
- Custom formats with regex matching and weighted scoring
- Title-matched release filter — releases for other shows with overlapping words in the title don't get grabbed
- Anime-aware search — series flagged as anime via the [Anime-Lists](https://github.com/Anime-Lists/anime-lists) mapping get absolute-numbering query forms (`Show 48`, `Show - 48`) alongside the `S01E48` form, and TVDB-cour-tagged releases (`Show S03E01`) are accepted when they translate to the requested TMDB-relative episode
- Interactive search modal with pack-type filters (Season Pack / Episodes / All), quality badges, click-to-sort columns (Size, Quality, Seeds), and an Episode Count tier in the comparator that surfaces season packs above individual episodes within the same quality
- Dead-torrent detection — the stallwatcher polls your download client for stalled torrents and blocklists them, with a per-episode circuit breaker so a single bad release can't cause a retry storm
- Manual search across all indexers with per-release scoring breakdown

**Automation**

- Automatic RSS sync on a configurable schedule
- Auto-search scored against your quality profile and release profiles
- Auto-import of completed downloads with rename support
- Configurable episode naming (standard, daily, anime)
- Season folder organization
- Import lists from TMDB, Trakt, Plex Watchlist, and custom URL lists
- Activity log pruning

**Integrations**

- **Indexers:** Newznab (NZBgeek, NZBFinder), Torznab (Prowlarr, Jackett), Pulse-managed indexers
- **Download clients:** [Haul](https://github.com/beacon-stack/haul), qBittorrent, Deluge, Transmission, SABnzbd, NZBGet
- **Media servers:** Plex, Jellyfin, Emby
- **Notifications:** Discord, Slack, Telegram, Pushover, Gotify, ntfy, email, webhook, custom command
- **Migration:** one-click import of quality profiles, libraries, indexers, download clients, and monitored series from a running Sonarr instance

**UI**

- Command palette (Cmd/Ctrl+K) with fuzzy search for pages, series, and actions
- Dark and light themes with 10+ presets shared across the Beacon services
- Live queue updates over WebSocket
- OpenAPI documentation at `/api/docs`

**Operations**

- Single static Go binary, no runtime dependencies
- SQLite database: no separate database server
- Zero telemetry, no analytics, no crash reporting, no phoning home
- Auto-generated API key on first run
- SSRF protection on outbound connections
- Graceful shutdown with drain timeout

## Getting started

### Docker

Pilot runs on its own, like Sonarr: add your indexers and a download client in Settings. On first start it creates a TV library at `/data/media/tv` and a set of default quality profiles.

Set the two paths in the compose file:

- **`/config`**: Pilot's database and settings. Keep it on a local disk.
- **`/data`**: your media *and* downloads, in one folder. Give every Beacon app the same host folder at `/data`: imports are then instant hardlinks, and every app sees the same paths so no path mappings are needed. It must be writable by uid 1000.

The compose file is [`docker/docker-compose.yml`](docker/docker-compose.yml). Then:

```bash
docker compose -f docker/docker-compose.yml up -d
```

The web UI is at `http://<this-host>:8383`. No login is needed from your own network, and connections from anywhere else are refused. To reach Pilot from outside, or to put a login in front of it for everyone, set a username and password in Settings → App Settings → Security. Scripts and other apps use the API key from Settings → App Settings.

### Running it with the other Beacon apps

Each app ships its own compose file; run the ones you want and connect them. [beaconstack.io](https://beaconstack.io) has larger examples, from Pilot + Haul up to the full stack with Pulse and a VPN.

### Build from source

Requires Go 1.25+ and Node 22+. The default Docker image includes ffmpeg/ffprobe for media scanning; install it separately if you build locally and want that feature.

```bash
git clone https://github.com/beacon-stack/pilot
cd pilot
cd web/ui && npm ci && npm run build && cd ../..
make build
./bin/pilot
```

Pilot listens on port 8383 by default. If something else already owns that port, override with `PILOT_SERVER_PORT`.

## Configuration

Most settings live in the web UI. For the ones you'll want at container-start time, use environment variables or a YAML config file at `/config/config.yaml` (also searched at `~/.config/pilot/config.yaml` and `./config.yaml`).

| Variable | Default | Description |
|---|---|---|
| `PILOT_SERVER_PORT` | `8383` | Web UI and API port |
| `PILOT_DATABASE_PATH` | `/config/pilot.db` | SQLite database file |
| `PILOT_AUTH_API_KEY` | auto | API key; autogenerated on first run if unset |
| `PILOT_PULSE_URL` | — | Pulse URL, e.g. `http://192.168.1.10:9696` (optional) |
| `PILOT_PULSE_API_KEY` | — | Pulse's API key, needed with `PILOT_PULSE_URL` |
| `ADVERTISE_HOST` | container hostname | Address Pulse uses to reach Pilot; set it to this host's IP when the apps run as separate compose projects |
| `PILOT_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `PILOT_LOG_FORMAT` | `json` | `json` or `text` |

## Migrating from Sonarr

Pilot imports from a running Sonarr instance in one pass. Open Settings → Import, enter the Sonarr URL and API key, preview what will be brought over, and pick the categories to bring in. Supported:

- Quality profiles
- Libraries (root folders)
- Indexers
- Download clients
- Series with monitoring state

## Where Pilot fits in the Beacon stack

```
┌─────────────┐     registers      ┌──────────┐
│    Pulse    │◄───────────────────┤  Pilot   │
│ (control    │────managed─────────►  (TV)    │
│   plane)    │  indexers + profiles│          │
└─────────────┘                    └────┬─────┘
                                        │
                                 grab torrent
                                        ▼
                                   ┌─────────┐
                                   │  Haul   │
                                   │  (BT)   │
                                   └─────────┘
```

If the full stack is running, Pilot pulls shared indexers and quality profiles from Pulse, sends torrent grabs through Haul, and polls Haul's `/api/v1/stalls` endpoint to blocklist dead torrents. Standalone is fine — Pulse and Haul are optional.

## Power user notes

**Title matching.** The release filter runs every parsed title through a matcher in `internal/parser/title.go` that rejects titles whose parsed show name doesn't match the requested series after normalization. Year suffixes, bracketed edition tags, and common release-group stylings all pass through cleanly. If a legitimate release is being rejected, the matcher is the place to start.

**Stallwatcher.** `internal/core/stallwatcher/service.go` polls the download client's stall endpoint on a configurable interval, classifies stalled torrents by reason, and blocklists them. The per-episode circuit breaker caps auto-blocklist events at three strikes per 24 hours so a misconfigured indexer can't trigger a retry storm.

**Search ranking.** Releases are ordered by quality, then revision (PROPER/REPACK), then episode count, then seed count in log buckets, then age. The episode-count tier is what lifts season packs above single episodes of the same quality. Seed buckets with an age cap stop an old torrent with an inflated seed count from winning.

**Tests.** `make test` runs the Go suite; the ranking, stall-watcher, blocklist and authentication tests pin behaviour that has broken before, so fix the code rather than the test when one fails.

**API surface.** Everything the UI does is available through the REST API — OpenAPI docs at `/api/docs`.

## Privacy

Pilot makes outbound connections only to services you explicitly configure: TMDB for metadata, your indexers, your download clients, your media servers, and your notification targets. It asks GitHub about new versions only when you press Check for updates, and your browser loads the interface fonts from Google Fonts. No telemetry, no analytics, no crash reporting. Credentials are stored locally and never written to logs.

## Built with Claude

Pilot was built by one person with extensive help from [Claude](https://claude.ai) (Anthropic). Architecture, design decisions, bug triage, and this README are mine. Many of the keystrokes are not. If something in the code or the docs doesn't make sense, [open an issue](https://github.com/beacon-stack/pilot/issues).

## Development

```bash
make build    # compile to bin/pilot
make run      # build + run
make dev      # hot reload (requires air)
make test     # go test ./...
make check    # lint, dead-code checks, type-check, frontend tests
make generate # regenerate database code after changing SQL
```

## Contributing

Bug reports, feature requests, and pull requests are welcome. Please open an issue before starting anything large.

## License

MIT — see [LICENSE](LICENSE).
