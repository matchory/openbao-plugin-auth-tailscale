# openbao-plugin-auth-tailscale

An [OpenBao](https://openbao.org) auth method that logs a caller in by its
[Tailscale](https://tailscale.com) identity. The caller sends a role name and
no secret; the plugin asks the local `tailscaled` which node owns the
request's source address, and issues a token if that node carries the role's
tags.

Work in progress: there is no release yet.

## License

[Mozilla Public License 2.0](LICENSE)
