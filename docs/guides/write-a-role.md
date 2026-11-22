# Write a role

A role says which Tailscale tags a node must carry, and what a node that carries them gets. At the end you'll
have a role that only the workload you mean can use.

## Choose the tags

Bind the tag that names the workload, and nothing broader:

```bash
bao write auth/tailscale/role/api bound_tags=tag:example-api token_policies=api
```

A node must carry every tag in `bound_tags`. Listing two tags narrows the role to nodes with both:

```bash
bao write auth/tailscale/role/api-prod bound_tags=tag:example-api,tag:prod token_policies=api-prod
```

Never bind a tag that hosts carry, such as a tag for all servers. Containers and other programs that reach the
tailnet through their host's Tailscale arrive from the host's address, so every one of them would hold the role.
Bind tags that only the workload's own node has.

Never bind the tags of your OpenBao nodes either. A login for a role that binds only tags the OpenBao host's
node carries is always refused, since every local process on any OpenBao host would hold it.

A tag is the identity. Check in your tailnet policy who owns each tag you bind (`tagOwners`), and which
credentials can create nodes with it: each of them can use the role.

## Choose the lifetimes

```bash
bao write auth/tailscale/role/api bound_tags=tag:example-api token_policies=api \
  token_ttl=30m token_max_ttl=4h
```

Keep `token_ttl` short: logging in again costs the caller one request and no secret. Each renewal checks the
node again and is refused if the node lost a bound tag or the role's tags or policies changed, so a short TTL
is also how soon such a change takes effect. Other token settings of a role apply to tokens issued afterwards.

Batch tokens (`token_type=batch`) are never renewed, so nothing re-checks them. Give them a TTL you'd accept as
the delay before a removed tag takes effect.

When a node's key expires, its tokens end no later than the key does, whatever the role says.

## Let the caller reach OpenBao

A role is not enough: the tailnet policy must also let the tag reach OpenBao. That grant is the tailnet's own
veto over who may even ask.

## Check it

From a node with the tags, a login answers with a token. From a node without them, it answers
`permission denied`:

```bash
curl -s -X POST -d '{"role":"api"}' https://openbao.example.ts.net/v1/auth/tailscale/login | jq .auth.metadata
```

[Roles](../reference/roles.md) lists every field.
