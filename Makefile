BINARY := openbao-plugin-auth-tailscale
GOOS ?= linux
GOARCH ?= $(shell go env GOARCH)

.PHONY: build test lint vuln e2e

# The flags of a release build, so a local build has the release's checksum.
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -buildvcs=false \
		-ldflags "-s -w -buildid=" -o bin/$(BINARY)_$(GOOS)_$(GOARCH) ./cmd/$(BINARY)

test:
	go tool gotestsum -- -race -cover ./...

lint:
	golangci-lint run

vuln:
	go tool govulncheck ./...

# Needs Linux, Docker and sudo: see docs/testing.md.
e2e:
	test/integration/run.sh
