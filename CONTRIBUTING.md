# Contributing

Bug reports, fixes and improvements are welcome. Open an issue first for
larger changes, so we can agree on the approach before you write the code.
Report security issues privately, as described in [SECURITY.md](SECURITY.md).

## Prerequisites

- Go, at the version in [go.mod](go.mod) or newer.
- [golangci-lint](https://golangci-lint.run/), at the version that
  [ci.yml](.github/workflows/ci.yml) pins.
- Linux and Docker for the end-to-end test.

## Build and test

```bash
make build   # bin/openbao-plugin-auth-tailscale_linux_<arch>, with the release's flags
make test    # unit tests, with the race detector
make lint    # golangci-lint
make vuln    # govulncheck
make e2e     # end-to-end, against a local headscale
```

CI runs the same targets. [docs/testing.md](docs/testing.md) describes every
kind of test the plugin has, and what each one covers.

## Linters

CI runs two linters; run both before opening a pull request.

**Go**: golangci-lint, configured in `.golangci.toml`:

```bash
golangci-lint run        # check
golangci-lint run --fix  # fix what can be fixed automatically
golangci-lint fmt        # format
```

**Everything else** (Markdown, YAML, shell):
[super-linter](https://github.com/super-linter/super-linter), configured in
[linter.yml](.github/workflows/linter.yml) with rule files in
`.github/linters/`. To run the most common check on its own:

```bash
npx -y markdownlint-cli2 --config .github/linters/.markdown-lint.yml "**/*.md"
```

## Code style

- Handle every error. Compare errors with `errors.Is` and `errors.As`, and
  wrap them with `%w`.
- Package `identity` imports only the standard library. It holds every rule
  about addresses, nodes and tags, so the rules can be read and tested
  without OpenBao or Tailscale.
- Every error on the login or renewal path is a refusal. Nothing about a
  node is kept between requests.
- Document exported identifiers. Other comments say why the code is the way
  it is, not what it does.

## Scope

- The plugin is for anyone running OpenBao on a tailnet. Keep code, docs and
  tests free of assumptions about a particular organisation or tailnet;
  examples use `tag:example-api` and `example.ts.net`.
- Tailscale's control server comes first. The end-to-end test runs against
  headscale, but a change should not trade Tailscale behaviour for headscale
  behaviour.
- Tests that verify a guarantee in [SECURITY.md](SECURITY.md) name it in a
  `// Guards: G<n>` comment.

## Pull requests

- Keep changes focused, with tests for new behaviour and fixed bugs.
- When behaviour changes, update the documentation. Follow the
  [style guide](docs/STYLE.md).
- Add an entry under `Unreleased` in [CHANGELOG.md](CHANGELOG.md).
- Sign your commits and sign them off (`git commit -S -s`). A pull request
  needs one approval.
- Dependency updates come from Dependabot.

[docs/releasing.md](docs/releasing.md) describes how a release is cut.

By contributing, you agree that your contributions are licensed under the
[Mozilla Public License 2.0](LICENSE).
