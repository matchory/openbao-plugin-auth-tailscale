package identity

import (
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"
)

var now = time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

// self is the stable ID of the node tailscaled runs as in these tests.
const self = "nSELF"

func validNode() *Node {
	return &Node{
		StableID: "nTEST1",
		Name:     "api-1.example.ts.net.",
		Tags:     []string{"tag:example-api", "tag:prod"},
		Addresses: []netip.Addr{
			netip.MustParseAddr("100.64.0.5"),
			netip.MustParseAddr("fd7a:115c:a1e0::5"),
		},
		AllowedIPs: []netip.Prefix{
			netip.MustParsePrefix("100.64.0.5/32"),
			netip.MustParsePrefix("fd7a:115c:a1e0::5/128"),
		},
	}
}

// Guards: G1
func TestParseRemoteAddr(t *testing.T) {
	t.Parallel()

	accepted := map[string]string{
		"100.64.0.5":                "100.64.0.5",
		"100.127.255.254":           "100.127.255.254",
		"fd7a:115c:a1e0::4428:ef52": "fd7a:115c:a1e0::4428:ef52",
		"::ffff:100.64.0.5":         "100.64.0.5",
	}
	for in, want := range accepted {
		got, err := ParseRemoteAddr(in)
		if err != nil {
			t.Errorf("ParseRemoteAddr(%q): unexpected error %v", in, err)

			continue
		}

		if got != netip.MustParseAddr(want) {
			t.Errorf("ParseRemoteAddr(%q) = %v, want %v", in, got, want)
		}
	}

	refused := []string{
		"100.128.0.1", "10.0.0.1", "127.0.0.1", "203.0.113.7", "fd7a:115c:a1e1::1",
		"", "100.64.0.5:443", "fe80::1%eth0", "fd7a:115c:a1e0::5%eth0", "not-an-ip",
		"::ffff:10.0.0.1",
	}
	for _, in := range refused {
		if _, err := ParseRemoteAddr(in); !errors.Is(err, ErrNotTailnet) {
			t.Errorf("ParseRemoteAddr(%q): error = %v, want ErrNotTailnet", in, err)
		}
	}
}

// Guards: G3
func TestCheckNode(t *testing.T) {
	t.Parallel()

	v4 := netip.MustParseAddr("100.64.0.5")
	allow := func(prefix string) func(*Node) {
		return func(n *Node) { n.AllowedIPs = append(n.AllowedIPs, netip.MustParsePrefix(prefix)) }
	}

	tests := []struct {
		name   string
		change func(*Node)
		remote netip.Addr
		want   error
	}{
		{name: "valid over IPv4", remote: v4},
		{name: "valid over IPv6", remote: netip.MustParseAddr("fd7a:115c:a1e0::5")},
		{
			name:   "untagged",
			change: func(n *Node) { n.Tags = nil },
			remote: v4,
			want:   ErrUntagged,
		},
		{name: "shared in", change: func(n *Node) { n.Shared = true }, remote: v4, want: ErrShared},
		{
			name:   "marked expired",
			change: func(n *Node) { n.Expired = true },
			remote: v4,
			want:   ErrExpired,
		},
		{
			name:   "key expiry passed",
			change: func(n *Node) { n.KeyExpiry = now.Add(-time.Second) },
			remote: v4, want: ErrExpired,
		},
		{
			name:   "key expiry ahead",
			change: func(n *Node) { n.KeyExpiry = now.Add(time.Hour) },
			remote: v4,
		},
		{
			name:   "address of another node",
			remote: netip.MustParseAddr("100.64.0.9"),
			want:   ErrAddressMismatch,
		},
		{name: "hosts a Service over IPv4", change: allow("100.100.104.251/32"), remote: v4},
		{
			name:   "hosts a Service over IPv6",
			change: allow("fd7a:115c:a1e0::1a28:68fc/128"),
			remote: v4,
		},
		{name: "subnet router", change: allow("10.0.0.0/24"), remote: v4, want: ErrRoutes},
		{
			name: "subnet router whose route is only advertised",
			change: func(n *Node) {
				n.AdvertisedRoutes = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")}
			},
			remote: v4,
			want:   ErrRoutes,
		},
		{
			name:   "no allowed addresses",
			change: func(n *Node) { n.AllowedIPs = nil },
			remote: v4,
			want:   ErrNotAllowed,
		},
		{
			name: "may not send from the remote address",
			change: func(n *Node) {
				n.AllowedIPs = []netip.Prefix{netip.MustParsePrefix("fd7a:115c:a1e0::5/128")}
			},
			remote: v4,
			want:   ErrNotAllowed,
		},
		{
			name:   "tailscaled's own node",
			change: func(n *Node) { n.StableID = self },
			remote: v4,
			want:   ErrSelf,
		},
		{name: "exit node", change: allow("0.0.0.0/0"), remote: v4, want: ErrRoutes},
		{name: "IPv6 exit node", change: allow("::/0"), remote: v4, want: ErrRoutes},
		{
			name:   "a range of tailnet addresses",
			change: allow("100.64.0.0/24"),
			remote: v4,
			want:   ErrRoutes,
		},
		{
			name:   "one address outside the tailnet",
			change: allow("192.0.2.1/32"),
			remote: v4,
			want:   ErrRoutes,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			n := validNode()
			if tc.change != nil {
				tc.change(n)
			}

			if err := CheckNode(n, Host{StableID: self}, tc.remote, now); !errors.Is(err, tc.want) {
				t.Errorf("CheckNode() = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCheckNodeNil(t *testing.T) {
	t.Parallel()

	if err := CheckNode(
		nil,
		Host{StableID: self},
		netip.MustParseAddr("100.64.0.5"),
		now,
	); err == nil {
		t.Error("CheckNode(nil) = nil, want an error")
	}
}

// Guards: G3
func TestMatchTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		bound []string
		want  error
	}{
		{name: "one of the node's tags", bound: []string{"tag:example-api"}},
		{name: "all of the node's tags", bound: []string{"tag:example-api", "tag:prod"}},
		{name: "a tag the node lacks", bound: []string{"tag:example-worker"}, want: ErrTagMismatch},
		{
			name:  "one present, one missing",
			bound: []string{"tag:example-api", "tag:example-worker"},
			want:  ErrTagMismatch,
		},
		{name: "nothing bound", bound: nil, want: ErrTagMismatch},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := MatchTags(validNode(), tc.bound); !errors.Is(err, tc.want) {
				t.Errorf("MatchTags() = %v, want %v", err, tc.want)
			}
		})
	}
}

// Guards: G2
func TestBoundCIDRs(t *testing.T) {
	t.Parallel()

	want := []netip.Prefix{
		netip.MustParsePrefix("100.64.0.5/32"),
		netip.MustParsePrefix("fd7a:115c:a1e0::5/128"),
	}
	if got := BoundCIDRs(validNode()); !slices.Equal(got, want) {
		t.Errorf("BoundCIDRs() = %v, want %v", got, want)
	}

	v4only := validNode()
	v4only.Addresses = v4only.Addresses[:1]

	if got := BoundCIDRs(v4only); !slices.Equal(got, want[:1]) {
		t.Errorf("BoundCIDRs(IPv4 only) = %v, want %v", got, want[:1])
	}
}

// Guards: G5
func TestCheckSameNode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		issuedTo string
		want     error
	}{
		{name: "the node that logged in", issuedTo: "nTEST1"},
		{name: "another node at the address", issuedTo: "nOTHER", want: ErrNodeChanged},
		{name: "a token that names no node", issuedTo: "", want: ErrNodeChanged},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := CheckSameNode(validNode(), tc.issuedTo); !errors.Is(err, tc.want) {
				t.Errorf("CheckSameNode() = %v, want %v", err, tc.want)
			}
		})
	}
}

// Guards: G3
func TestCheckRoleExcludesHost(t *testing.T) {
	t.Parallel()

	host := Host{StableID: self, Tags: []string{"tag:example-openbao", "tag:prod"}}

	tests := []struct {
		name  string
		bound []string
		want  error
	}{
		{name: "a tag the host lacks", bound: []string{"tag:example-api"}},
		{name: "the host's tag and one it lacks", bound: []string{"tag:prod", "tag:example-api"}},
		{name: "one of the host's tags", bound: []string{"tag:prod"}, want: ErrHostRole},
		{
			name:  "all of the host's tags",
			bound: []string{"tag:example-openbao", "tag:prod"},
			want:  ErrHostRole,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := CheckRoleExcludesHost(host, tc.bound); !errors.Is(err, tc.want) {
				t.Errorf("CheckRoleExcludesHost(%v) = %v, want %v", tc.bound, err, tc.want)
			}
		})
	}

	// An untagged host matches no role: a role binds at least one tag.
	if err := CheckRoleExcludesHost(Host{StableID: self}, []string{"tag:prod"}); err != nil {
		t.Errorf("an untagged host: %v", err)
	}
}

func TestCapTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ttl    time.Duration
		expiry time.Duration // from now; zero means the key does not expire
		want   time.Duration
	}{
		{name: "no key expiry", ttl: 30 * time.Minute, want: 30 * time.Minute},
		{
			name:   "key expires first",
			ttl:    30 * time.Minute,
			expiry: 10 * time.Minute,
			want:   10 * time.Minute,
		},
		{
			name:   "ttl ends first",
			ttl:    30 * time.Minute,
			expiry: 2 * time.Hour,
			want:   30 * time.Minute,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var keyExpiry time.Time
			if tc.expiry != 0 {
				keyExpiry = now.Add(tc.expiry)
			}

			if got := CapTTL(tc.ttl, keyExpiry, now); got != tc.want {
				t.Errorf("CapTTL() = %v, want %v", got, tc.want)
			}
		})
	}
}
