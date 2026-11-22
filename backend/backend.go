// Package backend is the OpenBao credential backend: it stores the method's
// configuration and roles, and connects a login to the rules in package
// identity.
package backend

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
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

// version is the release this binary was built as, such as "v0.1.0". The
// release build sets it; it is empty in any other build.
var version string

type backend struct {
	*framework.Backend

	// Both are replaced in tests.
	newWhoIser func(socket string) whoIser
	now        func() time.Time

	// The client for tailscaled is kept: each one holds a connection open,
	// in this process and in tailscaled, for as long as it exists.
	clientMu     sync.Mutex
	client       whoIser
	clientSocket string
}

// whoIser is what the backend asks tailscaled; *tailscaled.Client is the
// one it uses outside tests. See there for what each method promises.
type whoIser interface {
	WhoIs(ctx context.Context, addr netip.Addr) (*identity.Node, error)
	Self(ctx context.Context) (string, error)
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
		newWhoIser: func(socket string) whoIser { return tailscaled.New(socket) },
		now:        time.Now,
	}

	b.Backend = &framework.Backend{
		Help:           backendHelp,
		BackendType:    logical.TypeCredential,
		RunningVersion: version,
		AuthRenew:      b.renew,
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

// whoIser returns the client for the tailscaled behind socket, making a new
// one only when the configured socket has changed.
func (b *backend) whoIser(socket string) whoIser {
	b.clientMu.Lock()
	defer b.clientMu.Unlock()

	if b.client == nil || b.clientSocket != socket {
		b.client = b.newWhoIser(socket)
		b.clientSocket = socket
	}

	return b.client
}
