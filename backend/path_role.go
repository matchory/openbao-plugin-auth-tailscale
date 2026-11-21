package backend

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/openbao/openbao/sdk/v2/framework"
	"github.com/openbao/openbao/sdk/v2/helper/tokenutil"
	"github.com/openbao/openbao/sdk/v2/logical"
)

const rolePrefix = "role/"

// tagPattern is Tailscale's form for a tag: "tag:", a letter, then letters,
// digits and dashes.
var tagPattern = regexp.MustCompile(`^tag:[a-zA-Z][a-zA-Z0-9-]*$`)

type roleEntry struct {
	tokenutil.TokenParams

	// BoundTags are the tags a node must carry, all of them.
	BoundTags []string `json:"bound_tags"`
}

// tokenFields are the token settings a role may carry. The two that bind a
// token to addresses are missing on purpose: the method binds every token to
// the addresses of the node that logged in.
func tokenFields() []string {
	return []string{
		"token_ttl", "token_max_ttl", "token_policies", "token_type", "token_period",
		"token_explicit_max_ttl", "token_no_default_policy", "token_num_uses",
	}
}

func (b *backend) rolePaths() []*framework.Path {
	fields := map[string]*framework.FieldSchema{
		"name": {
			Type:        framework.TypeLowerCaseString,
			Description: "Name of the role.",
		},
		"bound_tags": {
			Type:        framework.TypeCommaStringSlice,
			Description: "Tailscale tags a node must carry to log in with this role. It must carry all of them.",
		},
	}
	tokenutil.AddTokenFieldsWithAllowList(fields, tokenFields())

	return []*framework.Path{
		{
			Pattern: "role/?$",
			Operations: map[logical.Operation]framework.OperationHandler{
				logical.ListOperation: &framework.PathOperation{
					Callback: b.listRoles,
					Summary:  "List the roles.",
				},
			},
			HelpSynopsis: "List the roles.",
		},
		{
			Pattern:        "role/" + framework.GenericNameRegex("name"),
			Fields:         fields,
			ExistenceCheck: b.roleExists,
			Operations: map[logical.Operation]framework.OperationHandler{
				logical.ReadOperation: &framework.PathOperation{
					Callback: b.readRole,
					Summary:  "Read a role.",
				},
				logical.CreateOperation: &framework.PathOperation{
					Callback: b.writeRole,
					Summary:  "Create a role.",
				},
				logical.UpdateOperation: &framework.PathOperation{
					Callback: b.writeRole,
					Summary:  "Update a role.",
				},
				logical.DeleteOperation: &framework.PathOperation{
					Callback: b.deleteRole,
					Summary:  "Delete a role.",
				},
			},
			HelpSynopsis: "Bind Tailscale tags to token settings.",
		},
	}
}

// role returns the named role, or nil when there is none.
func (b *backend) role(ctx context.Context, s logical.Storage, name string) (*roleEntry, error) {
	entry, err := s.Get(ctx, rolePrefix+strings.ToLower(name))
	if err != nil {
		return nil, fmt.Errorf("read role %q: %w", name, err)
	}

	if entry == nil {
		return nil, nil //nolint:nilnil // no role is not an error
	}

	role := &roleEntry{}
	if err := entry.DecodeJSON(role); err != nil {
		return nil, fmt.Errorf("decode role %q: %w", name, err)
	}

	// Whatever storage holds, a role never decides which addresses a token
	// is bound to.
	role.TokenBoundCIDRs = nil
	role.TokenStrictlyBindIP = false

	return role, nil
}

func (b *backend) roleExists(
	ctx context.Context,
	req *logical.Request,
	d *framework.FieldData,
) (bool, error) {
	role, err := b.role(ctx, req.Storage, roleName(d))

	return role != nil, err
}

func (b *backend) listRoles(ctx context.Context, req *logical.Request, _ *framework.FieldData) (
	*logical.Response, error,
) {
	names, err := req.Storage.List(ctx, rolePrefix)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}

	slices.Sort(names)

	return logical.ListResponse(names), nil
}

func (b *backend) readRole(ctx context.Context, req *logical.Request, d *framework.FieldData) (
	*logical.Response, error,
) {
	role, err := b.role(ctx, req.Storage, roleName(d))
	if err != nil || role == nil {
		return nil, err
	}

	data := map[string]any{"bound_tags": role.BoundTags}
	role.PopulateTokenData(data)
	delete(data, "token_bound_cidrs")
	delete(data, "token_strictly_bind_ip")

	return &logical.Response{Data: data}, nil
}

func (b *backend) writeRole(ctx context.Context, req *logical.Request, d *framework.FieldData) (
	*logical.Response, error,
) {
	name := roleName(d)

	role, err := b.role(ctx, req.Storage, name)
	if err != nil {
		return nil, err
	}

	if role == nil {
		role = &roleEntry{}
	}

	if raw, ok := d.GetOk("bound_tags"); ok {
		tags, _ := raw.([]string)
		role.BoundTags = dedupe(tags)
	}

	if len(role.BoundTags) == 0 {
		return logical.ErrorResponse(
			"bound_tags is required: a role must bind at least one tag",
		), nil
	}

	for _, tag := range role.BoundTags {
		if !tagPattern.MatchString(tag) {
			return logical.ErrorResponse(
				"bound_tags: %q is not a Tailscale tag (tag:<name>)",
				tag,
			), nil
		}
	}

	if err := role.ParseTokenFields(req, d); err != nil {
		return logical.ErrorResponse(err.Error()), nil
	}

	entry, err := logical.StorageEntryJSON(rolePrefix+name, role)
	if err != nil {
		return nil, fmt.Errorf("encode role %q: %w", name, err)
	}

	if err := req.Storage.Put(ctx, entry); err != nil {
		return nil, fmt.Errorf("store role %q: %w", name, err)
	}

	return nil, nil //nolint:nilnil // an empty response is how a write reports success
}

func (b *backend) deleteRole(ctx context.Context, req *logical.Request, d *framework.FieldData) (
	*logical.Response, error,
) {
	name := roleName(d)
	if err := req.Storage.Delete(ctx, rolePrefix+name); err != nil {
		return nil, fmt.Errorf("delete role %q: %w", name, err)
	}

	return nil, nil //nolint:nilnil // an empty response is how a delete reports success
}

// roleName returns the role a request names. The schema lower-cases it.
func roleName(d *framework.FieldData) string {
	name, _ := d.Get("name").(string)

	return name
}

// dedupe returns tags without repeats, in their first order.
func dedupe(tags []string) []string {
	unique := make([]string, 0, len(tags))
	for _, tag := range tags {
		if !slices.Contains(unique, tag) {
			unique = append(unique, tag)
		}
	}

	return unique
}
