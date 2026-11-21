# Releasing

A release is a `vX.Y.Z` tag on `main` (see `release.yml`). Before tagging, check that the plugin's security
guarantees hold for the commit, as [SECURITY.md](../SECURITY.md) describes them:

1. CI is green on the commit: lint, the Go tests (including the check that every published guarantee is
   guarded), the vulnerability scan, and the end-to-end test on every OpenBao version in its matrix.
2. The weekly fuzz run on `main` succeeded within the last 7 days. Check by hand whether a failing input is
   still open.
3. `CHANGELOG.md` has a section for the version, with its date, and `[Unreleased]` is empty.
4. Two builds of the commit give the same checksum:

   ```bash
   make build GOARCH=amd64 && sha256sum bin/*_linux_amd64
   ```

Then tag the commit, signed, and push the tag:

```bash
git tag -s v0.1.0 -m v0.1.0
git push origin v0.1.0
```

The workflow builds both architectures, writes `SHA256SUMS`, attests each binary and creates the GitHub
release with the version's section of `CHANGELOG.md` as its notes.

After it finishes, check the release as a user would: [Verify a release](guides/verify-a-release.md).

The release notes list the published guarantees and the tests that guard each: paste the output of

```bash
go test -v -run TestEveryGuaranteeIsGuarded . | grep ': G'
```
