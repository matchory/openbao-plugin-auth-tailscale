package tailscaled_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"

	"github.com/matchory/openbao-plugin-auth-tailscale/identity"
	"github.com/matchory/openbao-plugin-auth-tailscale/tailscaled"
)

var addr = netip.MustParseAddr("100.64.0.5")

// fakeDaemon serves the LocalAPI's whois endpoint on a Unix socket and
// returns the socket's path.
func fakeDaemon(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()

	return fakeDaemonCounting(t, handler, nil)
}

// fakeDaemonCounting is fakeDaemon, and adds one to connections for every
// connection a client opens.
func fakeDaemonCounting(t *testing.T, handler http.HandlerFunc, connections *atomic.Int32) string {
	t.Helper()

	return serveLocalAPI(t, "/localapi/v0/whois", handler, connections)
}

// serveLocalAPI serves handler at path on a Unix socket and returns the
// socket's path.
func serveLocalAPI(
	t *testing.T,
	path string,
	handler http.HandlerFunc,
	connections *atomic.Int32,
) string {
	t.Helper()

	socket := filepath.Join(shortTempDir(t), "ts.sock")

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatalf("listen on %s: %v", socket, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, handler)

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: time.Second,
		ConnState: func(_ net.Conn, state http.ConnState) {
			if connections != nil && state == http.StateNew {
				connections.Add(1)
			}
		},
	}

	go func() { _ = server.Serve(listener) }()

	t.Cleanup(func() { _ = server.Close() })

	return socket
}

// shortTempDir is t.TempDir with a path short enough for a Unix socket:
// macOS limits a socket path to 104 bytes.
func shortTempDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "ts") //nolint:usetesting // path length
	if err != nil {
		t.Fatalf("create a temporary directory: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	return dir
}

func respond(response any) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

func taggedNode() *tailcfg.Node {
	return &tailcfg.Node{
		StableID: "nTEST1",
		Name:     "api-1.example.ts.net.",
		Tags:     []string{"tag:example-api"},
		Addresses: []netip.Prefix{
			netip.MustParsePrefix("100.64.0.5/32"),
			netip.MustParsePrefix("fd7a:115c:a1e0::5/128"),
		},
		AllowedIPs: []netip.Prefix{
			netip.MustParsePrefix("100.64.0.5/32"),
			netip.MustParsePrefix("fd7a:115c:a1e0::5/128"),
			netip.MustParsePrefix("100.100.104.251/32"),
		},
		Hostinfo: (&tailcfg.Hostinfo{
			RoutableIPs: []netip.Prefix{netip.MustParsePrefix("100.100.104.251/32")},
		}).View(),
	}
}

func whoIs(node *tailcfg.Node) *apitype.WhoIsResponse {
	return &apitype.WhoIsResponse{Node: node, UserProfile: &tailcfg.UserProfile{}}
}

func TestWhoIsConverts(t *testing.T) {
	t.Parallel()

	var asked string

	socket := fakeDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query().Get("addr")
		respond(whoIs(taggedNode()))(w, r)
	})

	node, err := tailscaled.New(socket).WhoIs(t.Context(), addr)
	if err != nil {
		t.Fatalf("WhoIs: %v", err)
	}

	if asked != "100.64.0.5" {
		t.Errorf("asked the daemon about %q, want 100.64.0.5", asked)
	}

	if node.StableID != "nTEST1" || node.Name != "api-1.example.ts.net." {
		t.Errorf("StableID, Name = %q, %q", node.StableID, node.Name)
	}

	if !slices.Equal(node.Tags, []string{"tag:example-api"}) {
		t.Errorf("Tags = %v", node.Tags)
	}

	wantAddrs := []netip.Addr{addr, netip.MustParseAddr("fd7a:115c:a1e0::5")}
	if !slices.Equal(node.Addresses, wantAddrs) {
		t.Errorf("Addresses = %v, want %v", node.Addresses, wantAddrs)
	}

	if len(node.AllowedIPs) != 3 {
		t.Errorf("AllowedIPs = %v, want three prefixes", node.AllowedIPs)
	}

	if !slices.Equal(
		node.AdvertisedRoutes,
		[]netip.Prefix{netip.MustParsePrefix("100.100.104.251/32")},
	) {
		t.Errorf("AdvertisedRoutes = %v", node.AdvertisedRoutes)
	}

	if node.Shared || node.Expired || !node.KeyExpiry.IsZero() {
		t.Errorf(
			"Shared, Expired, KeyExpiry = %v, %v, %v",
			node.Shared,
			node.Expired,
			node.KeyExpiry,
		)
	}
}

func TestWhoIsKeyExpiry(t *testing.T) {
	t.Parallel()

	expiry := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	n := taggedNode()
	n.KeyExpiry = expiry
	n.Expired = true

	node, err := tailscaled.New(fakeDaemon(t, respond(whoIs(n)))).WhoIs(t.Context(), addr)
	if err != nil {
		t.Fatalf("WhoIs: %v", err)
	}

	if !node.KeyExpiry.Equal(expiry) || !node.Expired {
		t.Errorf("KeyExpiry, Expired = %v, %v", node.KeyExpiry, node.Expired)
	}
}

func TestWhoIsShared(t *testing.T) {
	t.Parallel()

	n := taggedNode()
	n.Sharer = 42

	node, err := tailscaled.New(fakeDaemon(t, respond(whoIs(n)))).WhoIs(t.Context(), addr)
	if err != nil {
		t.Fatalf("WhoIs: %v", err)
	}

	if !node.Shared {
		t.Error("Shared = false for a node with a sharer")
	}
}

func TestWhoIsUntagged(t *testing.T) {
	t.Parallel()

	n := taggedNode()
	n.Tags = nil

	node, err := tailscaled.New(fakeDaemon(t, respond(whoIs(n)))).WhoIs(t.Context(), addr)
	if err != nil {
		t.Fatalf("WhoIs: %v", err)
	}

	if len(node.Tags) != 0 {
		t.Errorf("Tags = %v for a node without tags", node.Tags)
	}
}

// A daemon can answer without the node's Hostinfo; its view would panic.
func TestWhoIsWithoutHostinfo(t *testing.T) {
	t.Parallel()

	n := taggedNode()
	n.Hostinfo = tailcfg.HostinfoView{}

	node, err := tailscaled.New(fakeDaemon(t, respond(whoIs(n)))).WhoIs(t.Context(), addr)
	if err != nil || len(node.AdvertisedRoutes) != 0 {
		t.Errorf("WhoIs = %v, %v; want a node that advertises nothing", node, err)
	}
}

func TestWhoIsNotFound(t *testing.T) {
	t.Parallel()

	socket := fakeDaemon(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no match for IP:port", http.StatusNotFound)
	})

	// An address nobody has is a decision about the caller, not a failure
	// of the daemon.
	node, err := tailscaled.New(socket).WhoIs(t.Context(), addr)
	if !errors.Is(err, identity.ErrUnknownAddress) || node != nil {
		t.Errorf("WhoIs = %v, %v; want nil and ErrUnknownAddress", node, err)
	}
}

// Guards: G4
func TestWhoIsNilNode(t *testing.T) {
	t.Parallel()

	node, err := tailscaled.New(fakeDaemon(t, respond(struct{}{}))).WhoIs(t.Context(), addr)
	if err == nil || node != nil {
		t.Errorf("WhoIs = %v, %v; want nil and an error", node, err)
	}
}

// Guards: G4
func TestWhoIsHonoursContext(t *testing.T) {
	t.Parallel()

	socket := fakeDaemon(t, func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()

	if _, err := tailscaled.New(socket).WhoIs(ctx, addr); err == nil {
		t.Error("WhoIs returned no error after its deadline")
	}

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("WhoIs took %v after a 100ms deadline", elapsed)
	}
}

// Without a daemon the library keeps trying for several seconds, so the
// caller's deadline is what bounds a login.
func TestWhoIsNoSocket(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	socket := filepath.Join(shortTempDir(t), "absent.sock")

	if _, err := tailscaled.New(socket).WhoIs(ctx, addr); err == nil {
		t.Error("WhoIs returned no error without a daemon")
	}

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("WhoIs took %v after a 200ms deadline", elapsed)
	}
}

// tailscaled keeps a connection open for as long as a client does, so a
// client must reuse its connection.
func TestWhoIsReusesItsConnection(t *testing.T) {
	t.Parallel()

	var connections atomic.Int32

	client := tailscaled.New(fakeDaemonCounting(t, respond(whoIs(taggedNode())), &connections))
	for range 20 {
		if _, err := client.WhoIs(t.Context(), addr); err != nil {
			t.Fatalf("WhoIs: %v", err)
		}
	}

	if n := connections.Load(); n != 1 {
		t.Errorf("20 questions opened %d connections, want 1", n)
	}
}

func TestCheckTag(t *testing.T) {
	t.Parallel()

	for _, tag := range []string{"tag:example-api", "tag:a1"} {
		if err := tailscaled.CheckTag(tag); err != nil {
			t.Errorf("CheckTag(%q) = %v", tag, err)
		}
	}

	for _, tag := range []string{"example-api", "tag:", "tag:has space", "tag:1st", ""} {
		if err := tailscaled.CheckTag(tag); err == nil {
			t.Errorf("CheckTag(%q) accepted it", tag)
		}
	}
}

func TestSelf(t *testing.T) {
	t.Parallel()

	running := &ipnstate.Status{
		BackendState: "Running",
		Self:         &ipnstate.PeerStatus{ID: "nSELF"},
	}

	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{name: "running", handler: respond(running), want: "nSELF"},
		{name: "stopped", handler: respond(&ipnstate.Status{
			BackendState: "Stopped",
			Self:         &ipnstate.PeerStatus{ID: "nSELF"},
		})},
		{name: "needs login", handler: respond(&ipnstate.Status{BackendState: "NeedsLogin"})},
		{
			name:    "running without a node",
			handler: respond(&ipnstate.Status{BackendState: "Running"}),
		},
		{name: "running with an empty ID", handler: respond(&ipnstate.Status{
			BackendState: "Running",
			Self:         &ipnstate.PeerStatus{},
		})},
		{name: "daemon fails", handler: func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var query string

			socket := serveLocalAPI(
				t,
				"/localapi/v0/status",
				func(w http.ResponseWriter, r *http.Request) {
					query = r.URL.RawQuery
					tc.handler(w, r)
				},
				nil,
			)

			id, err := tailscaled.New(socket).Self(t.Context())
			if tc.want == "" {
				if err == nil || id != "" {
					t.Errorf("Self() = %q, %v; want an error", id, err)
				}

				return
			}

			if err != nil || id != tc.want {
				t.Errorf("Self() = %q, %v; want %q", id, err, tc.want)
			}

			// The peer list can be large, and nothing in it is needed.
			if query != "peers=false" {
				t.Errorf("asked for status with %q, want peers=false", query)
			}
		})
	}
}
