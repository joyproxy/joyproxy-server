#!/bin/sh
# Read eipmap, start joyproxy (default ports 2829 + 3829)
# Runs setup-sps-v3.sh if eipmap is missing
#
# Usage: sh /root/start-sps-v3.sh

set -e

BIN="${JOYPROXY_BIN:-/root/joyproxy-linux-amd64-v2.2}"
PORTS="${PORTS:-2829 3829}"
EIPMAP="${EIPMAP:-/root/eipmap.txt}"
SETUP="${SETUP_SCRIPT:-/root/setup-sps-v3.sh}"
AUTH_URL="${AUTH_URL:-http://127.0.0.1:6301/get}"
TRAFFIC_URL="${TRAFFIC_URL:-http://127.0.0.1:6303/traffic}"
LOG="${JOYPROXY_LOG:-/root/joyproxy.stdout}"

if [ ! -x "$BIN" ]; then
    echo "start-sps-v3: binary not found: $BIN" >&2
    exit 1
fi

if [ ! -s "$EIPMAP" ] || [ "$FORCE_SETUP" = "1" ]; then
    echo "start-sps-v3: auto run setup-sps-v3.sh"
    sh "$SETUP"
fi

start_one() {
    _pub="$1"
    _priv="$2"
    _port="$3"
    _bind="$4"
    nohup "$BIN" sps -p "$_bind" -g "$_pub" \
        --auth-nouser \
        --auth-url "$AUTH_URL" \
        --traffic-url "$TRAFFIC_URL" \
        --auth-cache 0 \
        --max-conns-rate 0 \
        >> "$LOG" 2>&1 &
    echo "start-sps-v3: started pid=$! port=$_port bind=$_bind -g $_pub log=$LOG"
}

start_ports() {
    _pub="$1"
    _priv="$2"
    _single="$3"
    for _port in $PORTS; do
        if [ "$_single" = "1" ]; then
            start_one "$_pub" "$_priv" "$_port" ":${_port}"
        else
            start_one "$_pub" "$_priv" "$_port" "${_priv}:${_port}"
        fi
    done
}

_rows=$(wc -l < "$EIPMAP" | tr -d ' ')
_pub_cnt=$(awk '{print $1}' "$EIPMAP" | sort -u | wc -l | tr -d ' ')

echo "start-sps-v3: ports=$PORTS log=$LOG"

if [ "$_rows" -gt 1 ] && [ "$_pub_cnt" -gt 1 ]; then
    echo "start-sps-v3: MULTI ($_rows bindings x $(echo $PORTS | wc -w | tr -d ' ') ports)"
    while read -r _pub _priv; do
        [ -z "$_pub" ] && continue
        case "$_pub" in \#*) continue ;; esac
        start_ports "$_pub" "$_priv" 0
    done < "$EIPMAP"
else
    _pub=$(awk 'NR==1{print $1}' "$EIPMAP")
    _priv=$(awk 'NR==1{print $2}' "$EIPMAP")
    echo "start-sps-v3: SINGLE public=$_pub"
    start_ports "$_pub" "$_priv" 1
fi
