.PHONY: all build build-cli build-tui test tidy fmt clean package help

BINARY_CLI := bin/lnr
BINARY_TUI := bin/lnr-tui

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
	-X github.com/SakagamiJun/lnovel_tui/pkg/version.Version=$(VERSION) \
	-X github.com/SakagamiJun/lnovel_tui/pkg/version.GitCommit=$(GIT_COMMIT) \
	-X github.com/SakagamiJun/lnovel_tui/pkg/version.BuildDate=$(BUILD_DATE)

all: build

build: build-cli build-tui

build-cli:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_CLI) ./cmd/lnr

build-tui:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_TUI) ./cmd/lnr-tui

test:
	go test -v -race ./...

tidy:
	go mod tidy
	go mod verify

fmt:
	go fmt ./...

clean:
	rm -rf bin dist

package:
	./scripts/package.sh

help:
	@echo "Available targets:"
	@echo "  make build      - Build both CLI (lnr) and TUI (lnr-tui) binaries"
	@echo "  make build-cli  - Build CLI binary into bin/lnr"
	@echo "  make build-tui  - Build TUI binary into bin/lnr-tui"
	@echo "  make test       - Run all test suites with race detector"
	@echo "  make tidy       - Tidy and verify go modules"
	@echo "  make fmt        - Format Go source code"
	@echo "  make clean      - Remove build and distribution artifacts"
	@echo "  make package    - Package distribution archives"
