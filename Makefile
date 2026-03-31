## Screenarr Makefile

MODULE  := github.com/screenarr/screenarr
BINARY  := screenarr
BIN_DIR := ./bin
CMD     := ./cmd/screenarr

VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS   := -ldflags "\
  -X $(MODULE)/internal/version.Version=$(VERSION) \
  -X $(MODULE)/internal/version.BuildTime=$(BUILD_TIME) \
  -X $(MODULE)/internal/config.DefaultTMDBKey=$(TMDB_API_KEY) \
  -X $(MODULE)/internal/config.DefaultTraktClientID=$(TRAKT_CLIENT_ID)"

IMAGE ?= ghcr.io/screenarr/screenarr

.PHONY: build run dev test test/unit test/cover test/race test/frontend \
        lint check generate clean docker docker/push help

## build: Compile the binary into ./bin/screenarr
build:
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY) $(CMD)

## run: Build and run the server (no hot reload)
run: build
	$(BIN_DIR)/$(BINARY)

## dev: Run with hot reload via air (install: go install github.com/air-verse/air@latest)
dev:
	air

## test: Run all tests
test:
	go test ./...

## test/unit: Run only unit tests (skips integration tests)
test/unit:
	go test -short ./...

## test/cover: Run tests and open coverage report
test/cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## test/race: Run tests with the race detector (used in CI)
test/race:
	go test -race ./...

## test/frontend: Run frontend tests
test/frontend:
	cd web/ui && npm test

## lint: Run golangci-lint
lint:
	golangci-lint run

## check: Run all pre-push checks (Go lint + TypeScript type-check + frontend tests)
check: lint
	cd web/ui && npx tsc --noEmit

## generate: Regenerate sqlc query code
generate:
	sqlc generate

## clean: Remove build artifacts
clean:
	rm -rf $(BIN_DIR) tmp coverage.out coverage.html

## docker: Build the Docker image locally
docker:
	docker build \
	  --build-arg VERSION=$(VERSION) \
	  --build-arg BUILD_TIME=$(BUILD_TIME) \
	  --build-arg TMDB_API_KEY=$(TMDB_API_KEY) \
	  --build-arg TRAKT_CLIENT_ID=$(TRAKT_CLIENT_ID) \
	  -t $(IMAGE):$(VERSION) \
	  -t $(IMAGE):latest \
	  -f docker/Dockerfile .

## docker/push: Build and push the image to the registry
docker/push: docker
	docker push $(IMAGE):$(VERSION)
	docker push $(IMAGE):latest

## help: Print this help message
help:
	@grep -E '^## ' Makefile | sed 's/## //'
