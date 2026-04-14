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

Pilot is a self-hosted TV series manager with a React web UI and a REST API. It does what Sonarr does — monitors a library, searches indexers, grabs the best available release for each episode, imports completed downloads, talks to your media server — but in a single Go binary with a modern UI and sensible defaults. It runs standalone or slots into the Beacon stack alongside [Prism](https://github.com/beacon-stack/prism) (movies), [Haul](https://github.com/beacon-stack/haul) (BitTorrent), and [Pulse](https://github.com/beacon-stack/pulse) (control plane).

## Is this for you?

Pilot is designed to feel familiar if you're coming from Sonarr but sharper in the places Sonarr has long needed sharpening. The one-click Sonarr import pulls your entire setup — libraries, quality profiles, indexers, download clients, monitored series — in about thirty seconds, so there's nothing to rebuild from scratch. Once it's running, the UI stays out of your way for the day-to-day (dashboard, calendar, wanted list, queue) but the parts that have always been painful are genuinely better: interactive release search has proper pack-type filters and Sonarr-parity Episode Count ranking so season packs actually float to the top, the stallwatcher auto-blocklists dead torrents so you stop re-grabbing the same broken release, and custom formats come with sensible defaults so you're not stuck doing a weekend of tutorials before anything works.

You'll probably like Pilot if you:

- Already use Sonarr and want a drop-in upgrade with zero reconfiguration
- Have been frustrated by Sonarr's interactive search not surfacing season packs clearly
- Have wasted a retry budget on dead torrents and want that handled automatically
- Want your TV manager to be in active development rather than maintenance mode

## Features

**Library management**

- Full TMDB integration for search, metadata, posters, episode tracking
- Per-series monitoring with configurable monitor types (all, future episodes, missing, none)
- Season and episode-level monitoring controls
- Season detail view with per-season episode counts and total size
- Wanted page covering missing episodes and cutoff-unmet upgrades
- Calendar view of upcoming and recently aired episodes
- Library stats with breakdowns by quality, genre, network, and storage trends

**Release handling**

- Quality profiles with resolution, source, codec, and HDR dimensions
- Custom formats with regex matching and weighted scoring
- Strict title matching in the release filter — Pilot won't grab "Breaking Bad Bulgaria" when you asked for "Breaking Bad"
- Interactive search modal with pack-type filters (Season Pack / Episodes / All), quality badges, and a Sonarr-parity ranking that surfaces season packs at the top within each quality tier
- Automatic dead-torrent detection and blocklisting — Pilot's stallwatcher polls Haul (or your download client) for stalled torrents and blocklists them with a per-episode circuit breaker so you don't retry the same bad release forever
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

**UI**

- Command palette (Cmd/Ctrl+K) with fuzzy search for pages, series, and actions
- Dark and light themes with 10+ presets shared across the Beacon services
- Live queue updates over WebSocket — no polling
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

> **Running Sonarr too?** Pilot uses port 8383, so you can run both side by side during migration.

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

## Sonarr migration

Pilot imports from a running Sonarr instance in one step. Open Settings → Import, enter your Sonarr URL and API key, preview what will be brought over, and pick which categories to import. Supported:

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

Pilot runs fine standalone — Pulse and Haul are optional. If you run the full stack, Pilot pulls shared indexers and quality profiles from Pulse, sends torrent grabs through Haul, and polls Haul's `/api/v1/stalls` endpoint to automatically blocklist dead torrents before they waste another retry.

## Power user notes

**Strict title matching.** The release filter runs every parsed title through a strict matcher (`internal/core/parser/parser.go`) that blocks false positives like "Breaking Bad Bulgaria" when you're searching for "Breaking Bad." The matcher handles year suffixes, bracketed edition tags, and common release-group stylings. If a legitimate release is getting rejected, the matcher is where to start.

**Stallwatcher.** `internal/core/stallwatcher/service.go` polls Haul's stall endpoint on a configurable interval and classifies stalled torrents by reason. A per-episode circuit breaker limits auto-blocklist events to three strikes in 24 hours so that even a misconfigured indexer can't cause a retry storm. Comprehensive regression tests live alongside.

**Interactive search.** The season-level interactive search is built around Sonarr's DownloadDecisionComparer ranking: Quality → Custom Format Score → Protocol → Episode Count. Season packs naturally surface at the top within each quality tier via the Episode Count tier — no custom formats required. Filter pills (Season Pack / Episodes / All) default to Season Pack when the search is season-scoped so the UI matches intent.

**Regression suite.** Pilot has a hard-won test suite covering the dead-torrent, wrong-torrent, and quality-profile failure modes it's shipped in the past. `make test` runs it in about two seconds. See [CLAUDE.md](CLAUDE.md) for the guarded files and the rationale behind each.

**API surface.** Everything the UI does is available through the REST API — OpenAPI docs at `/api/docs`.

## Privacy

Pilot makes outbound connections only to services you explicitly configure: TMDB for metadata, your indexers, your download clients, your media servers, and your notification targets. No telemetry, no analytics, no crash reporting, no update checks. Credentials are stored locally and never written to logs.

## Built with Claude

Pilot was built by one person with extensive help from [Claude](https://claude.ai) (Anthropic). Architecture, design decisions, bug triage, and this README are mine. Many of the keystrokes are not. If something in the code or the docs doesn't make sense, that's a bug worth reporting — [open an issue](https://github.com/beacon-stack/pilot/issues).

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
