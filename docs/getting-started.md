# Getting started

You'll end with a node on your tailnet logging in to OpenBao with nothing but a role name, and a token that only
that node can use.

You need:

- A tailnet where you can create tags and tagged nodes.
- A Linux host on the tailnet for OpenBao 2.6 or newer. This page calls it `openbao`. The plugin is tested
  with `tailscaled` 1.104.
- A second node, tagged `tag:example-api`, with `curl`. This page calls it the caller.
- A grant in your tailnet policy that lets `tag:example-api` reach `openbao` on TCP port 80.

This setup is for trying the plugin out. It keeps OpenBao's data in memory and serves plain HTTP inside the
tailnet. [Deploy behind tailscale serve](guides/deploy-behind-tailscale-serve.md) covers a real deployment.

## 1. Install the plugin

Download the binary for your architecture from the
[releases page](https://github.com/matchory/openbao-plugin-auth-tailscale/releases), check it as
[Verify a release](guides/verify-a-release.md) describes, and put it in a directory of its own:

```bash
sudo install -D -m 0755 openbao-plugin-auth-tailscale_*_linux_amd64 \
  /etc/openbao/plugins/openbao-plugin-auth-tailscale
```

## 2. Start OpenBao

Save this as `openbao.hcl`:

```hcl
storage "inmem" {}

disable_mlock    = true
api_addr         = "http://127.0.0.1:8210"
plugin_directory = "/etc/openbao/plugins"

# For you, on this host.
listener "tcp" {
  address     = "127.0.0.1:8210"
  tls_disable = true
}

# For callers, behind tailscale serve. It believes the address serve reports.
listener "tcp" {
  address                          = "127.0.0.1:8202"
  tls_disable                      = true
  x_forwarded_for_authorized_addrs = "127.0.0.1/32"
}
```

Start it, then initialize and unseal it in a second terminal:

```bash
bao server -config=openbao.hcl
```

```bash
export VAULT_ADDR=http://127.0.0.1:8210
bao operator init -key-shares=1 -key-threshold=1
bao operator unseal    # paste the unseal key
export VAULT_TOKEN=... # the root token
```

## 3. Enable the auth method

```bash
bao plugin register \
  -sha256="$(sha256sum /etc/openbao/plugins/openbao-plugin-auth-tailscale | cut -d' ' -f1)" \
  auth openbao-plugin-auth-tailscale
bao auth enable -path=tailscale openbao-plugin-auth-tailscale
bao write auth/tailscale/role/api bound_tags=tag:example-api token_ttl=30m
```

`bao auth list` now shows `tailscale/`, and `bao read auth/tailscale/role/api` shows the role.

## 4. Put OpenBao on the tailnet

```bash
sudo tailscale serve --bg --http=80 http://127.0.0.1:8202
```

`tailscale serve status` lists `http://openbao.<your tailnet>.ts.net` with the proxy target.

## 5. Log in from the caller

On the caller, using the name from the last step:

```bash
curl -s -X POST -d '{"role":"api"}' http://openbao.example.ts.net/v1/auth/tailscale/login
```

The answer is JSON. `.auth.client_token` is the token, and `.auth.metadata` names the node that logged in:

```json
{
  "role": "api",
  "node_id": "nPKyyg3CNTRL",
  "node_name": "caller.example.ts.net.",
  "tags": "tag:example-api",
  "address": "100.64.0.2"
}
```

## 6. See that the token belongs to the caller

Still on the caller, the token works:

```bash
curl -s -H "X-Vault-Token: $TOKEN" http://openbao.example.ts.net/v1/auth/token/lookup-self
```

Send the same request from any other node, and OpenBao answers `permission denied`: the token is bound to the
caller's tailnet addresses.

## Where to go next

- [Deploy behind tailscale serve](guides/deploy-behind-tailscale-serve.md), before anything real depends on this.
- [Write a role](guides/write-a-role.md), for choosing tags and lifetimes.
- [Troubleshoot](guides/troubleshoot.md), if step 5 answered `permission denied`.
