# Token metadata

Every token the method issues carries five metadata keys. They appear in the login response, in
`auth/token/lookup-self`, and in OpenBao's audit log with each request the token makes.

| Key | Value | Example |
| --- | --- | --- |
| `role` | The role the node logged in with | `api` |
| `node_id` | The node's stable ID, which stays the same for as long as the node exists | `nPKyyg3CNTRL` |
| `node_name` | The node's name in the tailnet | `api-1.example.ts.net.` |
| `tags` | The node's tags at login, sorted and comma-separated | `tag:example-api,tag:prod` |
| `address` | The source address of the login request | `100.64.0.2` |

## Entity and alias

The method's alias name is the role name. Every node that logs in with a role shares one entity, so a workload
whose nodes come and go stays one entity. To tell nodes apart, use `node_id` in the audit log.

Templates that take an identity from the alias, such as a PKI role's allowed names or an identity token's
claims, see the role name.
