# Testing

The plugin has four kinds of test. Each answers a different question, and CI runs all of them.

| Kind | Where | Runs | Answers |
| --- | --- | --- | --- |
| Unit | `*_test.go` in each package | `make test`, on every push | Does each rule, path and conversion behave as specified? |
| Fuzz | `identity/fuzz_test.go`, `tailscaled/fuzz_test.go` | weekly, 10 minutes per target | Does any input, or any answer from `tailscaled`, make the rules accept what they must refuse, or crash? |
| Guarantee check | `security_test.go` | `make test`, on every push | Does every guarantee in `SECURITY.md` have a test that proves it? |
| End-to-end | `test/integration/run.sh` | `make e2e`, on every pull request | Does a login work, and fail, as it should with real components? |

## Unit tests

```bash
make test                          # everything, with the race detector
go test ./backend/ -run TestLogin  # one package, one group
```

`identity` is tested with tables: one row per accepted or refused case. `tailscaled` is tested against a small
HTTP server on a Unix socket that answers like the daemon's local API. `backend` is tested through OpenBao's own
request handling with in-memory storage and a stand-in for `tailscaled`, so a test sends the same request a
caller would.

## Fuzz tests

```bash
go test -run='^$' -fuzz='^FuzzParseRemoteAddr$' -fuzztime=30s ./identity/
go test -run='^$' -fuzz='^FuzzCheckNode$' -fuzztime=30s ./identity/
go test -run='^$' -fuzz='^FuzzWhoIsAnswer$' -fuzztime=30s ./tailscaled/
```

Each target states what must hold for any input: an accepted address is inside Tailscale's ranges, and an
accepted node owns the address and may send from nothing else. `FuzzWhoIsAnswer` starts from the JSON
`tailscaled` sends and checks an accepted node against that JSON, so it also catches a field the conversion drops.

A failing input is saved under the package's `testdata/fuzz/`; commit it with the fix, and it becomes a regression
test. The weekly run keeps its corpus in the Actions cache from one run to the next, and opens an issue when it
finds a failing input.

## Security model

`SECURITY.md` publishes the guarantees as headings of the form `### G1: …`. A test that proves one says so in its
doc comment, on the line that the function follows or that more comment lines follow:

```go
// Guards: G5
func TestRenewRefusedWhenNodeChanged(t *testing.T) {
```

The end-to-end script marks its sections the same way, with `# Guards: G2`, followed by lines that run a `check`
before the next blank line. `TestEveryGuaranteeIsGuarded` fails when a published guarantee has no guard, when a
guard names a guarantee that isn't published, and when a marker marks no test. A marker says which guarantee a
test is about, not that the test covers all of it: read the tests for what each one checks. Run it with
`-v` to list the files guarding each guarantee:

```bash
go test -v -run TestEveryGuaranteeIsGuarded .
```

## End-to-end test

```bash
make build && make e2e
```

It needs Linux, Docker, `curl`, `jq` and `openssl`, and no Tailscale account. On macOS, run it in a Linux
container that shares the Docker socket and the host network.

The script starts [headscale](https://headscale.net) as the control server, four `tailscaled` processes in
userspace mode, and an OpenBao server with the plugin registered:

| Node | Identity | Role in the test |
| --- | --- | --- |
| server | `tag:example-server` | Runs `tailscale serve` in front of OpenBao; the plugin asks its `tailscaled` |
| caller | `tag:example-caller` | Logs in |
| other | `tag:example-caller` | Tries to use the caller's token |
| person | a user, no tags | Tries to log in |

OpenBao is reached over both paths the plugin supports behind `tailscale serve`: the HTTP proxy, where OpenBao
trusts `X-Forwarded-For` from loopback, and the TCP forward with PROXY protocol, where OpenBao terminates TLS
itself. A request sent through a node's outbound proxy arrives from that node, which is how one script acts as
several nodes.

It checks that a login works over both paths and both address families, that a token works only from the
caller, that a role bound to another tag, an unknown role and an untagged node are all refused, that a forged
`X-Forwarded-For` changes nothing, that a renewal is refused once the caller's tag changes, and that a login is
refused once `tailscaled` is gone.

`OPENBAO_VERSION` and `TAILSCALE_VERSION` choose the versions; CI runs the newest release of OpenBao's two latest
minors. `E2E_PAUSE=600` keeps everything running for ten minutes after a failure.

## What no test covers

- A hosted Tailscale control server. headscale stands in for it; the plugin only talks to `tailscaled`.
- A node that hosts a Tailscale Service. The rule that lets it log in has a unit test; headscale has no Services.
- An OpenBao cluster with more than one node, including a login after a leader change.
- A listener directly on the tailnet interface. Userspace `tailscaled` hands connections to a backend from
  loopback, so the end-to-end test always goes through `tailscale serve`.
