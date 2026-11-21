# Development guide

An OpenBao auth plugin: a caller logs in with a role name and no secret, and
the plugin asks the local `tailscaled` which node owns the request's source
address.

Read [SECURITY.md](SECURITY.md) before changing the login or renewal path.

## Layout

| Package | Holds | May import |
| --- | --- | --- |
| `identity` | Every rule about addresses, nodes and tags | The standard library only |
| `tailscaled` | WhoIs against the local daemon, converted to `identity.Node` | `identity`, `tailscale.com` |
| `backend` | The OpenBao credential backend: config, roles, login, renewal | `identity`, `tailscaled`, the OpenBao SDK |
| `cmd/openbao-plugin-auth-tailscale` | Serves the backend as an external plugin | `backend` |

A rule about who may log in belongs in `identity`, with a table test. The
backend only connects the rules to OpenBao.

## Commands

```bash
make test    # unit tests, race detector on
make lint    # golangci-lint, the version ci.yml pins
make vuln    # govulncheck
make build   # release-flag build into bin/
make e2e     # end-to-end against headscale; Linux and Docker
```

Run the tests a change touches while working. CI runs everything.

## Rules that are easy to break

- Fail closed. Any error while checking a login or a renewal is a refusal.
- Never cache what `tailscaled` said. Each login and each renewal asks again.
- A refusal returns `logical.ErrPermissionDenied` with no detail. The reason
  goes to the debug log.
- A role never sets `token_bound_cidrs`. The plugin binds every token to the
  node's own addresses.
- A test that proves a guarantee carries `// Guards: G<n>`. A test fails if a
  guarantee in `SECURITY.md` has none.

## Standards

- Code passes `.golangci.toml`. A `nolint` names its linter and gives a reason.
- Exported identifiers have doc comments. Other comments say why, never what.
- `README.md` and `docs/` follow [docs/STYLE.md](docs/STYLE.md).
- Examples use `tag:example-api`, `tag:example-worker` and `example.ts.net`.
- A change in behaviour gets a line under `Unreleased` in `CHANGELOG.md`.
