# ── Frontend build stage ──────────────────────────────────────────────────────
# The UI is plain JS/CSS, the same for every platform, so build it once
# on the runner's own platform instead of again under arm64 emulation.
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend

WORKDIR /app/web/ui

# Install dependencies first (cached unless package files change)
COPY web/ui/package.json web/ui/package-lock.json ./
RUN npm ci

# Copy frontend source and build
COPY web/ui/ ./
RUN npm run build
# Output lands in /app/web/static/

# ── Go build stage ───────────────────────────────────────────────────────────
# Go cross-compiles: build natively and target the image's platform with
# GOOS/GOARCH. Emulating arm64 made this step ~10x slower (466 s vs 48 s).
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS
ARG TARGETARCH

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

# Guard against silent keyless builds — see Dockerfile.beacon for the
# rationale.
RUN [ -n "${TMDB_API_KEY}" ] || (echo >&2 "ERROR: TMDB_API_KEY is empty. Refusing to bake a keyless binary. Set PILOT_TMDB_API_KEY in your shell before rebuilding." && exit 1)

# XOR-obfuscate provider keys so recognizable strings can't be recovered
# from the binary via `strings | grep`. ./tools/obfuscate emits base64
# ciphertext + XOR key as -X ldflag fragments; defaults.go deobfuscates
# at runtime. modernc/sqlite is pure Go — CGO_ENABLED=0 gives a fully
# static binary.
RUN TMDB_LDFLAGS="$(go run ./tools/obfuscate obfuscatedTMDBKey tmdbKeyXORKey "${TMDB_API_KEY}")" && \
    TRAKT_LDFLAGS="$(go run ./tools/obfuscate obfuscatedTraktClientID traktClientIDXORKey "${TRAKT_CLIENT_ID}")" && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
      -ldflags "-X github.com/beacon-stack/pilot/internal/version.Version=${VERSION} \
                -X github.com/beacon-stack/pilot/internal/version.BuildTime=${BUILD_TIME} \
                ${TMDB_LDFLAGS} \
                ${TRAKT_LDFLAGS} \
                -s -w" \
      -o /pilot ./cmd/pilot

# Files for the final image, which has no shell to make them: the config
# folder (owned by the app's user) and a "wget" link to the app, so
# compose health checks written for the old image keep working.
RUN mkdir -p /rootfs/config /rootfs/usr/bin && ln -s /pilot /rootfs/usr/bin/wget


# ── ffprobe: probe-only, static, cross-compiled ───────────────────────────────
# Only what reading a file's codec, resolution, audio and subtitle details
# needs: file input, every demuxer and parser, and the common decoders.
# No encoders, filters, network or devices. Built on the runner's own
# platform and cross-compiled for the target, so no emulation.
FROM --platform=$BUILDPLATFORM debian:bookworm-slim AS ffprobe
ARG TARGETARCH
# ffmpeg 7.1.1, checked against FFmpeg's release signing key
# (FCF9 86EA 15E6 E293 A564 4F10 B432 2F04 D676 58D8) on 2026-11-23.
ARG FFMPEG_VERSION=7.1.1
ARG FFMPEG_SHA256=733984395e0dbbe5c046abda2dc49a5544e7e0e1e2366bba849222ae9e3a03b1
RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) pkgs="gcc libc6-dev"; cross="" ;; \
      arm64) pkgs="gcc libc6-dev gcc-aarch64-linux-gnu libc6-dev-arm64-cross"; cross="--enable-cross-compile --arch=aarch64 --target-os=linux --cross-prefix=aarch64-linux-gnu-" ;; \
      *) echo "unsupported $TARGETARCH"; exit 1 ;; \
    esac; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl xz-utils make pkg-config $pkgs; \
    curl -fsSL -o /tmp/ffmpeg.tar.xz "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz"; \
    echo "${FFMPEG_SHA256}  /tmp/ffmpeg.tar.xz" | sha256sum -c -; \
    mkdir /src; tar -xJf /tmp/ffmpeg.tar.xz -C /src --strip-components=1; \
    cd /src; \
    ./configure $cross \
      --disable-everything --disable-autodetect --disable-asm \
      --disable-doc --disable-debug --disable-network \
      --disable-programs --enable-ffprobe \
      --disable-avdevice --disable-avfilter --disable-swscale --disable-swresample \
      --enable-static --disable-shared --extra-ldflags=-static --pkg-config-flags=--static \
      --enable-protocol=file,pipe \
      --enable-demuxers --enable-parsers \
      --enable-decoder=mjpeg,png,h264,hevc,mpeg1video,mpeg2video,mpeg4,msmpeg4v3,vc1,wmv3,vp8,vp9,theora \
      --enable-decoder=aac,aac_latm,ac3,eac3,dca,truehd,mlp,flac,alac,opus,vorbis,mp3,mp2,wmav2,pcm_s16le,pcm_s24le,pcm_s16be,pcm_bluray,pcm_dvd \
      --enable-decoder=srt,subrip,ass,ssa,webvtt,mov_text,pgssub,dvdsub,dvbsub,hdmv_text_subtitle; \
    make -j"$(nproc)" ffprobe; \
    install -D -m 0755 ffprobe /out/ffprobe; \
    rm -rf /src /tmp/ffmpeg.tar.xz /var/lib/apt/lists/*

# ── Final stage ───────────────────────────────────────────────────────────────
# Distroless: the app, CA certificates, timezone data and /tmp. No shell,
# no package manager, nothing to patch. Pinned by digest; bump on purpose.
FROM gcr.io/distroless/static-debian12@sha256:d75cdd72874d4790092fcb1b058493ecf6bb5bf2b2b897045b00ff01d91843f2

COPY --from=builder /pilot /pilot

# ffprobe only (not all of ffmpeg), for reading media details.
COPY --from=ffprobe /out/ffprobe /usr/local/bin/ffprobe
COPY --from=builder /rootfs/usr/bin/ /usr/bin/
COPY --from=builder --chown=1000:1000 /rootfs/config /config

ENV PILOT_DATABASE_PATH=/config/pilot.db \
    PILOT_PULSE_URL=http://pulse:9696 \
    PILOT_LOG_LEVEL=info \
    TZ=UTC \
    HOME=/config

VOLUME ["/config"]

EXPOSE 8383

# The same user ID as the old Alpine image, so existing /config folders
# keep working.
USER 1000:1000

HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 CMD ["/pilot", "healthcheck"]

ENTRYPOINT ["/pilot"]
