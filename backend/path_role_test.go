package backend

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/openbao/openbao/sdk/v2/logical"
)

func writeRole(
	t *testing.T,
	b *backend,
	s logical.Storage,
	name string,
	data map[string]any,
) *logical.Response {
	t.Helper()

	return request(t, b, s, logical.UpdateOperation, "role/"+name, data)
}

func TestRoleRoundTrip(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)

	resp := writeRole(t, b, s, "api", map[string]any{
		"bound_tags":     []string{"tag:example-api"},
		"token_ttl":      "30m",
		"token_max_ttl":  "4h",
		"token_policies": []string{"api"},
	})
	if resp.IsError() {
		t.Fatalf("write role: %v", resp.Error())
	}

	resp = request(t, b, s, logical.ReadOperation, "role/api", nil)
	if !slices.Equal(resp.Data["bound_tags"].([]string), []string{"tag:example-api"}) {
		t.Errorf("bound_tags = %v", resp.Data["bound_tags"])
	}

	if resp.Data["token_ttl"] != int64(1800) || resp.Data["token_max_ttl"] != int64(14400) {
		t.Errorf(
			"token_ttl, token_max_ttl = %v, %v",
			resp.Data["token_ttl"],
			resp.Data["token_max_ttl"],
		)
	}

	if !slices.Equal(resp.Data["token_policies"].([]string), []string{"api"}) {
		t.Errorf("token_policies = %v", resp.Data["token_policies"])
	}
}

func TestRoleUpdateKeepsUnnamedFields(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)
	writeRole(
		t,
		b,
		s,
		"api",
		map[string]any{"bound_tags": []string{"tag:example-api"}, "token_ttl": "30m"},
	)
	writeRole(t, b, s, "api", map[string]any{"token_max_ttl": "4h"})

	resp := request(t, b, s, logical.ReadOperation, "role/api", nil)
	if !slices.Equal(resp.Data["bound_tags"].([]string), []string{"tag:example-api"}) ||
		resp.Data["token_ttl"] != int64(1800) {
		t.Errorf("role after a partial update = %v", resp.Data)
	}
}

func TestRoleRequiresTags(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)

	for name, data := range map[string]map[string]any{
		"absent": {"token_ttl": "30m"},
		"empty":  {"bound_tags": []string{}},
	} {
		resp := writeRole(t, b, s, name, data)
		if !resp.IsError() || !strings.Contains(resp.Error().Error(), "bound_tags") {
			t.Errorf("%s bound_tags: response = %v, want an error naming bound_tags", name, resp)
		}
	}
}

func TestRoleRejectsNonTag(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)

	for _, tag := range []string{"example-api", "tag:", "tag:has space"} {
		resp := writeRole(t, b, s, "api", map[string]any{"bound_tags": []string{"tag:ok", tag}})
		if !resp.IsError() || !strings.Contains(resp.Error().Error(), tag) {
			t.Errorf("bound_tags entry %q: response = %v, want an error naming it", tag, resp)
		}
	}
}

func TestRoleDedupesTags(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)
	writeRole(t, b, s, "api", map[string]any{"bound_tags": []string{"tag:a", "tag:a", "tag:b"}})

	resp := request(t, b, s, logical.ReadOperation, "role/api", nil)
	if !slices.Equal(resp.Data["bound_tags"].([]string), []string{"tag:a", "tag:b"}) {
		t.Errorf("bound_tags = %v, want [tag:a tag:b]", resp.Data["bound_tags"])
	}
}

// Guards: G2
func TestRoleIgnoresBoundCIDRs(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)
	writeRole(t, b, s, "api", map[string]any{
		"bound_tags":             []string{"tag:example-api"},
		"token_bound_cidrs":      "10.0.0.0/8",
		"token_strictly_bind_ip": true,
	})

	role, err := b.role(t.Context(), s, "api")
	if err != nil || role == nil {
		t.Fatalf("load role: %v, %v", role, err)
	}

	if len(role.TokenBoundCIDRs) != 0 || role.TokenStrictlyBindIP {
		t.Errorf(
			"stored role has bound CIDRs %v, strict %v",
			role.TokenBoundCIDRs,
			role.TokenStrictlyBindIP,
		)
	}

	resp := request(t, b, s, logical.ReadOperation, "role/api", nil)
	for _, field := range []string{"token_bound_cidrs", "token_strictly_bind_ip"} {
		if _, ok := resp.Data[field]; ok {
			t.Errorf("a role read returns %s", field)
		}
	}
}

func TestRoleNameIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)
	writeRole(t, b, s, "API", map[string]any{"bound_tags": []string{"tag:example-api"}})

	if resp := request(t, b, s, logical.ReadOperation, "role/api", nil); resp == nil {
		t.Error("role/API is not found as role/api")
	}
}

func TestRoleList(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)
	writeRole(t, b, s, "worker", map[string]any{"bound_tags": []string{"tag:example-worker"}})
	writeRole(t, b, s, "api", map[string]any{"bound_tags": []string{"tag:example-api"}})

	resp := request(t, b, s, logical.ListOperation, "role/", nil)
	if !slices.Equal(resp.Data["keys"].([]string), []string{"api", "worker"}) {
		t.Errorf("roles = %v, want [api worker]", resp.Data["keys"])
	}
}

func TestRoleDelete(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)
	writeRole(t, b, s, "api", map[string]any{"bound_tags": []string{"tag:example-api"}})
	request(t, b, s, logical.DeleteOperation, "role/api", nil)

	if resp := request(t, b, s, logical.ReadOperation, "role/api", nil); resp != nil {
		t.Errorf("a deleted role still reads as %v", resp.Data)
	}
}

func TestRoleDefaultsToMountTTL(t *testing.T) {
	t.Parallel()

	b, s := testBackend(t, nil)
	writeRole(t, b, s, "api", map[string]any{"bound_tags": []string{"tag:example-api"}})

	role, err := b.role(t.Context(), s, "api")
	if err != nil || role == nil {
		t.Fatalf("load role: %v, %v", role, err)
	}

	if role.TokenTTL != time.Duration(0) {
		t.Errorf("TokenTTL = %v, want 0 for the mount's default", role.TokenTTL)
	}
}
