package backend

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/hashicorp/go-sockaddr"
	"github.com/openbao/openbao/sdk/v2/framework"
	"github.com/openbao/openbao/sdk/v2/logical"

	"github.com/matchory/openbao-plugin-auth-tailscale/identity"
)

// Keys of the request field, the token metadata and the token's internal
// data.
const (
	keyRole   = "role"
	keyNodeID = "node_id"
)

var (
	errNoConnection = errors.New("request carries no connection information")
	errNoRole       = errors.New("no such role")
)

func (b *backend) loginPaths() []*framework.Path {
	return []*framework.Path{{
		Pattern: "login$",
		Fields: map[string]*framework.FieldSchema{
			keyRole: {
				Type:        framework.TypeLowerCaseString,
				Description: "Name of the role to log in with.",
			},
		},
		Operations: map[logical.Operation]framework.OperationHandler{
			logical.UpdateOperation: &framework.PathOperation{
				Callback: b.login,
				Summary:  "Log in as the tailnet node the request comes from.",
			},
		},
		HelpSynopsis: "Log in with a role, as the tailnet node the request comes from.",
	}}
}

// attestation is what a successful check of a request established.
type attestation struct {
	node   *identity.Node
	role   *roleEntry
	remote netip.Addr
}

// attest decides whether the node a request comes from may use the named
// role. Login and renewal both go through it, so a renewal is held to
// everything a login is. Any error is a refusal.
func (b *backend) attest(
	ctx context.Context,
	req *logical.Request,
	roleName string,
) (*attestation, error) {
	if req.Connection == nil {
		return nil, errNoConnection
	}

	remote, err := identity.ParseRemoteAddr(req.Connection.RemoteAddr)
	if err != nil {
		return nil, fmt.Errorf("source address %q: %w", req.Connection.RemoteAddr, err)
	}

	cfg, err := b.config(ctx, req.Storage)
	if err != nil {
		return nil, err
	}

	whoIsCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// tailscaled is asked before the role is looked up: a refusal then
	// takes as long for a role that exists as for one that does not.
	node, err := b.newWhoIser(cfg.Socket).WhoIs(whoIsCtx, remote)
	if err != nil {
		return nil, fmt.Errorf("ask tailscaled: %w", err)
	}

	if err := identity.CheckNode(node, remote, b.now()); err != nil {
		return nil, fmt.Errorf("node at %s: %w", remote, err)
	}

	role, err := b.role(ctx, req.Storage, roleName)
	if err != nil {
		return nil, err
	}

	if role == nil {
		return nil, fmt.Errorf("role %q: %w", roleName, errNoRole)
	}

	if err := identity.MatchTags(node, role.BoundTags); err != nil {
		return nil, fmt.Errorf("node %s, role %q: %w", node.StableID, roleName, err)
	}

	return &attestation{node: node, role: role, remote: remote}, nil
}

func (b *backend) login(ctx context.Context, req *logical.Request, d *framework.FieldData) (
	*logical.Response, error,
) {
	roleName, _ := d.Get(keyRole).(string)

	att, err := b.attest(ctx, req, roleName)
	if err != nil {
		return nil, b.refuse(req, roleName, err)
	}

	tags := slices.Clone(att.node.Tags)
	slices.Sort(tags)

	auth := &logical.Auth{
		DisplayName: roleName,
		// One entity per role: every new node of a workload would
		// otherwise become an entity of its own.
		Alias: &logical.Alias{Name: roleName},
		InternalData: map[string]any{
			keyRole:   roleName,
			keyNodeID: att.node.StableID,
		},
		Metadata: map[string]string{
			keyRole:     roleName,
			keyNodeID:   att.node.StableID,
			"node_name": att.node.Name,
			"tags":      strings.Join(tags, ","),
			"address":   att.remote.String(),
		},
	}

	if err := att.role.PopulateTokenAuth(auth, req); err != nil {
		return nil, b.refuse(req, roleName, err)
	}

	b.bindToNode(auth, att)

	return &logical.Response{Auth: auth}, nil
}

// bindToNode sets what a token takes from the node and not from the role:
// the addresses it works from, and lifetimes that end with the node key.
func (b *backend) bindToNode(auth *logical.Auth, att *attestation) {
	cidrs := identity.BoundCIDRs(att.node)

	auth.BoundCIDRs = make([]*sockaddr.SockAddrMarshaler, 0, len(cidrs))
	for _, cidr := range cidrs {
		auth.BoundCIDRs = append(auth.BoundCIDRs, &sockaddr.SockAddrMarshaler{
			SockAddr: sockaddr.MustIPAddr(cidr.String()),
		})
	}

	if att.node.KeyExpiry.IsZero() {
		return
	}

	// A zero TTL means the mount's default, which the key may not outlast
	// either.
	if auth.TTL == 0 {
		auth.TTL = b.System().DefaultLeaseTTL()
	}

	if auth.MaxTTL == 0 {
		auth.MaxTTL = b.System().MaxLeaseTTL()
	}

	now := b.now()
	auth.TTL = identity.CapTTL(auth.TTL, att.node, now)
	auth.MaxTTL = identity.CapTTL(auth.MaxTTL, att.node, now)

	if auth.Period != 0 {
		auth.Period = identity.CapTTL(auth.Period, att.node, now)
	}
}

// refuse logs why a request was refused and returns the one answer every
// refused caller gets.
func (b *backend) refuse(req *logical.Request, roleName string, reason error) error {
	remote := ""
	if req.Connection != nil {
		remote = req.Connection.RemoteAddr
	}

	b.Logger().Debug("refused", "role", roleName, "remote_addr", remote, "reason", reason)

	return logical.ErrPermissionDenied
}
