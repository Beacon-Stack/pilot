<p align="center">
  <h1 align="center">Pilot</h1>
  <p align="center">A self-hosted TV series manager built for simplicity.</p>
</p>
<p align="center">
  <a href="https://github.com/pilot/pilot/blob/main/LICENSE"><img src="https://img.shields.io/github/license/pilot/pilot" alt="License"></a>
  <img src="https://img.shields.io/badge/go-1.25-00ADD8?logo=go&logoColor=white" alt="Go 1.25">
</p>
<p align="center">
  <a href="https://github.com/pilot/pilot/issues">Bug Reports</a> ·
  <a href="https://github.com/pilot/pilot/wiki">Documentation</a>
</p>

---

**Pilot** monitors your TV series library, searches indexers, and automatically grabs the best available release for each episode. It is written in Go and React, starts in under a second, and idles under 60 MB of RAM.

If you are coming from Sonarr, Pilot can import your quality profiles, libraries, indexers, download clients, and series list in one step.

## Features

**Library management**

- Full TMDB integration for TV series search, metadata, posters, and episode tracking
- Per-series monitoring with configurable monitor types (all, future, missing, none)
- Season and episode level monitoring controls
- Wanted page showing missing episodes and cutoff-unmet upgrades
- Calendar view of upcoming and recently aired episodes
- Library statistics with breakdowns by quality, genre, network, and storage trends

**Quality and release handling**

- Quality profiles with resolution, source, codec, and HDR dimensions
- Quality definitions with configurable size limits per quality tier
- Custom formats with regex-based release matching and weighted scoring
- Built-in quality parser that extracts resolution, source, codec, HDR, and audio from release titles
- Manual search across all indexers with per-release scoring breakdown

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

Download clients:
- qBittorrent
- Deluge
- Transmission
- SABnzbd
- NZBGet

Media servers:
- Plex
- Jellyfin
- Emby

Notifications:
- Discord
- Slack
- Telegram
- Pushover
- Gotify
- ntfy
- Email (SMTP with STARTTLS/TLS)
- Webhook (generic HTTP)
- Custom command/script execution

**UI**

- Command palette (Cmd/Ctrl+K) with fuzzy search for pages, series, and actions
- Theme system with dark and light modes, 10+ presets (Nord, Dracula, Gruvbox, One Dark, Kanagawa, etc.)
- Per-mode theme preset tracking (separate dark and light selections)
- Directory browser for selecting library root paths
- Backup and restore from the settings page
- Error boundary with recovery UI
- WebSocket live updates for queue progress
- Responsive layout with collapsible sidebar and mobile drawer
- OpenAPI documentation at `/api/docs`

**Operations**

- Single static binary, no runtime dependencies
- SQLite by default, Postgres supported
- Zero telemetry, no analytics, no crash reporting, no phoning home
- Auto-generated API key on first run
- SSRF protection on all outbound connections (notification plugins, download clients)
- Graceful shutdown with 30 second drain timeout

## Getting Started

### Docker Compose (recommended)

```yaml
services:
  pilot:
    image: ghcr.io/pilot/pilot:latest
    ports:
      - "8383:8383"
    volumes:
      - /path/to/config:/config
      - /path/to/tv:/tv
    restart: unless-stopped
```

```bash
docker compose up -d
```

Open `http://localhost:8383`. No API keys or configuration required to get started.

The `/config` volume stores the database and auto-generated API key. The `/tv` path (or whatever you choose) must match the root path you configure in Settings > Libraries. Add additional volume mounts for each root folder you want accessible.

### Docker

```bash
docker run -d \
  --name pilot \
  -p 8383:8383 \
  -v /path/to/config:/config \
  -v /path/to/tv:/tv \
  ghcr.io/pilot/pilot:latest
```

### Build from source

Requires Go 1.25+ and Node.js 22+.

```bash
git clone https://github.com/pilot/pilot
cd pilot

# Build the frontend
cd web/ui && npm ci && npm run build && cd ../..

# Build the binary
make build

# Run
./bin/pilot
```

The default Docker image includes ffmpeg/ffprobe for media file analysis. When running from source, install ffmpeg separately if you want media scanning.

## Configuration

Pilot works out of the box with zero configuration. All settings can be changed through the web UI or via environment variables.

### Environment variables

All configuration keys can be set with the `PILOT_` prefix:

| Variable | Default | Description |
|----------|---------|-------------|
| `PILOT_SERVER_HOST` | `0.0.0.0` | Bind address |
| `PILOT_SERVER_PORT` | `8383` | HTTP port |
| `PILOT_DATABASE_DRIVER` | `sqlite` | `sqlite` or `postgres` |
| `PILOT_DATABASE_PATH` | auto-detected | SQLite file path |
| `PILOT_DATABASE_DSN` | | Postgres connection string |
| `PILOT_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `PILOT_LOG_FORMAT` | `json` | `json` or `text` |
| `PILOT_AUTH_API_KEY` | auto-generated | API key for external access |

### Config file

Pilot looks for `config.yaml` in these locations (in order):

1. `/config/config.yaml` (Docker volume mount)
2. `~/.config/pilot/config.yaml`
3. `/etc/pilot/config.yaml`
4. `./config.yaml`

```yaml
server:
  host: "0.0.0.0"
  port: 8383

database:
  driver: sqlite
  # path: "/config/pilot.db"

log:
  level: info
  format: json
```

### Database path

When running in Docker with a `/config` volume, the database defaults to `/config/pilot.db`. On bare metal it defaults to `~/.config/pilot/pilot.db`. Override with `PILOT_DATABASE_PATH` or `database.path` in the config file.

## API

Pilot exposes a REST API under `/api/v1/`. Interactive documentation is available at `/api/docs` when the server is running.

All API requests from external clients require an `X-Api-Key` header. Browser requests from the same origin are authenticated automatically. The API key is shown in Settings > App Settings, or can be set via the `PILOT_AUTH_API_KEY` environment variable.

## Sonarr Migration

Pilot can import from a running Sonarr instance. Go to Settings > Import, enter your Sonarr URL and API key, preview what will be imported, and select which categories to bring over. Supported imports:

- Quality profiles
- Libraries (root folders)
- Indexers
- Download clients
- Series (with monitoring state)

Pilot uses port 8383, so you can run both side by side during migration.

## Privacy

Pilot makes outbound connections only to services you explicitly configure: TMDB (for metadata), your indexers, your download clients, your media servers, and your notification targets. No telemetry, no analytics, no crash reporting, no update checks.

Credentials are stored in your local config and database only. They are never written to logs.

## Project Structure

```
cmd/pilot/          Entry point
internal/
  api/                  HTTP router, middleware, API v1 handlers
  config/               Configuration loading
  core/                 Domain services (show, quality, library, queue, etc.)
  db/                   Database migrations and generated query code (sqlc)
  parser/               Release title parser (quality, episode, language)
  scheduler/            Background job scheduler
plugins/
  downloaders/          qBittorrent, Deluge, Transmission, SABnzbd, NZBGet
  importlists/          TMDB, Trakt, Plex watchlist, custom list
  indexers/             Newznab, Torznab
  mediaservers/         Plex, Jellyfin, Emby
  notifications/        Discord, Slack, Telegram, Pushover, Gotify, ntfy, email, webhook, command
web/ui/                 React frontend (Vite + TypeScript)
```

## Development

```bash
# Hot reload (requires air: go install github.com/air-verse/air@latest)
make dev

# Run all checks (golangci-lint + TypeScript type check)
make check

# Run Go tests
make test

# Run frontend tests
cd web/ui && npm test

# Regenerate sqlc query code after editing .sql files
make generate
```

## How This Was Built

Pilot was built with AI assistance using [Claude Code](https://claude.ai/code) (Anthropic), with human design and architectural decisions throughout. The code is readable and tested. If something does not make sense, that is a bug. [Open an issue](https://github.com/pilot/pilot/issues).

## Contributing

Bug reports, feature requests, and pull requests are welcome. Please open an issue before starting work on large changes.

## License

MIT
