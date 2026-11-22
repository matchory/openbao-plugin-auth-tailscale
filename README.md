# openbao-plugin-auth-tailscale

An [OpenBao](https://openbao.org) auth method that logs a caller in by its [Tailscale](https://tailscale.com)
identity. The caller sends a role name and no secret. The plugin asks the local `tailscaled` which node owns the
request's source address, and issues a token if that node carries the role's tags. The token works only from
that node's addresses.

Use it for services on your tailnet that need secrets, certificates or database credentials from OpenBao and
have nowhere safe to keep a credential of their own.

## Quick start

On the OpenBao host, with the plugin binary in OpenBao's `plugin_directory`:

```bash
bao plugin register -sha256="$(sha256sum /etc/openbao/plugins/openbao-plugin-auth-tailscale | cut -d' ' -f1)" \
  auth openbao-plugin-auth-tailscale
bao auth enable -path=tailscale openbao-plugin-auth-tailscale
bao write auth/tailscale/role/api bound_tags=tag:example-api token_policies=api token_ttl=30m
```

From a node tagged `tag:example-api`:

```bash
curl -s -X POST -d '{"role":"api"}' https://openbao.example.ts.net/v1/auth/tailscale/login
```

The answer carries a token in `.auth.client_token`. [Getting started](docs/getting-started.md) walks through the
whole setup, including the part the quick start skips: making sure OpenBao sees the caller's real address.

## What it refuses

A login is refused when the source address is outside Tailscale's ranges, when `tailscaled` doesn't answer in
time, and when the node behind the address:

- has no tags (a person's device),
- is shared in from another tailnet,
- has an expired key,
- is a subnet router or an exit node,
- lacks one of the tags the role binds,
- lacks one of the app capabilities the role binds, if it binds any.

Every refusal is the same `permission denied`. The reason is in the plugin's debug log.

## Before you deploy it

The source address is the proof of identity, so OpenBao must see the caller's real one, and nothing but
`tailscaled` may be able to claim another. Read
[Deploy behind tailscale serve](docs/guides/deploy-behind-tailscale-serve.md) and the
[security model](SECURITY.md#security-model) first.

Identity is per node and per tag. Two programs on one node are one identity, and whoever can give a node a tag
can become that tag. [Limitations](docs/explanation/limitations.md) lists the rest.

## Documentation

- [Getting started](docs/getting-started.md)
- Guides: [deploy behind tailscale serve](docs/guides/deploy-behind-tailscale-serve.md),
  [write a role](docs/guides/write-a-role.md), [verify a release](docs/guides/verify-a-release.md),
  [troubleshoot](docs/guides/troubleshoot.md)
- Reference: [config](docs/reference/config.md), [roles](docs/reference/roles.md),
  [login](docs/reference/login.md), [token metadata](docs/reference/token-metadata.md)
- Explanation: [how a login works](docs/explanation/how-a-login-works.md),
  [security model](docs/explanation/security-model.md), [limitations](docs/explanation/limitations.md)
- [Testing](docs/testing.md), [contributing](CONTRIBUTING.md), [security policy](SECURITY.md)

## License

[Mozilla Public License 2.0](LICENSE)
