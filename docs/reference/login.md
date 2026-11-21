# Login

`POST auth/tailscale/login` logs the caller in. It needs no token and no secret.

```bash
curl -s -X POST -d '{"role":"api"}' https://openbao.example.ts.net/v1/auth/tailscale/login
```

## Request

### role

| | |
| --- | --- |
| Type | string |
| Default | none: required |

The role to log in with. A node can use any role whose tags it carries; there is no default role.

## Response

A successful login answers `200` with OpenBao's usual `auth` object:

| Field | Value |
| --- | --- |
| `auth.client_token` | The token |
| `auth.lease_duration` | Its lifetime in seconds |
| `auth.renewable` | `true` for a service token |
| `auth.policies` | The role's policies, plus `default` unless the role leaves it off |
| `auth.metadata` | The node that logged in: see [token metadata](token-metadata.md) |

The token is bound to the node's own tailnet addresses.

A refused login answers `403` with `permission denied`, whatever the reason.
[Troubleshoot](../guides/troubleshoot.md) lists the reasons and where to read them.

## Renewal

`POST auth/token/renew-self`, with the token, renews it. The request must come from the node that logged in.
A renewal repeats every check of a login against the role as it is at that moment, and is refused if the
address now belongs to another node.
