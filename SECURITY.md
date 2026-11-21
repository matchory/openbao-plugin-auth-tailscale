# Security policy

## Reporting a vulnerability

Report vulnerabilities privately through GitHub's
[private vulnerability reporting](https://github.com/matchory/openbao-plugin-auth-tailscale/security/advisories/new)
on matchory/openbao-plugin-auth-tailscale. Do not open a public issue, pull
request or discussion for them.

Include what is affected, how to reproduce it, and the impact you expect. We
will confirm receipt, keep you informed while we work on a fix, and credit you
in the advisory unless you prefer otherwise.

Vulnerabilities in Tailscale itself go to
[Tailscale](https://tailscale.com/security), and those in OpenBao to the
[OpenBao project](https://github.com/openbao/openbao/security/policy).

## Supported versions

Fixes go into the latest release. Older releases are not patched.

## Security model

The plugin makes the guarantees below, provided the environment properties
after them hold. Each guarantee names who it defends against and which
properties it relies on. Tests that verify a guarantee carry its ID
(`Guards: G1`); a check in the test suite fails if a guarantee listed here has
none.

### G1: A login succeeds only for the node that owns the source address

Attackers: any other node on the tailnet; a caller outside the tailnet.
Assumes: E1, E2, E5.

The plugin takes the source address OpenBao reports for the request and asks
the local `tailscaled` which node owns it. A request from outside Tailscale's
address ranges is refused without asking. A node is never accepted for an
address that is not one of its own.

### G2: A token works only from the addresses of the node it was issued to

Attackers: a holder of a leaked token, on the tailnet or off it.
Assumes: E1, E2.

Every token is bound to the node's own tailnet addresses, IPv4 and IPv6, and
to nothing else. A role cannot widen or remove that binding, and neither can
a role entry changed in storage.

### G3: A node gets a role only if it carries every tag the role binds

Attackers: any other node on the tailnet.
Assumes: E3, E5.

A node without tags (a person's device), a node shared in from another
tailnet, a node whose key has expired, and a node that routes for other
addresses (a subnet router or an exit node, whose clients would arrive from
the router's address) get no role at all.

### G4: Nothing succeeds when tailscaled cannot be asked

Attackers: anyone able to make `tailscaled` slow or unavailable.
Assumes: nothing.

A login or a renewal is refused when `tailscaled` gives no answer within the
configured timeout, or an answer without a node. The plugin keeps nothing
`tailscaled` said from one request to the next.

### G5: A renewal succeeds only for the node that logged in, with the tags and policies the role has now

Attackers: a node that was given an address another node had; a holder of a
leaked token; a node whose tag or role was taken away.
Assumes: E1, E2, E5.

A renewal repeats every check of a login and also requires the node at the
address to be the node that logged in. A tailnet address can pass to another
node; the stable node ID the plugin compares cannot.

A token keeps the policies it was issued with, so a renewal is refused when
the role's policies have changed since: the caller logs in again and gets a
token under the role as it is. A change to a role's other token settings,
such as its type or its number of uses, does not end tokens already issued.

Batch tokens are never renewed, so nothing re-checks them. They end with
their TTL.

### G6: A refused caller learns nothing about why

Attackers: any caller that can reach the login path.
Assumes: nothing.

Every refusal is the same permission-denied answer, whether the role does not
exist, the node lacks a tag, or `tailscaled` did not answer. The plugin asks
`tailscaled` before it looks up the role, so the time a refusal takes does
not tell whether a role exists. The reason goes to the plugin's debug log.

### Environment properties

| ID | Property | Ensured by |
| --- | --- | --- |
| E1 | Every path to OpenBao delivers the caller's real source address: a listener on the tailnet interface, or `tailscale serve` in front of a listener that trusts forwarded addresses from loopback only | operator |
| E2 | Only `tailscaled` can connect to a loopback listener that trusts forwarded addresses | operator, host firewall |
| E3 | Root on the OpenBao hosts, the tailnet's administrators, and every holder of a credential that can create a node with a bound tag are trusted | operator, tailnet policy |
| E4 | Every OpenBao node can answer WhoIs for every caller, which holds when all of them are reachable under the name callers use | operator |
| E5 | A node's identity is what the Tailscale control server says it is, unless Tailnet Lock is on, which the plugin is not tested with | operator |

E4 is about availability: where it does not hold, logins are refused, not
wrongly accepted.

### What the plugin does not defend against

- A local process on an OpenBao host, where E2 does not hold. It can send a
  forged forwarded address to the loopback listener and log in as any node.
- Anyone who can give a node a tag. A tag is the identity; whoever can assign
  it can become it.
- A caller behind the same node as another. Identity is per node, so two
  programs on one node are one identity.
