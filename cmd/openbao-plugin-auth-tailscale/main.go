// Command openbao-plugin-auth-tailscale is an OpenBao auth plugin that logs
// callers in by their Tailscale identity. OpenBao starts it; it is not run by
// hand.
package main

import (
	"log"

	"github.com/openbao/openbao/sdk/v2/plugin"

	"github.com/matchory/openbao-plugin-auth-tailscale/backend"
)

func main() {
	err := plugin.ServeMultiplex(&plugin.ServeOpts{BackendFactoryFunc: backend.Factory})
	if err != nil {
		log.Fatalf("serve the plugin: %v", err)
	}
}
