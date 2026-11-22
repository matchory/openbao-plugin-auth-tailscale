package identity

import (
	"net/netip"
	"testing"
	"time"
)

// Guards: G1
func FuzzParseRemoteAddr(f *testing.F) {
	for _, seed := range []string{
		"100.64.0.5", "fd7a:115c:a1e0::5", "::ffff:100.64.0.5", "100.64.0.5:443",
		"fe80::1%eth0", "", "203.0.113.7", "not-an-ip",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, s string) {
		addr, err := ParseRemoteAddr(s)
		if err != nil {
			return
		}

		if !isTailnet(addr) || addr.Zone() != "" || addr.Is4In6() {
			t.Errorf("ParseRemoteAddr(%q) accepted %v", s, addr)
		}
	})
}

// Guards: G1, G3
func FuzzCheckNode(f *testing.F) {
	f.Add("100.64.0.5", "nTEST1", "100.64.0.5", "100.64.0.5/32", false, false, int64(0))
	f.Add("100.64.0.5", "nTEST1", "100.64.0.9", "100.64.0.5/32", false, false, int64(0))
	f.Add("fd7a:115c:a1e0::5", "nTEST1", "fd7a:115c:a1e0::5", "::/0", false, false, int64(0))
	f.Add("100.64.0.5", "nTEST1", "100.64.0.5", "10.0.0.0/24", false, false, int64(0))
	f.Add("100.64.0.5", self, "100.64.0.5", "100.64.0.5/32", false, false, int64(0))
	f.Add("100.64.0.5", "nTEST1", "100.64.0.5", "100.64.0.5/32", true, false, int64(0))
	f.Add("100.64.0.5", "nTEST1", "100.64.0.5", "100.64.0.5/32", false, true, int64(0))
	f.Add("100.64.0.5", "nTEST1", "100.64.0.5", "100.64.0.5/32", false, false, int64(-1))

	f.Fuzz(func(
		t *testing.T,
		remote, stableID, own, allowed string,
		shared, expired bool,
		keyExpiry int64,
	) {
		remoteAddr, err := netip.ParseAddr(remote)
		if err != nil {
			return
		}

		node := &Node{
			StableID: stableID,
			Tags:     []string{"tag:example-api"},
			Shared:   shared,
			Expired:  expired,
		}
		// Bounded, so the duration cannot overflow into the other sign.
		keyExpiry %= 1 << 30
		if keyExpiry != 0 {
			node.KeyExpiry = now.Add(time.Duration(keyExpiry) * time.Second)
		}

		if addr, err := netip.ParseAddr(own); err == nil {
			node.Addresses = []netip.Addr{addr}
		}

		if prefix, err := netip.ParsePrefix(allowed); err == nil {
			node.AllowedIPs = []netip.Prefix{prefix}
		}

		if CheckNode(node, self, remoteAddr, now) != nil {
			return
		}

		// What an accepted node must be, stated without the code's own
		// conditions.
		switch {
		case stableID == self:
			t.Error("accepted the node tailscaled runs as")
		case shared, expired, keyExpiry < 0:
			t.Errorf("accepted a node that is shared %v, expired %v, key expiry %v",
				shared, expired, node.KeyExpiry)
		case len(node.Addresses) != 1 || node.Addresses[0] != remoteAddr:
			t.Errorf("accepted %v for a node whose addresses are %v", remoteAddr, node.Addresses)
		case len(node.AllowedIPs) != 1 || node.AllowedIPs[0].Addr() != remoteAddr ||
			node.AllowedIPs[0].Bits() != remoteAddr.BitLen():
			// With one allowed prefix, it can only be the remote address
			// alone: anything wider routes for others.
			t.Errorf("accepted %v for a node that may send from %v", remoteAddr, node.AllowedIPs)
		}
	})
}
