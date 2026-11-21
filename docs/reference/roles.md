# Roles

`auth/tailscale/role/<name>` binds Tailscale tags to token settings. Role names are case-insensitive and stored
in lower case.

```bash
bao write auth/tailscale/role/api bound_tags=tag:example-api token_policies=api token_ttl=30m
bao read auth/tailscale/role/api
bao list auth/tailscale/role
bao delete auth/tailscale/role/api
```

A write changes only the fields it names.

[Write a role](../guides/write-a-role.md) explains how to choose the values.

## bound_tags

| | |
| --- | --- |
| Type | list of strings, comma-separated on the command line |
| Default | none: required |

The tags a node must carry to use the role. It must carry all of them. Each entry has the form `tag:<name>`,
where the name starts with a letter and continues with letters, digits and dashes. Repeated entries are stored
once. A role without tags is rejected.

## Token fields

These are OpenBao's standard token fields and behave as in every auth method.

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `token_ttl` | duration | `0`: the mount's default | Lifetime of a token, and what a renewal extends it to |
| `token_max_ttl` | duration | `0`: the mount's maximum | The longest a token can live through renewals |
| `token_policies` | list of strings | none | Policies on the token |
| `token_type` | string | `default` | `service`, `batch` or `default` |
| `token_period` | duration | `0` | Makes the token periodic: each renewal sets its lifetime to this |
| `token_explicit_max_ttl` | duration | `0` | A hard limit on the token's life, renewals included |
| `token_no_default_policy` | boolean | `false` | Leaves the `default` policy off the token |
| `token_num_uses` | integer | `0`: unlimited | How many requests the token may make |

When the node's key has an expiry, `token_ttl`, `token_max_ttl` and `token_period` are shortened so that a
token ends no later than the key.

`token_ttl` and `token_max_ttl` also bound how long a token outlives a change to its node: an address passed to
another node, or a tag removed. [Limitations](../explanation/limitations.md#a-token-is-checked-again-only-at-renewal)
has the details.

## Entity alias

Every token of a role gets the entity alias named after the role. A role that is deleted and created again under
the same name gets the same entity, with its groups and policies, even if its `bound_tags` are different.

## Fields a role does not have

`token_bound_cidrs` and `token_strictly_bind_ip` are not fields of a role. Every token is bound to the tailnet
addresses of the node that logged in, IPv4 and IPv6, and a role cannot change that. Writing either field has no
effect.
