# Compatibility

From 1.0, the plugin follows [semantic versioning](https://semver.org/spec/v2.0.0.html). You can upgrade
within a major version without changing your roles, config, policies or the scripts that log in. A change that
would break what this page lists comes only in a new major version, under "Changed" or "Removed" in
[CHANGELOG.md](../../CHANGELOG.md). Before 1.0, a minor release can break it, and the changelog says so.

## What stays the same within a major version

| Interface | What holds |
| --- | --- |
| Paths | `config`, `role/<name>` (read, write, list, delete) and `login` keep their paths and operations |
| [Config](config.md) and [role](roles.md) fields | Names, types, defaults and the values each accepts. A new field is optional, and its default keeps today's behaviour |
| Stored roles and config | What one release stored, every later release of the major version reads and applies the same way |
| [Login](login.md) | The request takes `role`; a refusal is `403` with `permission denied` and nothing else |
| [Token metadata](token-metadata.md) | The five keys and what each holds. A release may add a key |
| Entity alias | The alias name is the role name |
| Guarantees | The guarantees G1 to G6 in [SECURITY.md](../../SECURITY.md#security-model). A release may add one or strengthen one, never weaken or remove one |
| Release files | The binary names, `SHA256SUMS` and the build provenance attestation that [Verify a release](../guides/verify-a-release.md) checks |

## What can change in any release

- **A login that used to succeed can be refused.** When a release closes a way to get a token the guarantees
  don't allow, it refuses those logins at once and lists the change under "Security" in the changelog. The
  plugin fails closed, and waiting for a major version would leave the gap open.
- **Log messages**, including the refusal reasons in [Troubleshoot](../guides/troubleshoot.md). Read them, but
  don't match on their exact text.
- **The Go packages.** `identity`, `tailscaled` and `backend` are the plugin's own code, not a library.
- **The Go toolchain and dependencies** a release is built with. Each release's checksums are its own.

## Supported versions

| | Supported |
| --- | --- |
| OpenBao | The minor versions the end-to-end test runs on: 2.6 and 2.7 |
| `tailscaled` | 1.104, the version the end-to-end test runs |
| Platforms | Linux on amd64 and arm64 |

The plugin asks `tailscaled` only for `whois` and `status` over its local API. Other versions aren't tested.
When a new OpenBao minor version comes out, the test adds it, and support for the oldest one can end in a minor
release, listed under "Changed".

Security fixes go into the latest release only, as [SECURITY.md](../../SECURITY.md#supported-versions) says.
