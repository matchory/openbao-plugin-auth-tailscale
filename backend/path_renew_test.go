package backend

import (
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/openbao/openbao/sdk/v2/logical"
)

func renew(
	t *testing.T,
	b *backend,
	s logical.Storage,
	auth *logical.Auth,
	remote string,
) (*logical.Response, error) {
	t.Helper()

	return b.HandleRequest(t.Context(), &logical.Request{
		Operation: logical.RenewOperation, Path: "login", Storage: s, Auth: auth,
		Connection: &logical.Connection{RemoteAddr: remote},
	})
}

// Guards: G5
func TestRenewSucceeds(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)
	auth := mustLogin(t, b, s, "100.64.0.5")

	resp, err := renew(t, b, s, auth, "100.64.0.5")
	if err != nil || resp == nil || resp.Auth == nil {
		t.Fatalf("renewal = %v, %v", resp, err)
	}

	if resp.Auth.TTL != 30*time.Minute || resp.Auth.MaxTTL != 4*time.Hour {
		t.Errorf("TTL, MaxTTL = %v, %v", resp.Auth.TTL, resp.Auth.MaxTTL)
	}

	if who.calls != 2 {
		t.Errorf("tailscaled was asked %d times for a login and a renewal, want 2", who.calls)
	}
}

// Guards: G5
func TestRenewOverTheOtherAddressFamily(t *testing.T) {
	t.Parallel()

	b, s, _ := loginBackend(t)
	auth := mustLogin(t, b, s, "100.64.0.5")

	if resp, err := renew(t, b, s, auth, "fd7a:115c:a1e0::5"); err != nil || resp == nil {
		t.Errorf("renewal over IPv6 after a login over IPv4 = %v, %v", resp, err)
	}
}

// Guards: G4, G5
func TestRenewRefusals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// change is what happens between the login and the renewal.
		change func(t *testing.T, b *backend, s logical.Storage, who *fakeWhoIser, auth *logical.Auth)
	}{
		{"the address passed to another node with the same tags", func(
			_ *testing.T, _ *backend, _ logical.Storage, who *fakeWhoIser, _ *logical.Auth,
		) {
			who.node.StableID = "nOTHER"
		}},
		{"the node lost the tag", func(
			_ *testing.T, _ *backend, _ logical.Storage, who *fakeWhoIser, _ *logical.Auth,
		) {
			who.node.Tags = []string{"tag:prod"}
		}},
		{"tailscaled is down", func(
			_ *testing.T, _ *backend, _ logical.Storage, who *fakeWhoIser, _ *logical.Auth,
		) {
			who.node, who.err = nil, errors.New("connection refused")
		}},
		{"the role was deleted", func(
			t *testing.T, b *backend, s logical.Storage, _ *fakeWhoIser, _ *logical.Auth,
		) {
			t.Helper()
			request(t, b, s, logical.DeleteOperation, "role/api", nil)
		}},
		{"the role binds another tag", func(
			t *testing.T, b *backend, s logical.Storage, _ *fakeWhoIser, _ *logical.Auth,
		) {
			t.Helper()
			writeRole(t, b, s, "api", map[string]any{"bound_tags": []string{"tag:example-worker"}})
		}},
		{"the role grants other policies", func(
			t *testing.T, b *backend, s logical.Storage, _ *fakeWhoIser, _ *logical.Auth,
		) {
			t.Helper()
			writeRole(t, b, s, "api", map[string]any{"token_policies": []string{"api", "more"}})
		}},
		{"the token does not say which node logged in", func(
			_ *testing.T, _ *backend, _ logical.Storage, _ *fakeWhoIser, auth *logical.Auth,
		) {
			delete(auth.InternalData, "node_id")
		}},
		{"the token names its node in another type", func(
			_ *testing.T, _ *backend, _ logical.Storage, _ *fakeWhoIser, auth *logical.Auth,
		) {
			auth.InternalData["node_id"] = 7
		}},
		{"the token does not say which role it used", func(
			_ *testing.T, _ *backend, _ logical.Storage, _ *fakeWhoIser, auth *logical.Auth,
		) {
			delete(auth.InternalData, "role")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, s, who := loginBackend(t)
			auth := mustLogin(t, b, s, "100.64.0.5")

			tc.change(t, b, s, who, auth)

			resp, err := renew(t, b, s, auth, "100.64.0.5")
			assertRefused(t, resp, err)
		})
	}
}

// The framework answers a renewal that carries no token before the backend
// sees it.
func TestRenewWithoutAuth(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)

	resp, err := renew(t, b, s, nil, "100.64.0.5")
	if err == nil || resp != nil || who.calls != 0 {
		t.Errorf(
			"renewal = %v, %v after %d questions to tailscaled; want an error",
			resp,
			err,
			who.calls,
		)
	}
}

func TestRenewPicksUpRoleTTL(t *testing.T) {
	t.Parallel()

	b, s, _ := loginBackend(t)
	auth := mustLogin(t, b, s, "100.64.0.5")

	writeRole(t, b, s, "api", map[string]any{"token_ttl": "10m"})

	resp, err := renew(t, b, s, auth, "100.64.0.5")
	if err != nil || resp == nil {
		t.Fatalf("renewal = %v, %v", resp, err)
	}

	if resp.Auth.TTL != 10*time.Minute {
		t.Errorf("TTL = %v, want the role's new 10m", resp.Auth.TTL)
	}
}

// OpenBao counts a token's maximum lifetime from when it was issued, so the
// cap for an expiring node key is counted from then too.
func TestRenewCapsLifetimesAtKeyExpiry(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)
	auth := mustLogin(t, b, s, "100.64.0.5")
	auth.IssueTime = testNow.Add(-20 * time.Minute)

	who.node.KeyExpiry = testNow.Add(5 * time.Minute)

	resp, err := renew(t, b, s, auth, "100.64.0.5")
	if err != nil || resp == nil {
		t.Fatalf("renewal = %v, %v", resp, err)
	}

	if resp.Auth.TTL != 5*time.Minute || resp.Auth.MaxTTL != 25*time.Minute {
		t.Errorf("TTL, MaxTTL = %v, %v; want 5m and 25m", resp.Auth.TTL, resp.Auth.MaxTTL)
	}
}

// Guards: G2
func TestRenewKeepsTheTokensAddresses(t *testing.T) {
	t.Parallel()

	b, s, who := loginBackend(t)
	auth := mustLogin(t, b, s, "100.64.0.5")

	who.node.Addresses = append(who.node.Addresses, netip.MustParseAddr("100.64.0.77"))
	who.node.AllowedIPs = append(who.node.AllowedIPs, netip.MustParsePrefix("100.64.0.77/32"))

	resp, err := renew(t, b, s, auth, "100.64.0.5")
	if err != nil || resp == nil {
		t.Fatalf("renewal = %v, %v", resp, err)
	}

	if got := boundCIDRs(resp.Auth); !slices.Equal(got, bothAddresses) {
		t.Errorf("BoundCIDRs after renewal = %v, want the login's %v", got, bothAddresses)
	}
}
