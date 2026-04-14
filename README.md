<p align="center">
  <h1 align="center">Pilot</h1>
  <p align="center">A self-hosted TV series manager built for simplicity.</p>
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

**Pilot** monitors your TV series library, searches indexers, and automatically grabs the best available release for each episode. It is written in Go and React, starts in under a second, and idles under 60 MB of RAM.

Pilot is part of the [Beacon](https://beaconstack.io) media stack — it runs alongside [Prism](https://github.com/beacon-stack/prism) (movies), [Haul](https://github.com/beacon-stack/haul) (BitTorrent downloader), and [Pulse](https://github.com/beacon-stack/pulse) (control plane) — but it also runs standalone if you only want a TV manager.

If you are coming from Sonarr, Pilot can import your quality profiles, libraries, indexers, download clients, and series list in one step.

## Features

**Library management**

- Full TMDB integration for TV series search, metadata, posters, and episode tracking
- Per-series monitoring with configurable monitor types (all, future, missing, none)
- Season and episode level monitoring controls
- Season detail view with per-season episode counts + total size
- Interactive release search with pack-type filtering (Season Pack / Episodes / All) and Sonarr-parity Episode Count ranking so season packs surface at the top
- Strict title-match filter — the "Breaking Bad Bulgaria" class of wrong-torrent bug is blocked at the filter stage
- Wanted page showing missing episodes and cutoff-unmet upgrades
- Calendar view of upcoming and recently aired episodes
- Library statistics with breakdowns by quality, genre, network, and storage trends

**Quality and release handling**

- Quality profiles with resolution, source, codec, and HDR dimensions
- Quality definitions with configurable size limits per quality tier
- Custom formats with regex-based release matching and weighted scoring
- Built-in quality parser that extracts resolution, source, codec, HDR, and audio from release titles
- Manual search across all indexers with per-release scoring breakdown
- Dead-torrent detection and blocklisting — Pilot's stallwatcher polls Haul for stalled torrents and blocklists them automatically, with a three-strikes circuit breaker per episode to prevent infinite retry loops

**Automation**

- Automatic RSS sync on a configurable schedule
- Auto-search across all indexers, scoring against your quality profile and custom formats
- Auto-import of completed downloads into your library with rename support
- Configurable episode naming formats (standard, daily, anime)
- Season folder organization
- Import lists from TMDB Popular, TMDB Trending, Trakt Popular, Trakt Trending, Trakt Custom Lists, Plex Watchlist, and custom URL lists
- Activity log pruning (older than 30 days, runs daily)

**Integrations**

Indexers:
- Newznab (NZBgeek, NZBFinder, etc.)
- Torznab (Prowlarr, Jackett)
- [Pulse](https://github.com/beacon-stack/pulse) — centrally managed indexers pushed from the Pulse control plane

Download clients:
- [Haul](https://github.com/beacon-stack/haul) — first-class integration with the Beacon torrent client
- qBittorrent, Deluge, Transmission
- SABnzbd, NZBGet

Media servers:
- Plex, Jellyfin, Emby

Notifications:
- Discord, Slack, Telegram, Pushover, Gotify, ntfy
- Email (SMTP with STARTTLS/TLS)
- Webhook (generic HTTP)
- Custom command/script execution

**UI**

- Command palette (Cmd/Ctrl+K) with fuzzy search for pages, series, and actions
- Interactive release search modal with pack-type filters, quality badges, seed-count column, and "override" button for blocklisted rows
- Theme system with dark and light modes, 10+ presets shared across the Beacon services
- Directory browser for selecting library root paths
- WebSocket live updates for queue progress
- OpenAPI documentation at `/api/docs`

**Operations**

- Single static binary, no runtime dependencies
- Postgres backend
- Zero telemetry, no analytics, no crash reporting, no phoning home
- Auto-generated API key on first run
- SSRF protection on all outbound connections (notification plugins, download clients)
- Graceful shutdown with drain timeout

## Getting started

### Docker Compose (recommended, as part of the Beacon stack)

The easiest way to run Pilot is as part of the full Beacon stack — see [`beacon-stack/stack`](https://github.com/beacon-stack/stack) for the full docker-compose setup with Postgres, Pulse, Pilot, Prism, and Haul behind a VPN container.

### Standalone Docker

```bash
docker run -d \
  --name pilot \
  -p 8383:8383 \
  -v /path/to/config:/config \
  -v /path/to/tv:/tv \
  ghcr.io/beacon-stack/pilot:latest
```

Open `http://localhost:8383`. No configuration required to get started.

### Build from source

Requires Go 1.25+ and Node.js 22+.

```bash
git clone https://github.com/beacon-stack/pilot
cd pilot
cd web/ui && npm ci && npm run build && cd ../..
make build
./bin/pilot
```

The default Docker image includes ffmpeg/ffprobe for media file analysis. When building from source, install ffmpeg separately if you want media scanning.

> **Running Sonarr too?** Pilot uses port 8383 so you can run both side by side during migration.

## Configuration

Pilot works with zero configuration. All settings are editable through the web UI or via environment variables.

### Key environment variables

| Variable | Default | Description |
|---|---|---|
| `PILOT_SERVER_HOST` | `0.0.0.0` | Bind address |
| `PILOT_SERVER_PORT` | `8383` | HTTP port |
| `PILOT_DATABASE_DSN` | | Postgres connection string |
| `PILOT_AUTH_API_KEY` | auto-generated | API key for external access |
| `PILOT_PULSE_URL` | | Pulse control-plane URL (optional) |
| `PILOT_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `PILOT_LOG_FORMAT` | `json` | `json` or `text` |

### Config file

Pilot looks for `config.yaml` in `/config/config.yaml`, `~/.config/pilot/config.yaml`, `/etc/pilot/config.yaml`, or `./config.yaml` (in that order).

## Sonarr migration

Pilot can import from a running Sonarr instance. Go to Settings → Import, enter your Sonarr URL and API key, preview what will be imported, and select which categories to bring over. Supported imports:

- Quality profiles
- Libraries (root folders)
- Indexers
- Download clients
- Series (with monitoring state)

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

Pilot is fine standalone — Pulse and Haul are optional dependencies. If you run the full stack, Pilot pulls its indexers and quality profiles from Pulse, sends torrent grabs through Haul, and polls Haul's `/api/v1/stalls` endpoint to automatically blocklist dead torrents.

## Privacy

Pilot makes outbound connections only to services you explicitly configure: TMDB (for metadata), your indexers, your download clients, your media servers, and your notification targets. No telemetry, no analytics, no crash reporting, no update checks. Credentials are stored locally and never written to logs.

## Project structure

```
cmd/pilot/          Entry point
internal/
  api/              HTTP router, middleware, v1 handlers
  config/           Configuration loading
  core/             Domain services (show, quality, library, queue, stallwatcher, etc.)
  db/               Database migrations and generated query code (sqlc)
  parser/           Release title parser (quality, episode, language, title-match)
  pulse/            Pulse control-plane integration
  scheduler/        Background job scheduler
plugins/
  downloaders/      Haul, qBittorrent, Deluge, Transmission, SABnzbd, NZBGet
  importlists/      TMDB, Trakt, Plex watchlist, custom list
  indexers/         Newznab, Torznab
  mediaservers/     Plex, Jellyfin, Emby
  notifications/    Discord, Slack, Telegram, Pushover, Gotify, ntfy, email, webhook, command
web/ui/             React 19 + TypeScript + Vite frontend
```

## Development

```bash
make build         # compile binary to bin/pilot
make run           # build + run
make dev           # hot reload with air
make test          # go test ./...
make check         # golangci-lint + tsc --noEmit
make sqlc          # regenerate SQLC code
```

The project has a regression suite that runs in under 2 seconds and locks in the dead-torrent failure modes that have regressed in the past — see [CLAUDE.md](CLAUDE.md) for the guarded files.

## Contributing

Bug reports, feature requests, and pull requests are welcome. Please open an issue before starting large changes.

## License

MIT
