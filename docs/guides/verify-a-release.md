# Verify a release

OpenBao runs a plugin only if its SHA-256 matches the one you registered. So the checksum you register should
be one you have checked yourself, against what the release workflow built.

You need the [GitHub CLI](https://cli.github.com/) for the second check.

## Check the checksum

Download the binary and `SHA256SUMS` from the
[releases page](https://github.com/matchory/openbao-plugin-auth-tailscale/releases) into one directory:

```bash
sha256sum --ignore-missing -c SHA256SUMS
```

It prints `OK` for the binary you downloaded.

## Check where it was built

Each binary has a build provenance attestation, signed by GitHub for this repository's release workflow:

```bash
gh attestation verify openbao-plugin-auth-tailscale_0.1.0_linux_amd64 \
  --repo matchory/openbao-plugin-auth-tailscale
```

It prints that the verification succeeded, with the workflow and the tag the binary was built from. A binary
that was changed after the build, or built anywhere else, fails this check.

## Build it yourself

Releases are reproducible. With the Go toolchain `go.mod` names, the same tag gives the same bytes:

```bash
git checkout v0.1.0
make build GOARCH=amd64
sha256sum bin/openbao-plugin-auth-tailscale_linux_amd64
```

The checksum equals the one in `SHA256SUMS`.

## Register what you checked

```bash
bao plugin register -sha256=<the checksum> -version=v0.1.0 auth openbao-plugin-auth-tailscale
```
