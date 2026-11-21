package backend

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/go-sockaddr"
	"github.com/openbao/openbao/sdk/v2/logical"

	"github.com/matchory/openbao-plugin-auth-tailscale/identity"
)

// fakeWhoIser answers every question with node or err and records what it
// was asked.
type fakeWhoIser struct {
	node *identity.Node
	err  error
	// block makes WhoIs wait for its context to end, as a hung daemon would.
	block bool

	calls    int
	asked    netip.Addr
	deadline time.Time
}

func (f *fakeWhoIser) WhoIs(ctx context.Context, addr netip.Addr) (*identity.Node, error) {
	f.calls++
	f.asked = addr
	f.deadline, _ = ctx.Deadline()

	if f.block {
		<-ctx.Done()

		return nil, ctx.Err()
	}

	return f.node, f.err
}

func validNode() *identity.Node {
	return &identity.Node{
		StableID: "nTEST1",
		Name:     "api-1.example.ts.net.",
		Tags:     []string{"tag:prod", "tag:example-api"},
		Addresses: []netip.Addr{
			netip.MustParseAddr("100.64.0.5"),
			netip.MustParseAddr("fd7a:115c:a1e0::5"),
		},
		AllowedIPs: []netip.Prefix{
			netip.MustParsePrefix("100.64.0.5/32"),
			netip.MustParsePrefix("fd7a:115c:a1e0::5/128"),
		},
		Tagged: true,
	}
}

// loginBackend returns a backend with the role "api", bound to
// tag:example-api, and a tailscaled that answers with the valid node.
func loginBackend(t *testing.T) (*backend, logical.Storage, *fakeWhoIser) {
	t.Helper()

	who := &fakeWhoIser{node: validNode()}
	b, s := testBackend(t, who)

	resp := writeRole(t, b, s, "api", map[string]any{
		"bound_tags":     []string{"tag:example-api"},
		"token_ttl":      "30m",
		"token_max_ttl":  "4h",
		"token_policies": []string{"api"},
	})
	if resp.IsError() {
		t.Fatalf("write role: %v", resp.Error())
	}

	return b, s, who
}

func login(b *backend, s logical.Storage, remote, role string) (*logical.Response, error) {
	req := &logical.Request{
		Operation: logical.UpdateOperation, Path: "login", Storage: s,
		Data:       map[string]any{"role": role},
		Connection: &logical.Connection{RemoteAddr: remote},
	}

	return b.HandleRequest(context.Background(), req)
}

func mustLogin(t *testing.T, b *backend, s logical.Storage, remote string) *logical.Auth {
	t.Helper()

	resp, err := login(b, s, remote, "api")
	if err != nil || resp == nil || resp.Auth == nil {
		t.Fatalf("login from %s: response %v, error %v", remote, resp, err)
	}

	return resp.Auth
}

func boundCIDRs(auth *logical.Auth) []string {
	cidrs := make([]string, 0, len(auth.BoundCIDRs))
	for _, cidr := range auth.BoundCIDRs {
		cidrs = append(cidrs, cidr.String())
	}

	slices.Sort(cidrs)

	return cidrs
}

// The form go-sockaddr prints a single address in.
var bothAddresses = []string{"100.64.0.5", "fd7a:115c:a1e0::5"}

// Guards: G2
func TestLoginSucceeds(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)
	auth := mustLogin(t, b, s, "100.64.0.5")

	if who.asked != netip.MustParseAddr("100.64.0.5") {
		t.Errorf("asked tailscaled about %v", who.asked)
	}

	if !slices.Equal(auth.Policies, []string{"api"}) || auth.TTL != 30*time.Minute ||
		auth.MaxTTL != 4*time.Hour {
		t.Errorf("Policies, TTL, MaxTTL = %v, %v, %v", auth.Policies, auth.TTL, auth.MaxTTL)
	}

	if auth.Alias == nil || auth.Alias.Name != "api" || auth.DisplayName != "api" {
		t.Errorf("Alias, DisplayName = %v, %q", auth.Alias, auth.DisplayName)
	}

	if got := boundCIDRs(auth); !slices.Equal(got, bothAddresses) {
		t.Errorf("BoundCIDRs = %v, want %v", got, bothAddresses)
	}

	wantMetadata := map[string]string{
		"role":      "api",
		"node_id":   "nTEST1",
		"node_name": "api-1.example.ts.net.",
		"tags":      "tag:example-api,tag:prod",
		"address":   "100.64.0.5",
	}
	if !reflect.DeepEqual(auth.Metadata, wantMetadata) {
		t.Errorf("Metadata = %v, want %v", auth.Metadata, wantMetadata)
	}

	if auth.InternalData["node_id"] != "nTEST1" || auth.InternalData["role"] != "api" {
		t.Errorf("InternalData = %v", auth.InternalData)
	}

	if !auth.Renewable {
		t.Error("the token is not renewable")
	}
}

// Guards: G2
func TestLoginOverIPv6(t *testing.T) {
	t.Parallel()

	b, s, _ := loginBackend(t)
	auth := mustLogin(t, b, s, "fd7a:115c:a1e0::5")

	if got := boundCIDRs(auth); !slices.Equal(got, bothAddresses) {
		t.Errorf("BoundCIDRs = %v, want %v", got, bothAddresses)
	}

	if auth.Metadata["address"] != "fd7a:115c:a1e0::5" {
		t.Errorf("address = %q", auth.Metadata["address"])
	}
}

// Guards: G1
func TestLoginMappedIPv4(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)
	mustLogin(t, b, s, "::ffff:100.64.0.5")

	if who.asked != netip.MustParseAddr("100.64.0.5") {
		t.Errorf("asked tailscaled about %v, want 100.64.0.5", who.asked)
	}
}

// Guards: G1, G3
func TestLoginRefusals(t *testing.T) {
	t.Parallel()

	change := func(f func(*identity.Node)) func(*fakeWhoIser) {
		return func(w *fakeWhoIser) { f(w.node) }
	}

	tests := []struct {
		name   string
		remote string
		role   string
		who    func(*fakeWhoIser)
	}{
		{name: "empty address", remote: "", role: "api"},
		{name: "address with a port", remote: "100.64.0.5:443", role: "api"},
		{name: "public address", remote: "203.0.113.7", role: "api"},
		{name: "loopback", remote: "127.0.0.1", role: "api"},
		{name: "no role named", remote: "100.64.0.5", role: ""},
		{name: "unknown role", remote: "100.64.0.5", role: "nope"},
		{name: "tailscaled fails", remote: "100.64.0.5", role: "api", who: func(w *fakeWhoIser) {
			w.node, w.err = nil, errors.New("no match")
		}},
		{
			name:   "tailscaled names no node",
			remote: "100.64.0.5",
			role:   "api",
			who: func(w *fakeWhoIser) {
				w.node = nil
			},
		},
		{
			name:   "untagged node",
			remote: "100.64.0.5",
			role:   "api",
			who: change(func(n *identity.Node) {
				n.Tagged, n.Tags = false, nil
			}),
		},
		{
			name:   "shared node",
			remote: "100.64.0.5",
			role:   "api",
			who: change(func(n *identity.Node) {
				n.Shared = true
			}),
		},
		{
			name:   "expired node",
			remote: "100.64.0.5",
			role:   "api",
			who: change(func(n *identity.Node) {
				n.Expired = true
			}),
		},
		{
			name:   "subnet router",
			remote: "100.64.0.5",
			role:   "api",
			who: change(func(n *identity.Node) {
				n.AllowedIPs = append(n.AllowedIPs, netip.MustParsePrefix("10.0.0.0/24"))
			}),
		},
		{name: "address of another node", remote: "100.64.0.9", role: "api"},
		{
			name:   "node lacks the tag",
			remote: "100.64.0.5",
			role:   "api",
			who: change(func(n *identity.Node) {
				n.Tags = []string{"tag:example-worker"}
			}),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, s, who := loginBackend(t)
			if tc.who != nil {
				tc.who(who)
			}

			resp, err := login(b, s, tc.remote, tc.role)
			if !errors.Is(err, logical.ErrPermissionDenied) || resp != nil {
				t.Errorf("login = %v, %v; want no response and ErrPermissionDenied", resp, err)
			}
		})
	}
}

// Guards: G1
func TestLoginWithoutConnection(t *testing.T) {
	t.Parallel()

	b, s, _ := loginBackend(t)

	resp, err := b.HandleRequest(t.Context(), &logical.Request{
		Operation: logical.UpdateOperation,
		Path:      "login",
		Storage:   s,
		Data:      map[string]any{"role": "api"},
	})
	if !errors.Is(err, logical.ErrPermissionDenied) || resp != nil {
		t.Errorf("login = %v, %v; want no response and ErrPermissionDenied", resp, err)
	}
}

// Guards: G6
func TestLoginRefusalsAreIndistinguishable(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)
	unknownResp, unknownErr := login(b, s, "100.64.0.5", "nope")

	who.node.Tags = []string{"tag:example-worker"}
	tagResp, tagErr := login(b, s, "100.64.0.5", "api")

	if unknownResp != nil || tagResp != nil || unknownErr == nil ||
		unknownErr.Error() != tagErr.Error() {
		t.Errorf(
			"unknown role: %v, %v; missing tag: %v, %v",
			unknownResp,
			unknownErr,
			tagResp,
			tagErr,
		)
	}
}

// Guards: G6
func TestLoginAsksTailscaledBeforeLookingUpTheRole(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)
	if _, err := login(b, s, "100.64.0.5", "nope"); err == nil {
		t.Fatal("login with an unknown role succeeded")
	}

	// Otherwise the time a refusal takes would tell a caller whether a
	// role exists.
	if who.calls != 1 {
		t.Errorf("tailscaled was asked %d times for an unknown role, want 1", who.calls)
	}
}

func TestLoginDoesNotAskForNonTailnetAddress(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)
	if _, err := login(b, s, "203.0.113.7", "api"); err == nil {
		t.Fatal("login from a public address succeeded")
	}

	if who.calls != 0 {
		t.Errorf("tailscaled was asked %d times about a public address", who.calls)
	}
}

// Guards: G4
func TestLoginUsesTimeout(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)
	request(t, b, s, logical.UpdateOperation, "config", map[string]any{"timeout": "5s"})

	before := time.Now()
	mustLogin(t, b, s, "100.64.0.5")

	if left := who.deadline.Sub(before); left < 4*time.Second || left > 6*time.Second {
		t.Errorf("tailscaled got %v to answer, want about 5s", left)
	}
}

// Guards: G4
func TestLoginRefusedWhenTailscaledHangs(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)
	who.block = true

	request(t, b, s, logical.UpdateOperation, "config", map[string]any{"timeout": "1s"})

	start := time.Now()

	_, err := login(b, s, "100.64.0.5", "api")
	if !errors.Is(err, logical.ErrPermissionDenied) {
		t.Errorf("login error = %v, want ErrPermissionDenied", err)
	}

	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("the login took %v with a 1s timeout", elapsed)
	}
}

func TestLoginUsesConfiguredSocket(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)

	var socket string

	b.newWhoIser = func(path string) identity.WhoIser {
		socket = path

		return who
	}

	request(t, b, s, logical.UpdateOperation, "config", map[string]any{"socket": "/run/x.sock"})
	mustLogin(t, b, s, "100.64.0.5")

	if socket != "/run/x.sock" {
		t.Errorf("asked the tailscaled at %q, want /run/x.sock", socket)
	}
}

func TestLoginCapsTTLAtKeyExpiry(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)
	who.node.KeyExpiry = testNow.Add(10 * time.Minute)

	auth := mustLogin(t, b, s, "100.64.0.5")
	if auth.TTL != 10*time.Minute || auth.MaxTTL != 10*time.Minute {
		t.Errorf("TTL, MaxTTL = %v, %v; want 10m for both", auth.TTL, auth.MaxTTL)
	}
}

func TestLoginCapsMountDefaultTTLAtKeyExpiry(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)
	writeRole(t, b, s, "api", map[string]any{"token_ttl": "0", "token_max_ttl": "0"})

	who.node.KeyExpiry = testNow.Add(10 * time.Minute)

	auth := mustLogin(t, b, s, "100.64.0.5")
	if auth.TTL != 10*time.Minute || auth.MaxTTL != 10*time.Minute {
		t.Errorf("TTL, MaxTTL = %v, %v; want 10m for both", auth.TTL, auth.MaxTTL)
	}
}

// Guards: G4
func TestLoginNeverCaches(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)
	mustLogin(t, b, s, "100.64.0.5")
	mustLogin(t, b, s, "100.64.0.5")

	if who.calls != 2 {
		t.Errorf("tailscaled was asked %d times for two logins", who.calls)
	}
}

// Guards: G2
func TestLoginBoundCIDRsComeFromNodeOnly(t *testing.T) {
	t.Parallel()

	b, s, _ := loginBackend(t)

	// What a tampered or migrated entry could hold: the API cannot write it.
	role := &roleEntry{BoundTags: []string{"tag:example-api"}}
	role.TokenBoundCIDRs = []*sockaddr.SockAddrMarshaler{
		{SockAddr: sockaddr.MustIPAddr("10.0.0.0/8")},
	}
	role.TokenStrictlyBindIP = true

	entry, err := logical.StorageEntryJSON("role/api", role)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Put(t.Context(), entry); err != nil {
		t.Fatal(err)
	}

	auth := mustLogin(t, b, s, "100.64.0.5")
	if got := boundCIDRs(auth); !slices.Equal(got, bothAddresses) {
		t.Errorf("BoundCIDRs = %v, want %v", got, bothAddresses)
	}
}
