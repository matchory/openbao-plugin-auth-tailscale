# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- The `tailscale` auth method: a caller logs in with a role name and no
  secret, as the tailnet node its request comes from.
- Roles bind Tailscale tags (`bound_tags`) to OpenBao's token settings.
- Every token is bound to the node's own tailnet addresses, IPv4 and IPv6.
- A renewal checks the node and the role again. It is refused when the
  address belongs to another node, and when the role's policies have changed.
- Nodes without tags, shared-in nodes, nodes with an expired key, subnet
  routers and exit nodes are refused.
- A node that advertises routes is refused as a router, approved or not,
  even when the control server leaves the routes out of what the OpenBao
  host sees.
- The node `tailscaled` runs as is refused, so a local process on the OpenBao
  host can't log in as the host.
- A node is refused unless `tailscaled` lists the source address among those
  it may send from.
- A `tailscaled` that is logged out or stopped is logged as a warning, not as
  a debug-level refusal.
- A request without connection information is logged as a warning: it did
  not come through a listener, which is a deployment to look at.
- The login path resolves its role for OpenBao, so a rate limit quota can
  apply to one role.
- Tested with OpenBao 2.6 and 2.7 and `tailscaled` 1.104.
