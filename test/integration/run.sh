#!/usr/bin/env bash
# End-to-end test: real tailscaled processes, a real OpenBao with the plugin,
# and a headscale control server, all on this host. It needs Linux, Docker,
# curl, jq and openssl, and no Tailscale account.
#
#   make build && test/integration/run.sh
#
# PLUGIN_BINARY names the plugin to test (default: bin/, as make build
# leaves it). OPENBAO_VERSION and TAILSCALE_VERSION choose what it runs with.
set -euo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)

OPENBAO_VERSION=${OPENBAO_VERSION:-2.7.2}
TAILSCALE_VERSION=${TAILSCALE_VERSION:-1.104.7}
HEADSCALE_IMAGE=docker.io/headscale/headscale:0.29.4@sha256:8833f828b414c0907b7e5c71da76473216fe17cce0818a166b536ec552c0903f
URL=http://127.0.0.1:8080

case $(uname -m) in
x86_64) ARCH=amd64 ;;
aarch64 | arm64) ARCH=arm64 ;;
*) echo "unsupported architecture $(uname -m)" >&2 && exit 1 ;;
esac

# Checksums of tailscale_<version>_<arch>.tgz, from pkgs.tailscale.com
declare -A TAILSCALE_SHA256=(
	["1.104.7_amd64"]=e56693ff74a64f2591b7e819c280635a100aae1ddfb0ef058fcd052b7cd8d16c
	["1.104.7_arm64"]=d6bed2720310b328b861f943d7b4016c5358a9bad60575eb51e8143b79068484
)

PLUGIN=openbao-plugin-auth-tailscale
PLUGIN_BINARY=${PLUGIN_BINARY:-$ROOT/bin/${PLUGIN}_linux_$ARCH}
WORK=$(mktemp -d /tmp/e2e.XXXXXX)
NODES=(server caller other person)
PIDS=()
FAILED=0

log() { printf '\n=== %s\n' "$*"; }
fail() {
	echo "FAIL: $*" >&2
	exit 1
}

cleanup() {
	local status=$?
	if ((status != 0 || FAILED != 0)); then
		for node in "${NODES[@]}"; do
			echo "--- tailscaled ($node)"
			tail -n 40 "$WORK/$node/log" 2>/dev/null || true
		done
		echo "--- openbao"
		tail -n 60 "$WORK/openbao.log" 2>/dev/null || true
		echo "--- headscale"
		docker logs --tail 60 e2e-headscale 2>&1 || true
	fi
	# E2E_PAUSE=600 keeps everything running that long after a failure, to
	# look around; $WORK is printed above
	if ((status != 0 || FAILED != 0)) && [[ -n ${E2E_PAUSE:-} ]]; then
		echo "paused for ${E2E_PAUSE}s; work directory $WORK"
		sleep "$E2E_PAUSE"
	fi
	kill "${PIDS[@]}" 2>/dev/null || true
	docker rm -f e2e-headscale >/dev/null 2>&1 || true
	rm -rf "$WORK"
}
trap cleanup EXIT

retry() {
	local attempts=$1
	shift
	for ((i = 0; i < attempts; i++)); do
		"$@" && return 0
		sleep 1
	done
	return 1
}

# check DESCRIPTION WANT GOT: one result line; a mismatch fails the run at
# the end, so one run shows every failing case
check() {
	if [[ $2 == "$3" ]]; then
		printf 'ok    %s (%s)\n' "$1" "$3"
	else
		printf 'FAIL  %s: want %s, got %s\n' "$1" "$2" "$3"
		FAILED=1
	fi
}

[[ -x $PLUGIN_BINARY ]] || fail "no plugin at $PLUGIN_BINARY: run make build first"

log "Fetching Tailscale $TAILSCALE_VERSION and OpenBao $OPENBAO_VERSION"
tgz=tailscale_${TAILSCALE_VERSION}_$ARCH.tgz
sum=${TAILSCALE_SHA256[${TAILSCALE_VERSION}_$ARCH]:-}
[[ -n $sum ]] || fail "no checksum recorded for $tgz"
curl -fsSL -o "$WORK/$tgz" "https://pkgs.tailscale.com/stable/$tgz"
echo "$sum  $WORK/$tgz" | sha256sum -c - >/dev/null
tar -xzf "$WORK/$tgz" -C "$WORK" --strip-components=1 \
	"tailscale_${TAILSCALE_VERSION}_$ARCH/tailscale" "tailscale_${TAILSCALE_VERSION}_$ARCH/tailscaled"

bao_tgz=openbao_${OPENBAO_VERSION}_linux_$ARCH.tar.gz
release=https://github.com/openbao/openbao/releases/download/v$OPENBAO_VERSION
curl -fsSL -o "$WORK/$bao_tgz" "$release/$bao_tgz"
curl -fsSL -o "$WORK/checksums.txt" "$release/checksums.txt"
(cd "$WORK" && grep " $bao_tgz\$" checksums.txt | sha256sum -c - >/dev/null)
tar -xzf "$WORK/$bao_tgz" -C "$WORK" bao

log "Starting headscale at $URL"
docker rm -f e2e-headscale >/dev/null 2>&1 || true
docker run -d --name e2e-headscale --network host \
	--tmpfs /var/lib/headscale --tmpfs /var/run/headscale \
	-v "$HERE/headscale:/etc/headscale:ro" "$HEADSCALE_IMAGE" serve >/dev/null
retry 60 curl -fsS -o /dev/null "$URL/health" || fail "headscale did not come up"
hs() { docker exec e2e-headscale headscale "$@"; }
operator=$(hs users create operator -o json | jq -r .id)
person=$(hs users create person -o json | jq -r .id)

# ts NODE ARGS...: the tailscale CLI against one node's daemon
ts() { "$WORK/tailscale" --socket="$WORK/$1/sock" "${@:2}"; }

# node NAME PROXY_PORT USER [TAG]: a userspace tailscaled, logged in to
# headscale. A request sent through its HTTP proxy comes from this node.
node() {
	local name=$1 port=$2 user=$3 tag=${4:-}
	mkdir -p "$WORK/$name"
	"$WORK/tailscaled" --tun=userspace-networking --state=mem: --statedir="$WORK/$name" \
		--socket="$WORK/$name/sock" --port=0 --outbound-http-proxy-listen="127.0.0.1:$port" \
		>"$WORK/$name/log" 2>&1 &
	PIDS+=($!)
	echo $! >"$WORK/$name/pid"
	retry 30 test -S "$WORK/$name/sock" || fail "tailscaled ($name) did not start"
	local key
	key=$(hs preauthkeys create --user "$user" --expiration 1h ${tag:+--tags "$tag"} | tail -n 1)
	ts "$name" up --login-server="$URL" --auth-key="$key" --hostname="e2e-$name" --accept-dns=false
}

log "Starting four nodes"
node server 1051 "$operator" tag:example-server
node caller 1052 "$operator" tag:example-caller
node other 1053 "$operator" tag:example-caller
node person 1054 "$person"
server4=$(ts server ip -4)
server6=$(ts server ip -6)
caller4=$(ts caller ip -4)
caller6=$(ts caller ip -6)
echo "server $server4 $server6, caller $caller4 $caller6"

log "Starting OpenBao with the plugin"
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 1 \
	-keyout "$WORK/tls.key" -out "$WORK/tls.pem" -subj /CN=e2e-server 2>/dev/null
mkdir "$WORK/plugins"
cp "$PLUGIN_BINARY" "$WORK/plugins/$PLUGIN"
sed "s|@WORK@|$WORK|g" "$HERE/openbao.hcl" >"$WORK/openbao.hcl"
"$WORK/bao" server -config="$WORK/openbao.hcl" -log-level=debug >"$WORK/openbao.log" 2>&1 &
PIDS+=($!)
export VAULT_ADDR=http://127.0.0.1:18210
bao() { "$WORK/bao" "$@"; }
retry 30 curl -fsS -o /dev/null "$VAULT_ADDR/v1/sys/seal-status" || fail "OpenBao did not come up"
init=$(bao operator init -key-shares=1 -key-threshold=1 -format=json)
bao operator unseal "$(jq -r '.unseal_keys_b64[0]' <<<"$init")" >/dev/null
VAULT_TOKEN=$(jq -r .root_token <<<"$init")
export VAULT_TOKEN

bao plugin register -sha256="$(sha256sum "$WORK/plugins/$PLUGIN" | cut -d' ' -f1)" auth "$PLUGIN" >/dev/null
bao auth enable -path=tailscale "$PLUGIN" >/dev/null
bao write auth/tailscale/config socket="$WORK/server/sock" >/dev/null
bao write auth/tailscale/role/caller bound_tags=tag:example-caller token_ttl=10m >/dev/null
bao write auth/tailscale/role/server-only bound_tags=tag:example-server >/dev/null

log "Serving OpenBao on the server node"
# Plain HTTP on the proxy path: HTTPS would need a certificate for the node
ts server serve --bg --http=80 http://127.0.0.1:18202 >/dev/null
ts server serve --bg --tcp=8200 --proxy-protocol=2 tcp://127.0.0.1:18200 >/dev/null

# serve's HTTP handler answers for the node's name only, so that path is
# reached by name and the proxy picks the address family. The TCP path is
# reached by address, once per family.
HTTP_URL=http://e2e-server.example.test
TCP_URL4=https://$server4:8200
TCP_URL6=https://[$server6]:8200

# call NODE ARGS...: curl through NODE's proxy, printing the response body
# and, on the last line, the status code
call() {
	local node_port
	case $1 in
	caller) node_port=1052 ;;
	other) node_port=1053 ;;
	person) node_port=1054 ;;
	esac
	curl -sk --max-time 15 -x "http://127.0.0.1:$node_port" -w '\n%{http_code}' "${@:2}" || echo 000
}
status() { tail -n 1 <<<"$1"; }
# refused_for REASON: yes if OpenBao's log has a refusal for REASON since the
# last call. A 403 alone would also be the answer to a refusal for any other
# reason.
LOG_LINES=0
refused_for() {
	local found=no
	tail -n +$((LOG_LINES + 1)) "$WORK/openbao.log" | grep -q "refused:.*reason=.*$1" && found=yes
	LOG_LINES=$(wc -l <"$WORK/openbao.log")
	echo "$found"
}
body() { sed '$d' <<<"$1"; }
login() { call "$1" -X POST -d "{\"role\":\"$2\"}" "${@:4}" "$3/v1/auth/tailscale/login"; }
lookup() { call "$1" -H "X-Vault-Token: $2" "$3/v1/auth/token/lookup-self"; }

retry 30 bash -c "[[ \$(curl -s -o /dev/null -w '%{http_code}' -x http://127.0.0.1:1052 '$HTTP_URL/v1/sys/health') == 200 ]]" ||
	fail "the caller cannot reach OpenBao through the server node"

log "Logins"
# Guards: G1, G2, G3
r=$(login caller caller "$HTTP_URL")
check "login, HTTP proxy path" 200 "$(status "$r")"
token=$(body "$r" | jq -r .auth.client_token)
# The proxy chooses the address family on this path
address=$(body "$r" | jq -r .auth.metadata.address)
[[ $address == "$caller6" ]] && http_address=$caller6 || http_address=$caller4
check "the token names one of the caller's addresses" "$http_address" "$address"
check "the token names the caller's tag" tag:example-caller "$(body "$r" | jq -r .auth.metadata.tags)"

check "login, PROXY protocol path, IPv4" 200 "$(status "$(login caller caller "$TCP_URL4")")"
r=$(login caller caller "$TCP_URL6")
check "login, PROXY protocol path, IPv6" 200 "$(status "$r")"
check "an IPv6 login names the caller's IPv6 address" "$caller6" "$(body "$r" | jq -r .auth.metadata.address)"
refused_for . >/dev/null
check "login for a role bound to another tag" 403 "$(status "$(login caller server-only "$HTTP_URL")")"
check "... because the node lacks the tag" yes "$(refused_for 'node lacks a bound tag')"
check "login for a role that does not exist" 403 "$(status "$(login caller nope "$HTTP_URL")")"
check "... because there is no such role" yes "$(refused_for 'no such role')"
check "login from a node without tags" 403 "$(status "$(login person caller "$HTTP_URL")")"
check "... because the node has no tags" yes "$(refused_for 'node has no tags')"
# Guards: G6
check "the refused caller is told nothing more" '["permission denied"]' \
	"$(body "$(login person caller "$HTTP_URL")" | jq -c .errors)"

r=$(login caller caller "$HTTP_URL" -H 'X-Forwarded-For: 100.64.0.99')
check "login with a forged X-Forwarded-For" 200 "$(status "$r")"
check "a forged X-Forwarded-For does not change the address" "$http_address" "$(body "$r" | jq -r .auth.metadata.address)"

log "Token binding"
# Guards: G2
check "the token works from the caller, HTTP proxy path" 200 "$(status "$(lookup caller "$token" "$HTTP_URL")")"
check "the token works from the caller, IPv4" 200 "$(status "$(lookup caller "$token" "$TCP_URL4")")"
check "the token works from the caller, IPv6" 200 "$(status "$(lookup caller "$token" "$TCP_URL6")")"
check "the token fails from another node with the same tag" 403 "$(status "$(lookup other "$token" "$HTTP_URL")")"
check "the token fails from that node on the PROXY protocol path" 403 "$(status "$(lookup other "$token" "$TCP_URL4")")"

log "Renewal"
# Guards: G5
renew() { call caller -X POST -H "X-Vault-Token: $token" "$HTTP_URL/v1/auth/token/renew-self"; }
check "renewal from the caller" 200 "$(status "$(renew)")"
caller_id=$(hs nodes list -o json | jq -r '.[] | select(.given_name == "e2e-caller") | .id')
hs nodes tag --identifier "$caller_id" --tags tag:example-retired >/dev/null
tagged() { ts server whois --json "$caller4" | jq -e '.Node.Tags == ["tag:example-retired"]' >/dev/null; }
retry 30 tagged || fail "the server node never saw the caller's new tag"
refused_for . >/dev/null
check "renewal after the caller lost its tag" 403 "$(status "$(renew)")"
check "... because the node lacks the tag" yes "$(refused_for 'node lacks a bound tag')"
check "login after the caller lost its tag" 403 "$(status "$(login caller caller "$HTTP_URL")")"
check "... because the node lacks the tag" yes "$(refused_for 'node lacks a bound tag')"

log "Without tailscaled"
# Guards: G4
# The control: sent straight to the loopback listener, a forged address is
# believed. That is why only tailscaled may reach that listener (E2).
forged() {
	curl -s -o /dev/null -w '%{http_code}' --max-time 15 -X POST -H "X-Forwarded-For: $1" \
		-d '{"role":"caller"}' http://127.0.0.1:18202/v1/auth/tailscale/login
}
other4=$(ts other ip -4)
check "control: a forged address on the loopback listener is believed" 200 "$(forged "$other4")"
kill "$(cat "$WORK/server/pid")"
retry 10 bash -c "! kill -0 $(cat "$WORK/server/pid") 2>/dev/null" || fail "the server's tailscaled did not stop"
refused_for . >/dev/null
check "the same request once tailscaled is gone" 403 "$(forged "$other4")"
check "... because tailscaled cannot be asked" yes "$(refused_for 'ask tailscaled')"

if ((FAILED != 0)); then
	fail "at least one case failed"
fi
log "All cases passed"
