package tsauth_test

import (
	"os"
	"regexp"
	"testing"
)

// TestE2ERunsTheShippedTailscale fails when the end-to-end test would run a
// tailscaled other than the version of the client the plugin is built with,
// as it would after a dependency update that left the script behind.
func TestE2ERunsTheShippedTailscale(t *testing.T) {
	t.Parallel()

	goMod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}

	script, err := os.ReadFile("test/integration/run.sh")
	if err != nil {
		t.Fatal(err)
	}

	shipped := regexp.MustCompile(`(?m)^\s*tailscale\.com v([0-9.]+)$`).FindSubmatch(goMod)
	tested := regexp.MustCompile(`(?m)^TAILSCALE_VERSION=\$\{TAILSCALE_VERSION:-([0-9.]+)\}$`).
		FindSubmatch(script)

	if shipped == nil || tested == nil {
		t.Fatalf("version not found: go.mod %q, run.sh %q", shipped, tested)
	}

	if string(shipped[1]) != string(tested[1]) {
		t.Errorf(
			"go.mod builds with tailscale.com %s, but run.sh tests tailscaled %s: "+
				"update TAILSCALE_VERSION and its checksums",
			shipped[1], tested[1],
		)
	}
}

// TestLintVersionsAgree fails when make lint and CI would lint with
// different versions of golangci-lint.
func TestLintVersionsAgree(t *testing.T) {
	t.Parallel()

	makefile, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatal(err)
	}

	ci, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}

	local := regexp.MustCompile(`(?m)^GOLANGCI_LINT_VERSION := ([0-9.]+)$`).FindSubmatch(makefile)
	pinned := regexp.MustCompile(`(?m)golangci-lint-action@.*\n(?:.*\n)*?\s+version: v([0-9.]+)$`).
		FindSubmatch(ci)

	if local == nil || pinned == nil {
		t.Fatalf("version not found: Makefile %q, ci.yml %q", local, pinned)
	}

	if string(local[1]) != string(pinned[1]) {
		t.Errorf("make lint uses golangci-lint %s, ci.yml %s", local[1], pinned[1])
	}
}
