package identity

import (
	"net/netip"
	"testing"
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
	f.Add("100.64.0.5", "100.64.0.5", "100.64.0.5/32")
	f.Add("100.64.0.5", "100.64.0.9", "100.64.0.5/32")
	f.Add("fd7a:115c:a1e0::5", "fd7a:115c:a1e0::5", "::/0")
	f.Add("100.64.0.5", "100.64.0.5", "10.0.0.0/24")

	f.Fuzz(func(t *testing.T, remote, own, allowed string) {
		remoteAddr, err := netip.ParseAddr(remote)
		if err != nil {
			return
		}

		node := &Node{Tags: []string{"tag:example-api"}}
		if addr, err := netip.ParseAddr(own); err == nil {
			node.Addresses = []netip.Addr{addr}
		}

		if prefix, err := netip.ParsePrefix(allowed); err == nil {
			node.AllowedIPs = []netip.Prefix{prefix}
		}

		if CheckNode(node, remoteAddr, now) != nil {
			return
		}

		if len(node.Addresses) != 1 || node.Addresses[0] != remoteAddr {
			t.Errorf("accepted %v for a node whose addresses are %v", remoteAddr, node.Addresses)
		}

		for _, p := range node.AllowedIPs {
			if !p.IsSingleIP() || !isTailnet(p.Addr()) {
				t.Errorf("accepted a node that may send from %v", p)
			}
		}
	})
}
