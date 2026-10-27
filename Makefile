# UEM — build, test and run targets.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0-dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)
GO_PKGS := ./cmd/... ./internal/...
AGENT_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
TEST_DB ?= postgres://uem:uem@localhost:5432/uem?sslmode=disable

.PHONY: all build server agent build-agents test test-unit vet web up down logs clean

all: build

build: server agent

server:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/uem-server ./cmd/server

agent:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/uem-agent ./cmd/agent

# Cross-compile the agent for every supported platform into dist/, which the
# server serves at /downloads/.
build-agents:
	@mkdir -p dist
	@for t in $(AGENT_TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "  agent $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" \
			-o dist/uem-agent-$$os-$$arch$$ext ./cmd/agent || exit 1; \
	done

vet:
	go vet $(GO_PKGS)

# Unit + integration tests (integration tests need Postgres at TEST_DB).
test: vet
	UEM_TEST_DATABASE_URL="$(TEST_DB)" go test -race $(GO_PKGS)

test-unit:
	go test $(GO_PKGS)

web:
	cd web && npm ci && npm run build

up:
	docker compose -f deploy/docker-compose.yml up -d --build

down:
	docker compose -f deploy/docker-compose.yml down

logs:
	docker compose -f deploy/docker-compose.yml logs -f server

clean:
	rm -rf bin dist
