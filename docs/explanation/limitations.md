# Limitations

## Identity is per node and per tag

Two programs on one node are one identity. Two nodes with the same tags are, to a role, the same. If two
workloads need different access, give them different tags.

## The loopback listener is the operator's to protect

Behind `tailscale serve`, OpenBao believes the caller's address from `127.0.0.1`. A local process on the
OpenBao host can claim any address unless a firewall rule stops it. The plugin can't see the difference.
[Deploy behind tailscale serve](../guides/deploy-behind-tailscale-serve.md#let-only-tailscaled-reach-the-listener)
has the rule.

## The control server is trusted

A node is who the Tailscale control server says it is. The plugin is not tested with Tailnet Lock.

## Batch tokens are never re-checked

A service token is checked again at every renewal. A batch token can't be renewed, so it stays valid until its
TTL ends, even if the node's tag is removed in the meantime. It is still bound to the node's addresses.

## A token is checked again only at renewal

Between renewals OpenBao doesn't ask the plugin anything. If a node is deleted and its address given to another
node, a token issued to the first keeps working from that address until its TTL ends or a renewal is refused.
Short TTLs keep that window small.

## No login without tailscaled

When the `tailscaled` on an OpenBao node is down, that node refuses every login and renewal. Tokens already
issued keep working until they expire.

## Not covered by the tests

The plugin has not been tested behind a listener directly on the tailnet interface, in an OpenBao cluster of
several nodes, or with nodes that host Tailscale Services. [Testing](../testing.md#what-no-test-covers) says why.

## OpenBao only

There is no build for HashiCorp Vault. The rules live in a package that doesn't depend on OpenBao's SDK, so one
is possible.
