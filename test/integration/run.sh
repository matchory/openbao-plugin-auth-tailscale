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

OPENBAO_VERSION=${OPENBAO_VERSION:-2.7.3}
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
# Checksums of openbao_<version>_linux_<arch>.tar.gz, from the release's
# checksums.txt: recorded here so a changed download fails instead of
# being checked against a list fetched from the same place
declare -A OPENBAO_SHA256=(
	["2.6.6_amd64"]=9df0ae1f5e15c8b5ad0a61be930c7d0e5d3e507ce9cbd912c161463f5130e644
	["2.6.6_arm64"]=5eda3fd7cff9d7e4e1cc21525b9919ecc59ffd69f66f7c391deca4f106734525
	["2.7.3_amd64"]=131df9a21cf3dd2dff47107110b369c9974cbdaa482bcbc723b81b3403c68ecd
	["2.7.3_arm64"]=b505b07af4206fdf0e81b747fcb7e9e63f2c2b05e26ead973cce62d0ed07d0e0
)

PLUGIN=openbao-plugin-auth-tailscale
PLUGIN_BINARY=${PLUGIN_BINARY:-$ROOT/bin/${PLUGIN}_linux_$ARCH}
WORK=$(mktemp -d /tmp/e2e.XXXXXX)
# Each node's outbound HTTP proxy port, filled in as the nodes start
declare -A PORT=()
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
		for node in "${!PORT[@]}"; do
			echo "--- tailscaled ($node)"
			tail -n 40 "$WORK/$node/log" 2>/dev/null || true
		done
		echo "--- openbao (openbao1)"
		tail -n 60 "$WORK/openbao.log" 2>/dev/null || true
		echo "--- openbao (openbao2)"
		tail -n 40 "$WORK/openbao2.log" 2>/dev/null || true
		echo "--- headscale"
		docker logs --tail 60 e2e-headscale 2>&1 || true
		# E2E_PAUSE=600 keeps everything running that long after a failure,
		# to look around
		if [[ -n ${E2E_PAUSE:-} ]]; then
			echo "paused for ${E2E_PAUSE}s; work directory $WORK"
			sleep "$E2E_PAUSE"
		fi
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
bao_sum=${OPENBAO_SHA256[${OPENBAO_VERSION}_$ARCH]:-}
[[ -n $bao_sum ]] || fail "no checksum recorded for $bao_tgz"
curl -fsSL -o "$WORK/$bao_tgz" "https://github.com/openbao/openbao/releases/download/v$OPENBAO_VERSION/$bao_tgz"
echo "$bao_sum  $WORK/$bao_tgz" | sha256sum -c - >/dev/null
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
	PORT[$name]=$port
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

log "Starting five nodes"
node server 1051 "$operator" tag:example-server
node caller 1052 "$operator" tag:example-caller
node other 1053 "$operator" tag:example-caller
node person 1054 "$person"
node server2 1055 "$operator" tag:example-server
server4=$(ts server ip -4)
server6=$(ts server ip -6)
caller4=$(ts caller ip -4)
caller6=$(ts caller ip -6)
echo "server $server4 $server6, caller $caller4 $caller6"

log "Starting an OpenBao cluster of two nodes with the plugin"
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 1 \
	-keyout "$WORK/tls.key" -out "$WORK/tls.pem" -subj /CN=e2e-server 2>/dev/null
mkdir "$WORK/plugins"
cp "$PLUGIN_BINARY" "$WORK/plugins/$PLUGIN"
# openbao NAME P JOIN LOG: one node of the cluster, its ports starting with P
openbao() {
	mkdir -p "$WORK/$1"
	sed -e "s|@WORK@|$WORK|g" -e "s|@NODE@|$1|g" -e "s|@P@|$2|g" -e "s|# @JOIN@|$3|" \
		"$HERE/openbao.hcl" >"$WORK/$1.hcl"
	"$WORK/bao" server -config="$WORK/$1.hcl" -log-level=debug >"$WORK/$4" 2>&1 &
	PIDS+=($!)
	retry 30 curl -fsS -o /dev/null "http://127.0.0.1:${2}8210/v1/sys/seal-status" || fail "OpenBao ($1) did not come up"
}
openbao openbao1 1 "" openbao.log
openbao openbao2 2 'retry_join { leader_api_addr = "http://127.0.0.1:18210" }' openbao2.log
export VAULT_ADDR=http://127.0.0.1:18210
VAULT2=http://127.0.0.1:28210
bao() { "$WORK/bao" "$@"; }
init=$(bao operator init -key-shares=1 -key-threshold=1 -format=json)
unseal_key=$(jq -r '.unseal_keys_b64[0]' <<<"$init")
bao operator unseal "$unseal_key" >/dev/null
VAULT_TOKEN=$(jq -r .root_token <<<"$init")
export VAULT_TOKEN
active() { [[ $(curl -s "http://127.0.0.1:${1}8210/v1/sys/leader" | jq -r .is_self) == true ]]; }
retry 30 active 1 || fail "openbao1 did not become the active node"
# The second node joins once it is unsealed, and then follows the first
retry 30 bash -c "VAULT_ADDR=$VAULT2 '$WORK/bao' operator unseal '$unseal_key' >/dev/null" ||
	fail "openbao2 could not be unsealed"
standby() { [[ $(curl -s -o /dev/null -w '%{http_code}' "$VAULT2/v1/sys/health") == 429 ]]; }
retry 60 standby || fail "openbao2 did not become a standby"
# It joins as a non-voter; autopilot promotes it once it has been stable,
# and only a voter can take over as the active node
voter() {
	bao operator raft list-peers -format=json |
		jq -e '.data.config.servers[] | select(.node_id == "openbao2") | .voter' >/dev/null
}
retry 60 voter || fail "openbao2 never became a voter"

bao plugin register -sha256="$(sha256sum "$WORK/plugins/$PLUGIN" | cut -d' ' -f1)" auth "$PLUGIN" >/dev/null
bao auth enable -path=tailscale "$PLUGIN" >/dev/null
bao write auth/tailscale/config socket="$WORK/server/sock" >/dev/null
bao write auth/tailscale/role/caller bound_tags=tag:example-caller token_ttl=10m >/dev/null
bao write auth/tailscale/role/server-only bound_tags=tag:example-server >/dev/null

log "Serving OpenBao on the server node"
# Plain HTTP on the proxy path: HTTPS would need a certificate for the node
ts server serve --bg --http=80 http://127.0.0.1:18202 >/dev/null
ts server serve --bg --tcp=8200 --proxy-protocol=2 tcp://127.0.0.1:18200 >/dev/null
# The standby, on a node of its own
ts server2 serve --bg --http=80 http://127.0.0.1:28202 >/dev/null

# serve's HTTP handler answers for the node's name only, so that path is
# reached by name and the proxy picks the address family. The TCP path is
# reached by address, once per family.
HTTP_URL=http://e2e-server.example.test
STANDBY_URL=http://e2e-server2.example.test
TCP_URL4=https://$server4:8200
TCP_URL6=https://[$server6]:8200

# call NODE ARGS...: curl through NODE's proxy, printing the response body
# and, on the last line, the status code
call() {
	curl -sk --max-time 15 -x "http://127.0.0.1:${PORT[$1]}" -w '\n%{http_code}' "${@:2}" || echo 000
}
status() { tail -n 1 <<<"$1"; }
# refused_for REASON: yes if either OpenBao node's log has a refusal for
# REASON since the last call. A 403 alone would also be the answer to a
# refusal for any other reason.
declare -A LOG_LINES=(["openbao.log"]=0 ["openbao2.log"]=0)
refused_for() {
	local found=no log
	for log in openbao.log openbao2.log; do
		tail -n +$((LOG_LINES[$log] + 1)) "$WORK/$log" | grep -q "refused:.*reason=.*$1" && found=yes
		LOG_LINES[$log]=$(wc -l <"$WORK/$log")
	done
	echo "$found"
}
body() { sed '$d' <<<"$1"; }
login() { call "$1" -X POST -d "{\"role\":\"$2\"}" "${@:4}" "$3/v1/auth/tailscale/login"; }
lookup() { call "$1" -H "X-Vault-Token: $2" "$3/v1/auth/token/lookup-self"; }

ready() { [[ $(status "$(call caller "$HTTP_URL/v1/sys/health")") == 200 ]]; }
retry 30 ready || fail "the caller cannot reach OpenBao through the server node"

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
# Every local process on the OpenBao host sends from this node
check "login from the server node, for its own tag" 403 "$(status "$(login server server-only "$HTTP_URL")")"
check "... because it is the node tailscaled runs as" yes "$(refused_for 'node is the one tailscaled runs as')"
# Guards: G6
check "the refused caller is told nothing more" '["permission denied"]' \
	"$(body "$(login person caller "$HTTP_URL")" | jq -c .errors)"

r=$(login caller caller "$HTTP_URL" -H 'X-Forwarded-For: 100.64.0.99')
check "login with a forged X-Forwarded-For" 200 "$(status "$r")"
check "a forged X-Forwarded-For does not change the address" "$http_address" "$(body "$r" | jq -r .auth.metadata.address)"

log "Rate limits"
# A quota can name one role, because the login path resolves its role for
# OpenBao. It counts every login for the role, refused or not.
bao write sys/quotas/rate-limit/caller-logins path=auth/tailscale/ role=caller rate=1 interval=1m >/dev/null
check "the first login under a quota of one a minute" 200 "$(status "$(login caller caller "$HTTP_URL")")"
check "the second login under that quota" 429 "$(status "$(login caller caller "$HTTP_URL")")"
# Refused for its tag, so it reached the plugin: the quota let it through
check "a login for another role is not counted against it" 403 "$(status "$(login caller server-only "$HTTP_URL")")"
bao delete sys/quotas/rate-limit/caller-logins >/dev/null
# With a quota for the mount too, the one for the role is what applies
bao write sys/quotas/rate-limit/caller-logins path=auth/tailscale/ role=caller rate=5 interval=1m >/dev/null
bao write sys/quotas/rate-limit/all-logins path=auth/tailscale/ rate=1 interval=1m >/dev/null
check "a login under a role quota and a stricter mount quota" 200 "$(status "$(login caller caller "$HTTP_URL")")"
check "another, still within the role quota" 200 "$(status "$(login caller caller "$HTTP_URL")")"
bao delete sys/quotas/rate-limit/caller-logins >/dev/null
bao delete sys/quotas/rate-limit/all-logins >/dev/null

log "Token binding"
# Guards: G2
check "the token works from the caller, HTTP proxy path" 200 "$(status "$(lookup caller "$token" "$HTTP_URL")")"
check "the token works from the caller, IPv4" 200 "$(status "$(lookup caller "$token" "$TCP_URL4")")"
check "the token works from the caller, IPv6" 200 "$(status "$(lookup caller "$token" "$TCP_URL6")")"
check "the token fails from another node with the same tag" 403 "$(status "$(lookup other "$token" "$HTTP_URL")")"
check "the token fails from that node on the PROXY protocol path" 403 "$(status "$(lookup other "$token" "$TCP_URL4")")"
check "the token fails off the tailnet" 403 \
	"$(curl -s -o /dev/null -w '%{http_code}' -H "X-Vault-Token: $token" "$VAULT_ADDR/v1/auth/token/lookup-self")"

log "Cluster"
# Guards: G1
# A login through the standby keeps the address the standby was told. Both
# nodes' plugins ask the server node's tailscaled here, as the config is the
# cluster's and every node of this test runs on one host.
other4=$(ts other ip -4)
other6=$(ts other ip -6)
r=$(login other caller "$STANDBY_URL")
check "login through the standby" 200 "$(status "$r")"
address=$(body "$r" | jq -r .auth.metadata.address)
check "... names the caller's own address" yes "$([[ $address == "$other4" || $address == "$other6" ]] && echo yes || echo no)"
refused_for . >/dev/null
# The server node's tailscaled doesn't run as the standby's node, as the
# active node's wouldn't in a cluster that forwards logins. The role binds
# only tags the OpenBao nodes carry.
check "login from the standby's own node, for its tag" 403 "$(status "$(login server2 server-only "$STANDBY_URL")")"
check "... because the role binds only the OpenBao nodes' tags" yes "$(refused_for 'tags the OpenBao node carries')"

# step_down P: until the node whose ports start with P is active
step_down() {
	local from=$((3 - $1))
	for _ in 1 2 3 4 5; do
		VAULT_ADDR=http://127.0.0.1:${from}8210 bao operator step-down >/dev/null 2>&1 || true
		retry 10 active "$1" && return 0
	done
	return 1
}
step_down 2 || fail "openbao2 never became the active node"
check "login through the new active node" 200 "$(status "$(login other caller "$STANDBY_URL")")"
check "login through the node that stepped down" 200 "$(status "$(login other caller "$HTTP_URL")")"
# And back, so the rest of the test runs against the cluster as it started
step_down 1 || fail "openbao1 never became the active node again"

log "Renewal"
# Guards: G5
renew() { call caller -X POST -H "X-Vault-Token: $token" "$HTTP_URL/v1/auth/token/renew-self"; }
check "renewal from the caller" 200 "$(status "$(renew)")"
bao write auth/tailscale/role/caller token_policies=example >/dev/null
refused_for . >/dev/null
check "renewal after the role's policies changed" 403 "$(status "$(renew)")"
check "... because the role no longer grants them" yes "$(refused_for 'role no longer grants')"
bao write auth/tailscale/role/caller token_policies= >/dev/null
check "renewal once the policies are back" 200 "$(status "$(renew)")"
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
check "control: a forged address on the loopback listener is believed" 200 "$(forged "$other4")"
refused_for . >/dev/null
check "login from an address no node has" 403 "$(forged 100.64.0.99)"
check "... because tailscaled knows no such node" yes "$(refused_for 'no node has this address')"

# Guards: G3
# A subnet router's clients arrive from the router's address. The policy
# grants the server node no access to the route, so headscale leaves it out
# of the router's AllowedIPs; only the router's own report names it.
ts other set --advertise-routes=10.99.0.0/24
other_id=$(hs nodes list -o json | jq -r '.[] | select(.given_name == "e2e-other") | .id')
hs nodes approve-routes --identifier "$other_id" --routes 10.99.0.0/24 >/dev/null
routes() { ts server whois --json "$other4" | jq -e '.Node.Hostinfo.RoutableIPs | index("10.99.0.0/24")' >/dev/null; }
retry 30 routes || fail "the server node never saw the other node's route"
refused_for . >/dev/null
check "login from a subnet router" 403 "$(status "$(login other caller "$HTTP_URL")")"
check "... because the node routes for other addresses" yes "$(refused_for 'node routes for other addresses')"

# Guards: G4
kill "$(cat "$WORK/server/pid")"
retry 10 bash -c "! kill -0 $(cat "$WORK/server/pid") 2>/dev/null" || fail "the server's tailscaled did not stop"
refused_for . >/dev/null
check "the same request once tailscaled is gone" 403 "$(forged "$other4")"
check "... because tailscaled cannot be asked" yes "$(refused_for 'ask tailscaled')"
check "renewal once tailscaled is gone" 403 \
	"$(curl -s -o /dev/null -w '%{http_code}' --max-time 15 -X POST -H "X-Forwarded-For: $caller4" \
		-H "X-Vault-Token: $token" http://127.0.0.1:18202/v1/auth/token/renew-self)"
check "... because tailscaled cannot be asked" yes "$(refused_for 'ask tailscaled')"

if ((FAILED != 0)); then
	fail "at least one case failed"
fi
log "All cases passed"
