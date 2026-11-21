// Package identity holds the rules that decide whether a tailnet node may log
// in: which source addresses count, which nodes are refused, and how a node's
// tags match a role. It imports only the standard library, so the rules can
// be read and tested without OpenBao or Tailscale.
package identity

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"time"
)

// The reasons a login is refused. Callers of this package log them; the
// caller of the login never sees which one applied.
var (
	ErrNotTailnet      = errors.New("address is not a Tailscale address")
	ErrUntagged        = errors.New("node has no tags")
	ErrShared          = errors.New("node is shared in from another tailnet")
	ErrExpired         = errors.New("node key has expired")
	ErrAddressMismatch = errors.New("address is not one of the node's own")
	ErrRoutes          = errors.New("node routes for other addresses")
	ErrTagMismatch     = errors.New("node lacks a bound tag")

	errNoNode = errors.New("no node")
)

// The ranges Tailscale assigns node and Service addresses from.
var (
	tailnetV4 = netip.MustParsePrefix("100.64.0.0/10")
	tailnetV6 = netip.MustParsePrefix("fd7a:115c:a1e0::/48")
)

// Node is what the local tailscaled says about the owner of an address.
type Node struct {
	// StableID identifies the node for as long as it exists. A tailnet
	// address can pass to another node; a stable ID cannot.
	StableID string
	Name     string
	Tags     []string
	// Addresses are the node's own tailnet addresses.
	Addresses []netip.Addr
	// AllowedIPs is everything the node may send from: its own addresses,
	// the Service addresses it hosts, and any routes it advertises.
	AllowedIPs []netip.Prefix
	// KeyExpiry is the zero time when the node key does not expire.
	KeyExpiry time.Time
	Expired   bool
	Tagged    bool
	// Shared is true for a node shared in from another tailnet.
	Shared bool
}

// WhoIser finds the node that owns a tailnet address.
type WhoIser interface {
	// WhoIs returns the node that owns addr, or an error. It never returns
	// a nil node without an error.
	WhoIs(ctx context.Context, addr netip.Addr) (*Node, error)
}

// ParseRemoteAddr parses the source address of a request. It returns
// ErrNotTailnet for anything but a bare address inside Tailscale's ranges.
func ParseRemoteAddr(s string) (netip.Addr, error) {
	addr, err := netip.ParseAddr(s)
	if err != nil || addr.Zone() != "" {
		return netip.Addr{}, ErrNotTailnet
	}

	// A dual-stack listener reports an IPv4 peer in IPv4-mapped form.
	addr = addr.Unmap()
	if !isTailnet(addr) {
		return netip.Addr{}, ErrNotTailnet
	}

	return addr, nil
}

// CheckNode reports why the node behind remote may not log in, or nil.
func CheckNode(n *Node, remote netip.Addr, now time.Time) error {
	switch {
	case n == nil:
		return errNoNode
	case !n.Tagged:
		return ErrUntagged
	case n.Shared:
		return ErrShared
	case n.Expired, !n.KeyExpiry.IsZero() && !n.KeyExpiry.After(now):
		return ErrExpired
	case !slices.Contains(n.Addresses, remote):
		return ErrAddressMismatch
	}

	// Traffic a subnet router or exit node forwards arrives from the
	// router's own address, so its clients would log in as the router.
	for _, p := range n.AllowedIPs {
		if !p.IsSingleIP() || !isTailnet(p.Addr()) {
			return ErrRoutes
		}
	}

	return nil
}

// MatchTags returns ErrTagMismatch unless the node carries every bound tag.
// A role that binds no tags matches no node.
func MatchTags(n *Node, bound []string) error {
	if len(bound) == 0 {
		return ErrTagMismatch
	}

	for _, tag := range bound {
		if !slices.Contains(n.Tags, tag) {
			return ErrTagMismatch
		}
	}

	return nil
}

// BoundCIDRs returns one single-address prefix per address of the node, for
// binding a token to the node in both address families.
func BoundCIDRs(n *Node) []netip.Prefix {
	cidrs := make([]netip.Prefix, 0, len(n.Addresses))
	for _, addr := range n.Addresses {
		cidrs = append(cidrs, netip.PrefixFrom(addr, addr.BitLen()))
	}

	return cidrs
}

// CapTTL shortens ttl so a token does not outlive the node key. A ttl of
// zero means "the mount's default" and is returned unchanged when the key
// does not expire.
func CapTTL(ttl time.Duration, n *Node, now time.Time) time.Duration {
	if n.KeyExpiry.IsZero() {
		return ttl
	}

	left := n.KeyExpiry.Sub(now)
	if ttl == 0 || left < ttl {
		return left
	}

	return ttl
}

func isTailnet(addr netip.Addr) bool {
	return tailnetV4.Contains(addr) || tailnetV6.Contains(addr)
}
