// Package tsauth is the root of an OpenBao auth plugin that logs callers in
// by the Tailscale identity of their source address.
//
// The rules about addresses, nodes and tags are in package identity, the
// client for the local tailscaled in package tailscaled, and the OpenBao
// backend in package backend.
package tsauth
