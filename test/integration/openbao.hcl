# The OpenBao of the end-to-end test. run.sh replaces @WORK@.
storage "inmem" {}

disable_mlock    = true
api_addr         = "http://127.0.0.1:18210"
plugin_directory = "@WORK@/plugins"

# For run.sh itself: setup, and the requests that claim a forged address.
listener "tcp" {
  address     = "127.0.0.1:18210"
  tls_disable = true
}

# Behind `tailscale serve --http`: trusts X-Forwarded-For from loopback only.
listener "tcp" {
  address                          = "127.0.0.1:18202"
  tls_disable                      = true
  x_forwarded_for_authorized_addrs = "127.0.0.1/32"
}

# Behind `tailscale serve --tcp --proxy-protocol`: trusts the PROXY header
# from loopback only, and terminates TLS itself.
listener "tcp" {
  address                         = "127.0.0.1:18200"
  tls_cert_file                   = "@WORK@/tls.pem"
  tls_key_file                    = "@WORK@/tls.key"
  proxy_protocol_behavior         = "allow_authorized"
  proxy_protocol_authorized_addrs = "127.0.0.1/32"
}
