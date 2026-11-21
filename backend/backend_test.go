package backend

import (
	"bytes"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/openbao/openbao/sdk/v2/logical"

	"github.com/matchory/openbao-plugin-auth-tailscale/identity"
)

var testNow = time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

// testBackend returns a backend whose tailscaled is w and whose clock stands
// still at testNow, with empty in-memory storage.
func testBackend(t *testing.T, w identity.WhoIser) (*backend, logical.Storage) {
	t.Helper()

	b, s, _ := testBackendWithLog(t, w)

	return b, s
}

// testBackendWithLog is testBackend, and also returns what the backend logs.
func testBackendWithLog(
	t *testing.T,
	w identity.WhoIser,
) (*backend, logical.Storage, *bytes.Buffer) {
	t.Helper()

	log := &bytes.Buffer{}
	conf := logical.TestBackendConfig()
	conf.StorageView = &logical.InmemStorage{}
	conf.Logger = hclog.New(&hclog.LoggerOptions{Output: log, Level: hclog.Debug})

	b := newBackend()
	b.newWhoIser = func(string) identity.WhoIser { return w }
	b.now = func() time.Time { return testNow }

	if err := b.Setup(t.Context(), conf); err != nil {
		t.Fatalf("set up the backend: %v", err)
	}

	return b, conf.StorageView, log
}

// request sends one request and fails the test on a Go error. An error
// response is returned for the test to inspect.
func request(
	t *testing.T,
	b *backend,
	s logical.Storage,
	op logical.Operation,
	path string,
	data map[string]any,
) *logical.Response {
	t.Helper()

	resp, err := b.HandleRequest(t.Context(), &logical.Request{
		Operation: op, Path: path, Storage: s, Data: data,
	})
	if err != nil {
		t.Fatalf("%s %s: %v", op, path, err)
	}

	return resp
}

func TestFactory(t *testing.T) {
	t.Parallel()

	conf := logical.TestBackendConfig()
	conf.StorageView = &logical.InmemStorage{}

	b, err := Factory(t.Context(), conf)
	if err != nil {
		t.Fatalf("Factory: %v", err)
	}

	if b.Type() != logical.TypeCredential {
		t.Errorf("Type() = %v, want a credential backend", b.Type())
	}
}

func TestLoginIsUnauthenticated(t *testing.T) {
	t.Parallel()

	b, _ := testBackend(t, nil)
	if !slices.Contains(b.SpecialPaths().Unauthenticated, "login") {
		t.Errorf(
			"unauthenticated paths = %v, want login among them",
			b.SpecialPaths().Unauthenticated,
		)
	}
}

// Not parallel: it changes the package's version for its duration.
func TestRunningVersion(t *testing.T) {
	version = "v1.2.3"

	t.Cleanup(func() { version = "" })

	if got := newBackend().RunningVersion; got != "v1.2.3" {
		t.Errorf("RunningVersion = %q, want the build's version v1.2.3", got)
	}
}
