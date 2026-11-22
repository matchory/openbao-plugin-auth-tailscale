package tailscaled

import (
	"encoding/json"
	"net/netip"
	"slices"
	"testing"
	"time"

	"tailscale.com/tailcfg"

	"github.com/matchory/openbao-plugin-auth-tailscale/identity"
)

// FuzzWhoIsAnswer feeds what tailscaled could answer through the conversion
// and the identity rules, and checks every accepted node against the answer
// itself: a field the conversion drops or misreads would let a node in.
//
// Guards: G1, G3
func FuzzWhoIsAnswer(f *testing.F) {
	const self = "nSELF"

	remote := netip.MustParseAddr("100.64.0.5")
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	for _, seed := range []string{
		`{"StableID":"nTEST1","Tags":["tag:example-api"],"Addresses":["100.64.0.5/32"],` +
			`"AllowedIPs":["100.64.0.5/32"]}`,
		`{"StableID":"nTEST1","Tags":["tag:example-api"],"Addresses":["100.64.0.5/32"],` +
			`"AllowedIPs":["100.64.0.5/32"],"Sharer":7}`,
		`{"StableID":"nTEST1","Tags":["tag:example-api"],"Addresses":["100.64.0.5/32"],` +
			`"AllowedIPs":["100.64.0.5/32","10.0.0.0/24"]}`,
		`{"StableID":"nTEST1","Tags":["tag:example-api"],"Addresses":["100.64.0.5/32"],` +
			`"AllowedIPs":["100.64.0.5/32"],"KeyExpiry":"2026-01-15T11:00:00Z"}`,
		`{"StableID":"nTEST1","Tags":["tag:example-api"],"Addresses":["100.64.0.5/32"],` +
			`"AllowedIPs":["100.64.0.5/32"],"Expired":true}`,
		`{"StableID":"nSELF","Tags":["tag:example-api"],"Addresses":["100.64.0.5/32"],` +
			`"AllowedIPs":["100.64.0.5/32"]}`,
		`{"StableID":"nTEST1","Tags":["tag:example-api"],"Addresses":["100.64.0.5/32"]}`,
		`{"StableID":"nTEST1","Tags":["tag:example-api"],"Addresses":["100.64.0.5/32"],` +
			`"AllowedIPs":["100.64.0.5/32"],"Hostinfo":{"RoutableIPs":["10.0.0.0/24"]}}`,
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, answer []byte) {
		var raw tailcfg.Node
		if json.Unmarshal(answer, &raw) != nil {
			return
		}

		node := convert(&raw)
		if identity.CheckNode(node, self, remote, now) != nil ||
			identity.MatchTags(node, []string{"tag:example-api"}) != nil {
			return
		}

		switch {
		case !slices.Contains(raw.Tags, "tag:example-api"):
			t.Errorf("accepted a node tagged %v", raw.Tags)
		case raw.Sharer != 0:
			t.Error("accepted a shared-in node")
		case raw.Expired, !raw.KeyExpiry.IsZero() && !raw.KeyExpiry.After(now):
			t.Errorf("accepted an expired node: %v, %v", raw.Expired, raw.KeyExpiry)
		case string(raw.StableID) == self:
			t.Error("accepted the node tailscaled runs as")
		case !slices.ContainsFunc(raw.Addresses, func(p netip.Prefix) bool { return p.Addr() == remote }):
			t.Errorf("accepted %v for a node whose addresses are %v", remote, raw.Addresses)
		case !slices.Contains(raw.AllowedIPs, netip.PrefixFrom(remote, remote.BitLen())):
			t.Errorf("accepted %v for a node that may send from %v", remote, raw.AllowedIPs)
		}

		var advertised []netip.Prefix
		if raw.Hostinfo.Valid() {
			advertised = raw.Hostinfo.RoutableIPs().AsSlice()
		}

		for _, p := range slices.Concat(raw.AllowedIPs, advertised) {
			if p.Bits() != p.Addr().BitLen() {
				t.Errorf("accepted a node that routes %v", p)
			}
		}
	})
}
