# Limit logins per role

An OpenBao rate limit quota can name one role of this method. At the end you'll have a quota that limits how often
callers can log in with that role, while every other role keeps its own pace.

You need a token that can write `sys/quotas/rate-limit`.

## Why limit logins

The login path needs no token, and each login asks `tailscaled` on the OpenBao node before anything else. A
caller that loops on logins costs a question to `tailscaled` and a token each time. A quota caps that, and a quota
per role keeps one noisy workload from using up the limit of the others.

## Write the quota

```bash
bao write sys/quotas/rate-limit/api-logins path=auth/tailscale/ role=api rate=10 interval=1m
```

- `path` is the method's mount, with the trailing slash.
- `role` is the role name. Role names are lower case, and the method lowers the name a caller sends, so `API` and
  `api` count as one role.
- `rate` and `interval` allow 10 logins a minute. A login past the limit answers `429` until the interval has
  passed.

The quota counts every login request that names the role, refused or not: OpenBao applies it before the method
decides. The method names the role for OpenBao without looking it up, so a quota can name a role that doesn't
exist yet, and a caller learns nothing about which roles exist from the answer.

To limit every login of the method together, leave out `role`:

```bash
bao write sys/quotas/rate-limit/tailscale-logins path=auth/tailscale/ rate=100 interval=1m
```

Both kinds can exist at once, but only the most specific one applies to a login: a login for `api` counts against
`api-logins` alone, even when `tailscale-logins` is stricter. The quota for the mount limits the roles that have no
quota of their own.

## Check it

```bash
bao read sys/quotas/rate-limit/api-logins
```

It shows the `path`, `role`, `rate` and `interval` you wrote. Log in from a node that carries the role's tags more
often than the rate allows: the logins past the limit answer `429`, and logins for other roles still answer as
before.

OpenBao's own documentation of `sys/quotas/rate-limit` lists the other fields, such as `block_interval`.
