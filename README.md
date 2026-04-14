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

Pilot is a self-hosted TV series manager with a React web UI and a REST API. It tracks a TV library, polls indexers for new episodes, grabs them through your download client, and files the finished downloads into season folders. It runs as a single Go binary, stores state in Postgres, and is configured from the UI or through environment variables.

Pilot is part of the Beacon media stack and runs alongside [Prism](https://github.com/beacon-stack/prism) (movies), [Haul](https://github.com/beacon-stack/haul) (BitTorrent), and [Pulse](https://github.com/beacon-stack/pulse) (control plane). Each of those is optional — Pilot works on its own too.

## Features

**Library management**

- Full TMDB integration for search, metadata, posters, and episode tracking
- Per-series monitoring with configurable monitor types (all, future episodes, missing, none)
- Season and episode-level monitoring controls
- Season detail view with per-season episode counts and total size
- Wanted page covering missing episodes and cutoff-unmet upgrades
- Calendar view of upcoming and recently aired episodes
- Library stats with breakdowns by quality, genre, network, and storage trends

**Release handling**

- Quality profiles with resolution, source, codec, and HDR dimensions
- Custom formats with regex matching and weighted scoring
- Title-matched release filter — releases for other shows with overlapping words in the title don't get grabbed
- Interactive search modal with pack-type filters (Season Pack / Episodes / All), quality badges, and an Episode Count tier in the comparator that surfaces season packs above individual episodes within the same quality
- Dead-torrent detection — the stallwatcher polls your download client for stalled torrents and blocklists them, with a per-episode circuit breaker so a single bad release can't cause a retry storm
- Manual search across all indexers with per-release scoring breakdown

**Automation**

- Automatic RSS sync on a configurable schedule
- Auto-search scored against your quality profile and custom formats
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
- Postgres backend
- Zero telemetry, no analytics, no crash reporting, no phoning home
- Auto-generated API key on first run
- SSRF protection on outbound connections
- Graceful shutdown with drain timeout

## Getting started

### Docker

```bash
docker run -d \
  --name pilot \
  -p 8383:8383 \
  -v /path/to/config:/config \
  -v /path/to/tv:/tv \
  ghcr.io/beacon-stack/pilot:latest
```

Open `http://localhost:8383`. Pilot generates an API key on first run — find it in Settings → App Settings.

### Docker Compose (with the rest of the stack)

The full Beacon stack — Postgres, Pulse, Pilot, Prism, Haul, and a VPN container — is wired up in [`beacon-stack/stack`](https://github.com/beacon-stack/stack). Point it at a media directory and go.

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
| `PILOT_DATABASE_DSN` | — | Postgres DSN (required) |
| `PILOT_AUTH_API_KEY` | auto | API key; autogenerated on first run if unset |
| `PILOT_PULSE_URL` | — | Pulse control-plane URL (optional) |
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

**Title matching.** The release filter runs every parsed title through a matcher in `internal/core/parser/parser.go` that rejects titles whose parsed show name doesn't match the requested series after normalization. Year suffixes, bracketed edition tags, and common release-group stylings all pass through cleanly. If a legitimate release is being rejected, the matcher is the place to start.

**Stallwatcher.** `internal/core/stallwatcher/service.go` polls the download client's stall endpoint on a configurable interval, classifies stalled torrents by reason, and blocklists them. The per-episode circuit breaker caps auto-blocklist events at three strikes per 24 hours so a misconfigured indexer can't trigger a retry storm.

**Interactive search ranking.** The season-level interactive search orders releases by Quality → Custom Format Score → Protocol → Episode Count. The Episode Count tier is what lifts season packs above individual episodes within the same quality tier, so pack-type filter pills (Season Pack / Episodes / All) default to Season Pack when the search is season-scoped.

**Regression suite.** `make test` runs the full suite in about two seconds. The guarded files and the failure modes each test exists to prevent are listed in [CLAUDE.md](CLAUDE.md).

**API surface.** Everything the UI does is available through the REST API — OpenAPI docs at `/api/docs`.

## Privacy

Pilot makes outbound connections only to services you explicitly configure: TMDB for metadata, your indexers, your download clients, your media servers, and your notification targets. No telemetry, no analytics, no crash reporting, no update checks. Credentials are stored locally and never written to logs.

## Built with Claude

Pilot was built by one person with extensive help from [Claude](https://claude.ai) (Anthropic). Architecture, design decisions, bug triage, and this README are mine. Many of the keystrokes are not. If something in the code or the docs doesn't make sense, [open an issue](https://github.com/beacon-stack/pilot/issues).

## Development

```bash
make build    # compile to bin/pilot
make run      # build + run
make dev      # hot reload (requires air)
make test     # go test ./...
make check    # golangci-lint + tsc --noEmit
make sqlc     # regenerate sqlc code
```

## Contributing

Bug reports, feature requests, and pull requests are welcome. Please open an issue before starting anything large.

## License

MIT — see [LICENSE](LICENSE).
