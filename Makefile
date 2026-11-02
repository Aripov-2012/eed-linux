GOCMD = go
VERSION ?= dev
COMMIT ?= unknown
BUILD_DATE ?= unknown
CHANNEL ?= development
PACKAGE = eed/internal/version
LDFLAGS = -s -w \
	-X $(PACKAGE).Version=$(VERSION) \
	-X $(PACKAGE).Commit=$(COMMIT) \
	-X $(PACKAGE).BuildDate=$(BUILD_DATE) \
	-X $(PACKAGE).ReleaseChannel=$(CHANNEL)

.PHONY: all fmt-check test vet race build run release-linux-amd64 checksums

all: test vet build

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt changes are required"; exit 1)

build:
	$(GOCMD) build ./...

test:
	$(GOCMD) test ./...

race:
	$(GOCMD) test -race ./...

vet:
	$(GOCMD) vet ./...

run:
	$(GOCMD) run ./cmd/eed --help

release-linux-amd64:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOCMD) build -trimpath -ldflags "$(LDFLAGS) -X $(PACKAGE).BuildTarget=linux/amd64" -o dist/eed-linux-amd64 ./cmd/eed

checksums:
	cd dist && sha256sum eed-* > SHA256SUMS
