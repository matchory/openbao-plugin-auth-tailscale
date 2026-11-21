// Package backend is the OpenBao credential backend: it stores the method's
// configuration and roles, and connects a login to the rules in package
// identity.
package backend

import (
	"context"
	"fmt"
	"time"

	"github.com/openbao/openbao/sdk/v2/framework"
	"github.com/openbao/openbao/sdk/v2/logical"

	"github.com/matchory/openbao-plugin-auth-tailscale/identity"
	"github.com/matchory/openbao-plugin-auth-tailscale/tailscaled"
)

const backendHelp = `
The tailscale auth method logs a caller in by its Tailscale identity. The
caller names a role and sends no secret. The method asks the local tailscaled
which node owns the request's source address, and issues a token if that node
carries every tag the role binds. The token works only from that node's
addresses.
`

type backend struct {
	*framework.Backend

	// Both are replaced in tests.
	newWhoIser func(socket string) identity.WhoIser
	now        func() time.Time
}

// Factory returns the backend, set up with conf. OpenBao calls it once per
// mount.
func Factory(ctx context.Context, conf *logical.BackendConfig) (logical.Backend, error) {
	b := newBackend()
	if err := b.Setup(ctx, conf); err != nil {
		return nil, fmt.Errorf("set up the backend: %w", err)
	}

	return b, nil
}

func newBackend() *backend {
	b := &backend{
		newWhoIser: tailscaled.New,
		now:        time.Now,
	}

	b.Backend = &framework.Backend{
		Help:        backendHelp,
		BackendType: logical.TypeCredential,
		PathsSpecial: &logical.Paths{
			Unauthenticated: []string{"login"},
		},
		Paths: framework.PathAppend(
			b.configPaths(),
			b.rolePaths(),
			b.loginPaths(),
		),
	}

	return b
}
