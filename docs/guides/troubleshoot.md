# Troubleshoot

A refused login always answers `permission denied`. The reason is in OpenBao's log, at debug level, in a line
from the plugin that contains `refused:` (one line in the log, wrapped here):

```text
[DEBUG] auth.openbao-plugin-auth-tailscale...: refused:
  reason="node at 100.64.0.4: node has no tags" remote_addr=100.64.0.4 role=api
```

Set OpenBao's `log_level` to `debug`, repeat the login, and find the reason below. A refusal that is a failure
and not a decision about the caller, such as an unreachable `tailscaled`, is logged as a warning, so it shows
at the default level too.

| Reason | Cause | Fix |
| --- | --- | --- |
| `request carries no connection information` | The request didn't come through a listener | Log in over the network, not from a script inside OpenBao |
| `address is not a Tailscale address`, with `remote_addr=127.0.0.1` | The listener isn't being told the caller's address, or doesn't believe it | Set `x_forwarded_for_authorized_addrs` or the PROXY protocol settings as in [Deploy behind tailscale serve](deploy-behind-tailscale-serve.md) |
| `address is not a Tailscale address`, with another address | The caller reached OpenBao from outside the tailnet | Reach OpenBao over the tailnet |
| `ask tailscaled: ...` | `tailscaled` isn't running, the socket path is wrong, or it didn't answer within the timeout | Check `tailscale status` on the OpenBao host, and `socket` and `timeout` in [config](../reference/config.md) |
| `ask tailscaled: tailscaled is Stopped, not Running` (or `NeedsLogin`, `Starting`) | `tailscaled` runs but isn't connected to the tailnet | `tailscale up` on the OpenBao host |
| `no node has this address` | This OpenBao node's `tailscaled` doesn't know the caller | Give the caller a grant to this node or its Service, so it appears in the node's netmap |
| `node has no tags` | The caller is a person's device | Log in from a tagged node |
| `node is shared in from another tailnet` | The caller belongs to another tailnet | Not supported |
| `node key has expired` | The caller's key ran out | Re-authenticate the caller, or disable key expiry for it |
| `address is not one of the node's own` | The request arrived from an address the node routes for, not its own | Log in from the node itself |
| `node routes for other addresses` | The caller is a subnet router or an exit node, or advertises a route that isn't approved | Run the workload on a node of its own, or stop advertising the route (`tailscale set --advertise-routes=`) |
| `node may not send from this address` | `tailscaled` doesn't list the source address among those the node may send from | Compare `tailscale whois <address>` on the OpenBao host with the node's addresses |
| `node is the one tailscaled runs as` | The request came from the OpenBao host itself | Log in from another node; see [Limitations](../explanation/limitations.md#the-openbao-host-cant-log-in) |
| `no such role` | The role name is wrong | `bao list auth/tailscale/role` |
| `node lacks a bound tag` | The node doesn't carry every tag in the role's `bound_tags` | Compare `tailscale whois <address>` on the OpenBao host with the role |
| `address now belongs to another node` | On renewal: the address was given to a different node | Log in again from the new node |
| `role no longer grants the token's policies` | On renewal: the role's policies changed after the login | Log in again |

## The plugin doesn't start

`bao auth enable` answers with a checksum error when the binary in `plugin_directory` isn't the one registered.
Register the checksum of the file that is actually there:

```bash
sha256sum /etc/openbao/plugins/openbao-plugin-auth-tailscale
```

## A token stops working

A token works only from the node's own tailnet addresses. It answers `permission denied` from anywhere else,
including from the same machine over a different network path.
