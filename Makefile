# UEM — build, test and run targets.
# Calendar versioning: releases are tagged vYYYY.MM.N (N counts releases within
# the month, from 0), so a build of a tag reports e.g. 2026.11.0. Builds between
# releases report e.g. 2026.11.0-3-g60f25a5 (3 commits later), plus -dirty when
# the tree has uncommitted changes. See `make release`.
VERSION ?= $(or $(shell git describe --tags --match 'v[0-9]*' --dirty 2>/dev/null | sed 's/^v//'),0.0.0-$(shell git rev-parse --short HEAD 2>/dev/null || echo dev))
LDFLAGS := -s -w -X main.Version=$(VERSION)

# Agents keep the last release's version while nothing they're built from has
# changed since that tag (console-only commits don't produce new agent versions
# like 2026.11.16-2-gc5b074e); otherwise they report VERSION like the server.
AGENT_SRC := cmd/agent internal/agent internal/protocol internal/agentdist go.mod go.sum
LAST_TAG := $(shell git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null)
AGENT_VERSION ?= $(if $(LAST_TAG),$(if $(shell git diff --quiet $(LAST_TAG) -- $(AGENT_SRC) 2>/dev/null && echo same),$(patsubst v%,%,$(LAST_TAG)),$(VERSION)),$(VERSION))
AGENT_LDFLAGS := -s -w -X main.Version=$(AGENT_VERSION)
GO_PKGS := ./cmd/... ./internal/...
AGENT_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
TEST_DB ?= postgres://uem:uem@localhost:5432/uem_test?sslmode=disable

.PHONY: all build server agent build-agents version agent-version release test test-unit vet web up down logs clean deploy-mac

all: build

build: server agent

server:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/uem-server ./cmd/server

agent:
	CGO_ENABLED=0 go build -ldflags "$(AGENT_LDFLAGS)" -o bin/uem-agent ./cmd/agent

# Cross-compile the agent for every supported platform into dist/, which the
# server serves at /downloads/.
build-agents:
	@mkdir -p dist
	@for t in $(AGENT_TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "  agent $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(AGENT_LDFLAGS)" \
			-o dist/uem-agent-$$os-$$arch$$ext ./cmd/agent || exit 1; \
	done
	@go run ./cmd/distmanifest dist "$(AGENT_VERSION)"

# Print the version the next build will report (server and console).
version:
	@echo $(VERSION)

# Print the version the next agent build will report.
agent-version:
	@echo $(AGENT_VERSION)

# Tag the current commit as the next calendar version (e.g. v2026.11.0), then
# rebuild the agents with `make build-agents`.
release:
	@test -z "$$(git status --porcelain)" || { echo "Commit or stash your changes first." >&2; exit 1; }
	@m=$$(date +%Y.%m); n=0; \
	while git rev-parse -q --verify "refs/tags/v$$m.$$n" >/dev/null; do n=$$((n+1)); done; \
	git tag -a "v$$m.$$n" -m "Release $$m.$$n" && echo "Tagged v$$m.$$n. Next: make build-agents"

vet:
	go vet $(GO_PKGS)

# Unit + integration tests (integration tests need Postgres at TEST_DB).
test: vet
	UEM_TEST_DATABASE_URL="$(TEST_DB)" go test -race $(GO_PKGS)

test-unit:
	go test $(GO_PKGS)

web:
	cd web && npm ci && npm run build

# Office Mac Mini: build and stage BEACON for the launchd daemons (deploy/macos).
# Then run: sudo deploy/macos/install.sh --system
deploy-mac:
	deploy/macos/install.sh

up:
	docker compose -f deploy/docker-compose.yml up -d --build

down:
	docker compose -f deploy/docker-compose.yml down

logs:
	docker compose -f deploy/docker-compose.yml logs -f server

clean:
	rm -rf bin dist
