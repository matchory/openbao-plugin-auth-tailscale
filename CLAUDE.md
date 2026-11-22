# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

An OpenBao auth plugin: a caller logs in with a role name and no secret, and
the plugin asks the local `tailscaled` which node owns the request's source
address.

Read [SECURITY.md](SECURITY.md) before changing the login or renewal path.

## Layout

| Package | Holds | May import |
| --- | --- | --- |
| `identity` | Every rule about addresses, nodes and tags | The standard library only |
| `tailscaled` | `Client`: WhoIs against the local daemon, converted to `identity.Node`, and the daemon's own node | `identity`, `tailscale.com` |
| `backend` | The OpenBao credential backend: config, roles, login, renewal | `identity`, `tailscaled`, the OpenBao SDK |
| `cmd/openbao-plugin-auth-tailscale` | Serves the backend as an external plugin | `backend` |

A rule about who may log in belongs in `identity`, with a table test. The
backend only connects the rules to OpenBao.

## How a login flows

`backend/path_login.go` `attest` runs the same checks for a login and a
renewal: `identity.ParseRemoteAddr` on the connection's address, `WhoIs` and
`Self` through the backend's `whoIser`, `identity.CheckNode` with the daemon's
own node, then `identity.MatchTags` against the role's `bound_tags`. A renewal
also requires `identity.CheckSameNode`. A login then binds the token to
`identity.BoundCIDRs` and caps its lifetimes with `identity.CapTTL`.

`refuse` logs the reason and returns `logical.ErrPermissionDenied`. A decision
about the caller is an `identity.Refusal` and logs at debug; add new ones as
`Refusal` constants. Any other error is a failure to decide, which logs as a
warning for the operator.

`whoIser` is the interface in `backend/backend.go`; `*tailscaled.Client`
implements it. The backend creates its client through `newWhoIser`, and
backend tests replace that with a stand-in, so they never need a real
`tailscaled`.

## Commands

```bash
make test    # unit tests, race detector on
make lint    # golangci-lint, the version ci.yml pins
make vuln    # govulncheck
make build   # release-flag build into bin/
make e2e     # end-to-end against headscale; Linux and Docker
```

```bash
go test ./backend/ -run TestLogin                    # one package, one group
go test -v -run TestEveryGuaranteeIsGuarded .        # which tests guard each G<n>
go test -run='^$' -fuzz='^FuzzCheckNode$' -fuzztime=30s ./identity/
golangci-lint fmt                                    # format
```

`make e2e` needs a fresh `make build` first. On macOS, run it in a Linux
container that shares the Docker socket and host network; see
[docs/testing.md](docs/testing.md).

Run the tests a change touches while working. CI runs everything.

## Rules that are easy to break

- Fail closed. Any error while checking a login or a renewal is a refusal.
- Never cache what `tailscaled` said. Each login and each renewal asks again.
- A refusal returns `logical.ErrPermissionDenied` with no detail. The reason
  goes to the log: a `Refusal` at debug, a failure as a warning.
- A role never sets `token_bound_cidrs`. The plugin binds every token to the
  node's own addresses.
- A test that proves a guarantee carries `// Guards: G<n>` in its doc
  comment. A test fails if a guarantee in `SECURITY.md` has none, or if a
  marker marks no test.

## Standards

- Code passes `.golangci.toml`. A `nolint` names its linter and gives a reason.
- Exported identifiers have doc comments. Other comments say why, never what.
- `README.md` and `docs/` follow [docs/STYLE.md](docs/STYLE.md).
- Examples use `tag:example-api`, `tag:example-worker` and `example.ts.net`.
- A change in behaviour gets a line under `Unreleased` in `CHANGELOG.md`.
