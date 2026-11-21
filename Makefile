BINARY := openbao-plugin-auth-tailscale
GOOS ?= linux
GOARCH ?= $(shell go env GOARCH)
# The release this build is, such as v0.1.0: the plugin reports it to OpenBao.
# Empty for a build that is not a release.
VERSION ?=

.PHONY: build test lint vuln e2e

# The flags of a release build, so a local build has the release's checksum.
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -buildvcs=false \
		-ldflags "-s -w -buildid= -X github.com/matchory/$(BINARY)/backend.version=$(VERSION)" \
		-o bin/$(BINARY)_$(GOOS)_$(GOARCH) ./cmd/$(BINARY)

# CI adds report options through the two variables; the tests are the same.
test:
	go tool gotestsum $(GOTESTSUM_FLAGS) -- -race -cover $(TESTFLAGS) ./...

lint:
	golangci-lint run

vuln:
	go tool govulncheck ./...

# Needs Linux and Docker: see docs/testing.md.
e2e:
	test/integration/run.sh
