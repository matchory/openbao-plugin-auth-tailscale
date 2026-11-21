# Config

`auth/tailscale/config` holds how the method reaches the local `tailscaled`. It works without being written:
both fields have defaults.

```bash
bao write auth/tailscale/config socket=/run/tailscale/tailscaled.sock timeout=2s
bao read auth/tailscale/config
```

## socket

| | |
| --- | --- |
| Type | string |
| Default | empty: the platform's default, `/var/run/tailscale/tailscaled.sock` on Linux |

Path of the socket of the `tailscaled` to ask. The user OpenBao runs as must be able to open it; by default any
local user can.

## timeout

| | |
| --- | --- |
| Type | duration |
| Default | `2s` |

How long one question to `tailscaled` may take. When it passes, the login or renewal is refused. It must be
greater than zero.

A read returns it in seconds.
