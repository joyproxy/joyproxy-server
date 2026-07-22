#!/bin/sh
# 自动读 eipmap，判断单/多出口并启动 joyproxy（无 eipmap 时自动跑 setup-sps.sh）
#
# Usage: sh /root/start-sps.sh

set -e

BIN="${JOYPROXY_BIN:-/root/joyproxy-linux-amd64-v2.2}"
PORT="${PORT:-3829}"
EIPMAP="${EIPMAP:-/root/eipmap.txt}"
SETUP="${SETUP_SCRIPT:-/root/setup-sps.sh}"
AUTH_URL="${AUTH_URL:-http://127.0.0.1:6301/get}"
TRAFFIC_URL="${TRAFFIC_URL:-http://127.0.0.1:6303/traffic}"

if [ ! -x "$BIN" ]; then
    echo "start-sps: binary not found: $BIN" >&2
    exit 1
fi

if [ ! -s "$EIPMAP" ] || [ "$FORCE_SETUP" = "1" ]; then
    echo "start-sps: auto run setup-sps.sh"
    sh "$SETUP"
fi

start_one() {
    _pub="$1"
    _priv="$2"
    _bind="$3"
    if [ "$_bind" = ":${PORT}" ]; then
        _log="/root/joyproxy.stdout"
    else
        _log="/root/joyproxy-${_priv}-${PORT}.stdout"
    fi
    nohup "$BIN" sps -p "$_bind" -g "$_pub" \
        --auth-nouser \
        --auth-url "$AUTH_URL" \
        --traffic-url "$TRAFFIC_URL" \
        --auth-cache 0 \
        --max-conns-rate 0 \
        >> "$_log" 2>&1 &
    echo "started pid=$! bind=$_bind -g $_pub log=$_log"
}

_rows=$(wc -l < "$EIPMAP" | tr -d ' ')
_pub_cnt=$(awk '{print $1}' "$EIPMAP" | sort -u | wc -l | tr -d ' ')

if [ "$_rows" -gt 1 ] && [ "$_pub_cnt" -gt 1 ]; then
    echo "start-sps: MULTI ($_rows bindings)"
    while read -r _pub _priv; do
        [ -z "$_pub" ] && continue
        case "$_pub" in \#*) continue ;; esac
        start_one "$_pub" "$_priv" "${_priv}:${PORT}"
    done < "$EIPMAP"
else
    _pub=$(awk 'NR==1{print $1}' "$EIPMAP")
    _priv=$(awk 'NR==1{print $2}' "$EIPMAP")
    echo "start-sps: SINGLE public=$_pub"
    start_one "$_pub" "$_priv" ":${PORT}"
fi
