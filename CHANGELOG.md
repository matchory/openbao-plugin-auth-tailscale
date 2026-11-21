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
- Tested with OpenBao 2.6 and 2.7 and `tailscaled` 1.104.
