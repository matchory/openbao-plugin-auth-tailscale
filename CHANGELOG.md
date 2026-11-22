# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Roles can bind app capabilities (`bound_capabilities`). A node must then
  hold every one of them, granted by the tailnet policy to the OpenBao
  nodes' tag, as well as carry the role's tags. A renewal checks them again.

## [0.1.0] - 2026-11-22

### Added

- The `tailscale` auth method: a caller logs in with a role name and no
  secret, as the tailnet node its request comes from.
- Roles bind Tailscale tags (`bound_tags`) to OpenBao's token settings.
- Every token is bound to the node's own tailnet addresses, IPv4 and IPv6.
- A renewal checks the node and the role again. It is refused when the
  address belongs to another node, and when the role's policies have changed.
- These are refused: nodes without tags, shared-in nodes, nodes with an
  expired key, and nodes `tailscaled` doesn't list as allowed to send from
  the source address.
- Subnet routers and exit nodes are refused, by the routes the control
  server lets them send from and by the routes they advertise, approved or
  not.
- The OpenBao hosts can't log in: the node `tailscaled` runs as is refused,
  and so is a role that binds only tags that node carries, which would match
  every OpenBao node of a cluster.
- A refusal is logged at debug level. A failure to decide is logged as a
  warning: `tailscaled` unreachable, logged out or stopped, or a request
  without connection information.
- The login path resolves its role for OpenBao, so a rate limit quota can
  apply to one role.
- Tested with OpenBao 2.6 and 2.7, `tailscaled` 1.104, and a cluster of two
  OpenBao nodes.
