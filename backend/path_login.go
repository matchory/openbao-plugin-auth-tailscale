package backend

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/go-sockaddr"
	"github.com/openbao/openbao/sdk/v2/framework"
	"github.com/openbao/openbao/sdk/v2/helper/policyutil"
	"github.com/openbao/openbao/sdk/v2/logical"

	"github.com/matchory/openbao-plugin-auth-tailscale/identity"
)

// Keys of the request field, the token metadata and the token's internal
// data.
const (
	keyRole   = "role"
	keyNodeID = "node_id"
)

// The backend's own reasons to refuse, beside those of package identity.
const (
	errNoConnection identity.Refusal = "request carries no connection information"
	errNoRole       identity.Refusal = "no such role"
	errNodeChanged  identity.Refusal = "address now belongs to another node"
	errRoleChanged  identity.Refusal = "role no longer grants the token's policies"
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
	// at is the one moment every time-dependent check and lifetime of
	// this request is measured from.
	at time.Time
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
	node, err := b.whoIser(cfg.Socket).WhoIs(whoIsCtx, remote)
	if err != nil {
		return nil, fmt.Errorf("ask tailscaled: %w", err)
	}

	now := b.now()
	if err := identity.CheckNode(node, remote, now); err != nil {
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

	return &attestation{node: node, role: role, remote: remote, at: now}, nil
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

	// The addresses come from the node, never from the role.
	cidrs := identity.BoundCIDRs(att.node)

	auth.BoundCIDRs = make([]*sockaddr.SockAddrMarshaler, 0, len(cidrs))
	for _, cidr := range cidrs {
		auth.BoundCIDRs = append(auth.BoundCIDRs, &sockaddr.SockAddrMarshaler{
			SockAddr: sockaddr.MustIPAddr(cidr.String()),
		})
	}

	b.capLifetimes(auth, att, att.at)

	return &logical.Response{Auth: auth}, nil
}

// capLifetimes shortens a token's lifetimes so it ends no later than the
// node key. OpenBao counts the maximum from when the token was issued and the
// others from now.
func (b *backend) capLifetimes(auth *logical.Auth, att *attestation, issued time.Time) {
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

	auth.TTL = identity.CapTTL(auth.TTL, att.node, att.at)
	auth.MaxTTL = identity.CapTTL(auth.MaxTTL, att.node, issued)

	if auth.Period != 0 {
		auth.Period = identity.CapTTL(auth.Period, att.node, att.at)
	}
}

// refuse logs why a request was refused and returns the one answer every
// refused caller gets.
func (b *backend) refuse(req *logical.Request, roleName string, reason error) error {
	remote := ""
	if req.Connection != nil {
		remote = req.Connection.RemoteAddr
	}

	// Who was refused for what is for debugging. That tailscaled or storage
	// is failing is something an operator must see without asking.
	log := b.Logger().Warn
	if errors.As(reason, new(identity.Refusal)) {
		log = b.Logger().Debug
	}

	log("refused", "role", roleName, "remote_addr", remote, "reason", reason)

	return logical.ErrPermissionDenied
}

// renew extends a token only if a login from the same address would succeed
// now, would name the node that logged in, and would grant the same policies. A tailnet address can pass to
// another node, and a role or a node's tags can change, while a token lives.
func (b *backend) renew(ctx context.Context, req *logical.Request, _ *framework.FieldData) (
	*logical.Response, error,
) {
	roleName, _ := req.Auth.InternalData[keyRole].(string)
	nodeID, _ := req.Auth.InternalData[keyNodeID].(string)

	att, err := b.attest(ctx, req, roleName)
	if err != nil {
		return nil, b.refuse(req, roleName, err)
	}

	if nodeID == "" || att.node.StableID != nodeID {
		return nil, b.refuse(req, roleName, errNodeChanged)
	}

	// A token keeps the policies it was issued with, so one whose role
	// has since been given other policies must log in again.
	if !policyutil.EquivalentPolicies(att.role.TokenPolicies, req.Auth.TokenPolicies) {
		return nil, b.refuse(req, roleName, errRoleChanged)
	}

	auth := req.Auth
	auth.TTL = att.role.TokenTTL
	auth.MaxTTL = att.role.TokenMaxTTL
	auth.Period = att.role.TokenPeriod

	issued := auth.IssueTime
	if issued.IsZero() {
		issued = att.at
	}

	b.capLifetimes(auth, att, issued)

	return &logical.Response{Auth: auth}, nil
}
