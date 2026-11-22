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

The binding is to addresses, not to the node. When an address passes to
another node, a token issued for it keeps working from that address until a
renewal is refused (G5) or the token's TTL ends. The TTL is the longest that
window can be.

### G3: A node gets a role only if it carries every tag the role binds

Attackers: any other node on the tailnet; a local process on an OpenBao host.
Assumes: E3, E5, E6.

These nodes get no role at all:

- a node without tags (a person's device);
- a node shared in from another tailnet;
- a node whose key has expired;
- a node that routes for other addresses, by what the control server says or
  by what the node advertises, approved or not (a subnet router or an exit
  node, whose clients would arrive from the router's address);
- a node `tailscaled` does not list as allowed to send from the source
  address, including one whose list is empty;
- the node `tailscaled` itself runs as, which every local process on the
  OpenBao host can send from.

The plugin reads routes in two places: the routes the control server lets
the node send from, and the routes the node itself advertises. A control
server can leave a route out of the first for an OpenBao host the policy
doesn't let use it, as headscale does; the second still names it. A node
that forwards traffic without advertising a route, through NAT for example,
looks like any other node: E6 is what keeps such a node out of a role.

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
such as its type, its number of uses or `token_no_default_policy`, does not
end tokens already issued.

These checks run when a token is renewed, not in between. A tag removed from a
node ends its tokens at their next renewal, or at the end of their TTL if they
are not renewed.

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
| E3 | Root on the OpenBao hosts, the tailnet's administrators, every holder of a credential that can create a node with a bound tag, and every holder of write access to the method's `config` path are trusted. The configured socket and its directory can be replaced only by root or `tailscaled` | operator, tailnet policy, OpenBao policy |
| E4 | Every OpenBao node can answer WhoIs for every caller, which holds when all of them are reachable under the name callers use | operator |
| E5 | A node's identity is what the Tailscale control server says it is, unless Tailnet Lock is on, which the plugin is not tested with | operator |
| E6 | A node that carries a bound tag sends only its own traffic: it forwards nothing for other hosts, containers or networks | operator, tailnet policy |

E4 is about availability: where it does not hold, logins are refused, not
wrongly accepted.

E3 covers `config` because its `socket` names the daemon the plugin trusts:
whoever can point it at a socket of their own can answer for any node.

### What the plugin does not defend against

- A local process on an OpenBao host, where E2 does not hold. It can send a
  forged forwarded address to the loopback listener and log in as any node.
- Anyone who can give a node a tag. A tag is the identity; whoever can assign
  it can become it.
- A caller behind the same node as another. Identity is per node, so two
  programs on one node are one identity.
- Anything behind a node that forwards traffic, where E6 does not hold. It
  arrives from the node's address and logs in as the node.
