# Deploy behind tailscale serve

At the end, OpenBao sees every caller's real tailnet address, and nothing on the OpenBao host but `tailscaled`
can claim an address that isn't its own. Both are requirements: the source address is the proof of identity.

`tailscale serve` hands a connection to OpenBao from `127.0.0.1`, so OpenBao has to be told the caller's address
and has to believe it only from there. There are two ways.

| | HTTP proxy | TCP forward with PROXY protocol |
| --- | --- | --- |
| TLS ends at | `tailscale serve` | OpenBao |
| Certificate | Tailscale's, for the node or Service name | Your own, on the OpenBao listener |
| The caller's address travels in | `X-Forwarded-For` | The PROXY protocol header |
| HTTP/2 between caller and OpenBao | No: serve makes its own connection to OpenBao | Yes |

Choose the HTTP proxy when you want Tailscale to handle the certificate. Choose the TCP forward when callers
should speak TLS to OpenBao itself.

## HTTP proxy

OpenBao listener:

```hcl
listener "tcp" {
  address                          = "127.0.0.1:8202"
  tls_disable                      = true
  x_forwarded_for_authorized_addrs = "127.0.0.1/32"
}
```

Serve:

```bash
sudo tailscale serve --bg --https=443 http://127.0.0.1:8202
```

A caller can't name another address this way: OpenBao uses the address `tailscale serve` reports, whatever
`X-Forwarded-For` the caller sent. The end-to-end test checks it.

## TCP forward with PROXY protocol

OpenBao listener:

```hcl
listener "tcp" {
  address                         = "127.0.0.1:8200"
  tls_cert_file                   = "/etc/openbao/tls/openbao.pem"
  tls_key_file                    = "/etc/openbao/tls/openbao.key"
  proxy_protocol_behavior         = "allow_authorized"
  proxy_protocol_authorized_addrs = "127.0.0.1/32"
}
```

Serve:

```bash
sudo tailscale serve --bg --tcp=8200 --proxy-protocol=2 tcp://127.0.0.1:8200
```

## Let only tailscaled reach the listener

Both listeners believe whatever arrives from `127.0.0.1`. Any other process on the host could connect and name
any node's address. Close that with a firewall rule that lets only root, which `tailscaled` runs as, connect:

```bash
sudo nft add table inet openbao_loopback
sudo nft 'add chain inet openbao_loopback output { type filter hook output priority 0; policy accept; }'
sudo nft 'add rule inet openbao_loopback output oifname lo tcp dport { 8200, 8202 } meta skuid != 0 reject with tcp reset'
```

Make the rule permanent the way your distribution keeps nftables rules.

> [!WARNING]
> Without this rule, a local process on the OpenBao host can log in as any tagged node. Root on that host can
> always do so; the rule is about everything else that runs there.

The `bao` command on the host is refused by the rule too, unless it runs as root. Give operators a third
listener that trusts no forwarded address, or have them use OpenBao through the tailnet like every other caller.

## Check it

From a tagged node, log in and read the address OpenBao recorded:

```bash
curl -s -X POST -d '{"role":"api"}' https://openbao.example.ts.net/v1/auth/tailscale/login |
  jq -r .auth.metadata.address
```

It prints that node's tailnet address. If it prints `127.0.0.1`, or the login is refused, the listener isn't
being told the caller's address: see [Troubleshoot](troubleshoot.md).

On the OpenBao host, as a user that isn't root, the listener must be closed:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -m 3 http://127.0.0.1:8202/v1/sys/health
```

It prints `000`.

## More than one OpenBao node

Each OpenBao node asks its own `tailscaled`. Put the same listener, serve configuration and firewall rule on
every node, and make sure every node can see every caller: if the nodes share a Tailscale Service, they all do.
A standby answers a login itself, asking its own `tailscaled`, so a node's `tailscaled` must be running for
logins that reach that node. The end-to-end test shows this on OpenBao 2.6 and 2.7, with each node's plugin
asking its own `tailscaled`.

A token is bound to the caller's addresses in both families, so it doesn't matter whether a later request
arrives over IPv4 or IPv6.
