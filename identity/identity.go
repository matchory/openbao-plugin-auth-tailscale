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

// A Refusal is a decision that a caller may not log in. Any other error on
// the way to that decision is a failure to decide: it refuses the caller
// just the same, but it is the operator's problem, not the caller's.
type Refusal string

func (r Refusal) Error() string { return string(r) }

// The reasons a login is refused. Callers of this package log them; the
// caller of the login never sees which one applied.
const (
	ErrNotTailnet      Refusal = "address is not a Tailscale address"
	ErrUnknownAddress  Refusal = "no node has this address"
	ErrUntagged        Refusal = "node has no tags"
	ErrShared          Refusal = "node is shared in from another tailnet"
	ErrExpired         Refusal = "node key has expired"
	ErrSelf            Refusal = "node is the one tailscaled runs as"
	ErrAddressMismatch Refusal = "address is not one of the node's own"
	ErrRoutes          Refusal = "node routes for other addresses"
	ErrNotAllowed      Refusal = "node may not send from this address"
	ErrTagMismatch     Refusal = "node lacks a bound tag"
)

var errNoNode = errors.New("no node")

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
	// Tags is empty for a node that belongs to a person.
	Tags []string
	// Addresses are the node's own tailnet addresses.
	Addresses []netip.Addr
	// AllowedIPs is everything the node may send from: its own addresses,
	// the Service addresses it hosts, and any routes it advertises.
	AllowedIPs []netip.Prefix
	// KeyExpiry is the zero time when the node key does not expire.
	KeyExpiry time.Time
	Expired   bool
	// Shared is true for a node shared in from another tailnet.
	Shared bool
}

// WhoIser finds the node that owns a tailnet address.
type WhoIser interface {
	// WhoIs returns the node that owns addr, ErrUnknownAddress when no
	// node does, or another error when it cannot tell. It never returns a
	// nil node without an error.
	WhoIs(ctx context.Context, addr netip.Addr) (*Node, error)
	// Self returns the stable ID of the node tailscaled itself runs as. It
	// returns an error when tailscaled is not connected to its tailnet,
	// and never an empty ID without one.
	Self(ctx context.Context) (string, error)
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

// CheckNode reports why the node behind remote may not log in, or nil. self
// is the stable ID of the node tailscaled runs as.
func CheckNode(n *Node, self string, remote netip.Addr, now time.Time) error {
	switch {
	case n == nil:
		return errNoNode
	// Any local process on the OpenBao host reaches OpenBao from the host's
	// own address, so it would log in as the host.
	case n.StableID == self:
		return ErrSelf
	case len(n.Tags) == 0:
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

	// A node is accepted only for an address tailscaled says it may send
	// from, so a missing or empty list refuses rather than passes.
	if !slices.Contains(n.AllowedIPs, netip.PrefixFrom(remote, remote.BitLen())) {
		return ErrNotAllowed
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

// CapTTL returns ttl, or the time the node key has left counted from start
// if that is shorter. A key that does not expire caps nothing.
func CapTTL(ttl time.Duration, n *Node, start time.Time) time.Duration {
	if n.KeyExpiry.IsZero() {
		return ttl
	}

	return min(ttl, n.KeyExpiry.Sub(start))
}

func isTailnet(addr netip.Addr) bool {
	return tailnetV4.Contains(addr) || tailnetV6.Contains(addr)
}
