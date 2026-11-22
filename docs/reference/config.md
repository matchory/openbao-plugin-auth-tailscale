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
local user can. The plugin asks it `whois` and `status`.

Whoever can write this field, or replace the socket at its path, decides which nodes can log in:
[Security model](../explanation/security-model.md#who-controls-the-configuration-controls-the-method).

## timeout

| | |
| --- | --- |
| Type | duration |
| Default | `2s` |

How long a login or renewal may wait for `tailscaled`. When it passes, the login or renewal is refused.

It counts in whole seconds and must be at least `1s`. A value under a second, such as `500ms`, is truncated to
`0` and rejected. A read returns it in seconds.
