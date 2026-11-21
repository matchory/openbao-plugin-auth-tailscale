# Security model

[SECURITY.md](../../SECURITY.md#security-model) is the home of the plugin's guarantees and of what they assume.
This page explains the reasoning behind them.

## The address is the credential

Most auth methods check something the caller sends. This one checks where the request comes from. That is
sound inside a tailnet, because WireGuard ties every packet to the node key of its sender: a node can't send
from another node's address.

It follows that the plugin is only as good as the address OpenBao reports. Everything between the caller and
OpenBao has to either keep the address or tell OpenBao what it was, and OpenBao must accept that information
from nothing else. Those are environment properties E1 and E2, and they are the operator's to ensure.
[Deploy behind tailscale serve](../guides/deploy-behind-tailscale-serve.md) shows how.

## A tag is the identity

The plugin doesn't know workloads; it knows tags. Whoever can put a tag on a node holds every role that binds
it: tailnet administrators, tag owners, and any credential that creates tagged nodes. That is property E3. It
is the same trust your tailnet policy already places in tags when it grants network access by them.

For the same reason a role should bind a tag only the workload's own node carries. Programs that share a node
share its identity.

## Why some nodes are always refused

- **Nodes without tags** are people's devices. People log in to OpenBao as themselves, with a method made for
  that.
- **Shared-in nodes** belong to another tailnet, whose administrators decide their tags.
- **Subnet routers and exit nodes** forward traffic for others, which arrives from the router's own address.

## Fail closed

The plugin can't tell an unknown address from an unreachable `tailscaled`, and doesn't try. Any error, any
timeout, any missing piece of an answer is a refusal. It also keeps nothing between requests, so there is no
stale answer to fall back on.

## One answer for every refusal

A caller that could tell "no such role" from "wrong tag" could list the roles. So every refusal is the same
`permission denied`, and the plugin asks `tailscaled` before reading the role, to keep the timing the same too.
The operator reads the reason in the debug log.

## How the guarantees are kept honest

Each guarantee in `SECURITY.md` is tied to the tests that prove it, and a test fails when one has none.
[Testing](../testing.md#security-model) describes the mechanism.
