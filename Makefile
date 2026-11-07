# Nextron Relic build targets.
#
#   make            build ./relic for the host
#   make test       run unit and integration tests
#   make dist       cross-compile static binaries for all platforms into dist/
#   make docker-test run the test suite on Linux in a container
#   make check-go   fail unless the Go toolchain matches the toolchain line in go.mod

# Version from the nearest tag without the leading "v" (v0.1.0 -> 0.1.0).
VERSION ?= $(patsubst v%,%,$(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0-dev))
COMMIT  ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null)
PKG      = github.com/Nextron-Labs/nextron-relic
LDFLAGS  = -s -w -X $(PKG)/internal/version.Version=$(VERSION) -X $(PKG)/internal/version.Commit=$(COMMIT)
GOFLAGS  = -trimpath

PLATFORMS = windows/amd64 windows/386 windows/arm64 \
            linux/amd64 linux/386 linux/arm64 \
            darwin/amd64 darwin/arm64

.PHONY: build test vet dist clean docker-test check-go

build:
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o relic ./cmd/relic

test:
	go test ./...

vet:
	go vet ./...
	@for p in $(PLATFORMS); do \
		GOOS=$${p%/*} GOARCH=$${p#*/} go vet ./... || exit 1; \
	done

dist:
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; \
		[ $$os = windows ] && ext=".exe"; \
		out=dist/relic-$(VERSION)-$$os-$$arch$$ext; \
		echo "building $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $$out ./cmd/relic || exit 1; \
	done
	@cd dist && if command -v sha256sum >/dev/null; then sha256sum relic-*; else shasum -a 256 relic-*; fi > SHA256SUMS

# Release builds must use the patched toolchain pinned in go.mod.
check-go:
	@want=$$(awk '/^toolchain /{print $$2}' go.mod); have=$$(go env GOVERSION); \
	if [ -n "$$want" ] && [ "$$want" != "$$have" ]; then \
		echo "Go toolchain is $$have, go.mod requires $$want" >&2; exit 1; \
	fi; \
	echo "Go toolchain $$have"

docker-test:
	docker run --rm -v "$(CURDIR)":/src -w /src golang:1.26-trixie go test ./...

clean:
	rm -rf dist relic relic.exe
