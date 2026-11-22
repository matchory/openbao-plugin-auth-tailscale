# One OpenBao node of the end-to-end test's cluster. run.sh replaces @WORK@,
# @NODE@ (openbao1, openbao2), @P@ (the first digit of every port: 1 or 2)
# and the JOIN comment in the raft block (with how the node joins the
# cluster, or nothing): a comment, so the template stays valid HCL.
storage "raft" {
  path    = "@WORK@/@NODE@"
  node_id = "@NODE@"
  # @JOIN@
}

disable_mlock    = true
api_addr         = "http://127.0.0.1:@P@8210"
cluster_addr     = "https://127.0.0.1:@P@8211"
plugin_directory = "@WORK@/plugins"

# For run.sh itself: setup, and the requests that claim a forged address.
listener "tcp" {
  address     = "127.0.0.1:@P@8210"
  tls_disable = true
}

# Behind `tailscale serve --http`: trusts X-Forwarded-For from loopback only.
listener "tcp" {
  address                          = "127.0.0.1:@P@8202"
  tls_disable                      = true
  x_forwarded_for_authorized_addrs = "127.0.0.1/32"
}

# Behind `tailscale serve --tcp --proxy-protocol`: trusts the PROXY header
# from loopback only, and terminates TLS itself.
listener "tcp" {
  address                         = "127.0.0.1:@P@8200"
  tls_cert_file                   = "@WORK@/tls.pem"
  tls_key_file                    = "@WORK@/tls.key"
  proxy_protocol_behavior         = "allow_authorized"
  proxy_protocol_authorized_addrs = "127.0.0.1/32"
}
