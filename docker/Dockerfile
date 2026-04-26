# ── Frontend build stage ──────────────────────────────────────────────────────
FROM node:22-alpine AS frontend

WORKDIR /app/web/ui

# Install dependencies first (cached unless package files change)
COPY web/ui/package.json web/ui/package-lock.json ./
RUN npm ci

# Copy frontend source and build
COPY web/ui/ ./
RUN npm run build
# Output lands in /app/web/static/

# ── Go build stage ───────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG BUILD_TIME=unknown
ARG TMDB_API_KEY
ARG TRAKT_CLIENT_ID

WORKDIR /app

# Download dependencies first (cached unless go.mod/go.sum change)
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Overwrite web/static/ with the freshly built frontend
COPY --from=frontend /app/web/static/ ./web/static/

# XOR-obfuscate provider keys so recognizable strings can't be recovered
# from the binary via `strings | grep`. ./tools/obfuscate emits base64
# ciphertext + XOR key as -X ldflag fragments; defaults.go deobfuscates
# at runtime. modernc/sqlite is pure Go — CGO_ENABLED=0 gives a fully
# static binary.
RUN TMDB_LDFLAGS="$(go run ./tools/obfuscate obfuscatedTMDBKey tmdbKeyXORKey "${TMDB_API_KEY}")" && \
    TRAKT_LDFLAGS="$(go run ./tools/obfuscate obfuscatedTraktClientID traktClientIDXORKey "${TRAKT_CLIENT_ID}")" && \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-X github.com/beacon-stack/pilot/internal/version.Version=${VERSION} \
                -X github.com/beacon-stack/pilot/internal/version.BuildTime=${BUILD_TIME} \
                ${TMDB_LDFLAGS} \
                ${TRAKT_LDFLAGS} \
                -s -w" \
      -o /pilot ./cmd/pilot

# ── Final stage ───────────────────────────────────────────────────────────────
# Alpine with ffmpeg (includes ffprobe) for media scanning out of the box.
FROM alpine:3.21

# ffmpeg pulls in ffprobe. ca-certificates is needed for outbound HTTPS.
# tzdata provides timezone data for correct time display.
RUN apk add --no-cache ffmpeg ca-certificates tzdata && \
    adduser -D -u 1000 pilot

# The binary
COPY --from=builder /pilot /pilot

# Beacon-stack-shaped defaults — the published image expects to land in a
# compose with services named `postgres` and (optionally) `pulse`. For
# standalone use, override these via the container's `environment:` block;
# env always beats Dockerfile ENV. See docker/docker-compose.yml for the
# standalone recipe.
ENV PILOT_DATABASE_DRIVER=postgres \
    PILOT_DATABASE_DSN="postgres://pilot@postgres:5432/pilot_db?sslmode=disable" \
    PILOT_DATABASE_PASSWORD_FILE=/run/secrets/pilot.txt \
    PILOT_PULSE_URL=http://pulse:9696 \
    PILOT_PULSE_API_KEY_FILE=/run/secrets/pulse-api-key.txt \
    PILOT_LOG_LEVEL=info \
    TZ=UTC

# Default data directory (mount a volume here in production)
VOLUME ["/config"]

EXPOSE 8383

USER pilot

ENTRYPOINT ["/pilot"]
