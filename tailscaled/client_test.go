package tailscaled_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"

	"github.com/matchory/openbao-plugin-auth-tailscale/tailscaled"
)

var addr = netip.MustParseAddr("100.64.0.5")

// fakeDaemon serves the LocalAPI's whois endpoint on a Unix socket and
// returns the socket's path.
func fakeDaemon(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()

	socket := filepath.Join(shortTempDir(t), "ts.sock")

	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", socket)
	if err != nil {
		t.Fatalf("listen on %s: %v", socket, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/localapi/v0/whois", handler)

	server := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}

	go func() { _ = server.Serve(listener) }()

	t.Cleanup(func() { _ = server.Close() })

	return socket
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

	if !node.Tagged || node.Shared || node.Expired || !node.KeyExpiry.IsZero() {
		t.Errorf("Tagged, Shared, Expired, KeyExpiry = %v, %v, %v, %v",
			node.Tagged, node.Shared, node.Expired, node.KeyExpiry)
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

	if node.Tagged {
		t.Error("Tagged = true for a node without tags")
	}
}

func TestWhoIsNotFound(t *testing.T) {
	t.Parallel()

	socket := fakeDaemon(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no match for IP:port", http.StatusNotFound)
	})

	node, err := tailscaled.New(socket).WhoIs(t.Context(), addr)
	if err == nil || node != nil {
		t.Errorf("WhoIs = %v, %v; want nil and an error", node, err)
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
