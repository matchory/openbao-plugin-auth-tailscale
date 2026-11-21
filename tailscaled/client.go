// Package tailscaled asks the local Tailscale daemon which node owns a
// tailnet address.
package tailscaled

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/tailcfg"

	"github.com/matchory/openbao-plugin-auth-tailscale/identity"
)

var (
	errNoNode = errors.New("tailscaled named no node")
	errNoSelf = errors.New("tailscaled named no node of its own")
)

type client struct {
	local *local.Client
}

// New returns a WhoIser that asks the tailscaled behind socket. An empty
// socket means the platform's default.
func New(socket string) identity.WhoIser {
	return &client{local: &local.Client{
		Socket: socket,
		// Without it the library also looks for a daemon by other means
		// than the socket it was given.
		UseSocketOnly: socket != "",
	}}
}

func (c *client) WhoIs(ctx context.Context, addr netip.Addr) (*identity.Node, error) {
	who, err := c.local.WhoIs(ctx, addr.String())
	if errors.Is(err, local.ErrPeerNotFound) {
		return nil, identity.ErrUnknownAddress
	}

	if err != nil {
		return nil, fmt.Errorf("whois %s: %w", addr, err)
	}

	if who == nil || who.Node == nil {
		return nil, fmt.Errorf("whois %s: %w", addr, errNoNode)
	}

	return convert(who.Node), nil
}

// Self asks tailscaled for its own state. A daemon that is logged out,
// stopped or still starting answers every WhoIs with "no such node", so its
// state is what tells that apart from a caller no tailnet knows.
func (c *client) Self(ctx context.Context) (string, error) {
	st, err := c.local.StatusWithoutPeers(ctx)
	if err != nil {
		return "", fmt.Errorf("status: %w", err)
	}

	if st.BackendState != ipn.Running.String() {
		return "", fmt.Errorf("tailscaled is %s, not %s", st.BackendState, ipn.Running)
	}

	if st.Self == nil || st.Self.ID == "" {
		return "", errNoSelf
	}

	return string(st.Self.ID), nil
}

func convert(n *tailcfg.Node) *identity.Node {
	addrs := make([]netip.Addr, 0, len(n.Addresses))
	for _, prefix := range n.Addresses {
		addrs = append(addrs, prefix.Addr())
	}

	return &identity.Node{
		StableID:   string(n.StableID),
		Name:       n.Name,
		Tags:       n.Tags,
		Addresses:  addrs,
		AllowedIPs: n.AllowedIPs,
		KeyExpiry:  n.KeyExpiry,
		Expired:    n.Expired,
		Shared:     n.Sharer != 0,
	}
}

// CheckTag reports why tag is not a Tailscale tag, by Tailscale's own rule.
func CheckTag(tag string) error {
	if err := tailcfg.CheckTag(tag); err != nil {
		return fmt.Errorf("not a Tailscale tag: %w", err)
	}

	return nil
}
