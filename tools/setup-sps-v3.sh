#!/bin/sh
# 1) Detect single/multi egress, write /root/eipmap.txt
# 2) Fetch iplist.txt users.txt under /root/auth/<public-ip>/
#
# API uses REMOTE_ADDR only (no URL public-ip param).
# Multi: curl -4 --interface <private-ip>; REMOTE_ADDR = that line public IP (OnlyIp check)
#
# Usage: sh /root/setup-sps-v3.sh

set -e

AUTH_BASE="${AUTH_BASE:-/root/auth}"
EIPMAP="${EIPMAP:-/root/eipmap.txt}"
API_BASE="${API_BASE:-http://118.178.178.30:2808/VAD/Pwc.aspx}"
PROBE_URL="${PROBE_URL:-http://118.178.178.30:2808/VAD/OnlyIp.aspx?yyy=ip}"
ROUTE_TARGET="${ROUTE_TARGET:-114.114.114.114}"
# Unbound ifaces: OnlyIp probe may hang; tune PROBE_* env vars
PROBE_CONNECT_TIMEOUT="${PROBE_CONNECT_TIMEOUT:-2}"
PROBE_MAX_TIME="${PROBE_MAX_TIME:-5}"
FETCH_CONNECT_TIMEOUT="${FETCH_CONNECT_TIMEOUT:-3}"
FETCH_MAX_TIME="${FETCH_MAX_TIME:-15}"

mkdir -p "$AUTH_BASE"
[ -f "${AUTH_BASE}/denykey.txt" ] || touch "${AUTH_BASE}/denykey.txt"

is_private_ip() {
    case "$1" in
        10.*|172.1[6-9].*|172.2[0-9].*|172.3[0-1].*|192.168.*|127.*|169.254.*) return 0 ;;
        *) return 1 ;;
    esac
}

public_on_nic() {
    for _ip in $(ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1); do
        if ! is_private_ip "$_ip"; then
            echo "$_ip"
            return
        fi
    done
}

list_192168() {
    for _ip in $(ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1); do
        case "$_ip" in 192.168.*) echo "$_ip" ;; esac
    done | sort -u
}

route_src() {
    ip route get "$ROUTE_TARGET" 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src"){print $(i+1);exit}}'
}

# No elastic public on iface: skip fast if route from fails
iface_route_ok() {
    _priv="$1"
    [ -n "$_priv" ] && [ "$_priv" != "0.0.0.0" ] || return 0
    ip route get "$ROUTE_TARGET" from "$_priv" >/dev/null 2>&1
}

curl_probe() {
    _priv="$1"
    _url="$2"
    _opts="-4 -s --connect-timeout ${PROBE_CONNECT_TIMEOUT} --max-time ${PROBE_MAX_TIME}"
    if [ -n "$_priv" ] && [ "$_priv" != "0.0.0.0" ]; then
        if command -v timeout >/dev/null 2>&1; then
            timeout "$((PROBE_MAX_TIME + 2))" curl $_opts --interface "$_priv" "$_url" 2>/dev/null
        else
            curl $_opts --interface "$_priv" "$_url" 2>/dev/null
        fi
    else
        if command -v timeout >/dev/null 2>&1; then
            timeout "$((PROBE_MAX_TIME + 2))" curl $_opts "$_url" 2>/dev/null
        else
            curl $_opts "$_url" 2>/dev/null
        fi
    fi
}

api_url() {
    echo "${API_BASE}?act=$1"
}

query_public() {
    _priv="$1"
    if [ -n "$_priv" ] && [ "$_priv" != "0.0.0.0" ]; then
        if ! iface_route_ok "$_priv"; then
            echo ""
            return 0
        fi
        _raw=$(curl_probe "$_priv" "$PROBE_URL" | tr -d '\r\n ')
    else
        _raw=$(curl_probe "" "$PROBE_URL" | tr -d '\r\n ')
    fi
    _pub=$(echo "$_raw" | grep -Eo '^([0-9]{1,3}\.){3}[0-9]{1,3}$' | head -n 1)
    [ -z "$_pub" ] && _pub=$(echo "$_raw" | grep -Eo '([0-9]{1,3}\.){3}[0-9]{1,3}' | head -n 1)
    echo "$_pub"
}

curl_via_iface() {
    _priv="$1"
    _url="$2"
    _opts="-4 -s --connect-timeout ${FETCH_CONNECT_TIMEOUT} --max-time ${FETCH_MAX_TIME}"
    if [ -n "$_priv" ] && [ "$_priv" != "0.0.0.0" ]; then
        if command -v timeout >/dev/null 2>&1; then
            timeout "$((FETCH_MAX_TIME + 2))" curl $_opts --interface "$_priv" "$_url" 2>/dev/null
        else
            curl $_opts --interface "$_priv" "$_url" 2>/dev/null
        fi
    else
        curl $_opts "$_url" 2>/dev/null
    fi
}

print_baseline() {
    _def_pub=$(query_public "")
    _def_user=$(curl_via_iface "" "$(api_url sps_cxuser)")
    echo "setup-sps-v3: === baseline (no --interface, API REMOTE_ADDR) ==="
    echo "  OnlyIp (REMOTE_ADDR) = ${_def_pub:-<empty>}"
    echo "  sps_cxuser:"
    if [ -n "$_def_user" ]; then
        echo "$_def_user" | sed 's/^/    /'
    else
        echo "    <empty>"
    fi
    echo ""
}

fetch_auth_files() {
    _pub="$1"
    _priv="$2"
    _dir="${AUTH_BASE}/${_pub}"
    mkdir -p "$_dir"

    _seen=$(query_public "$_priv")
    if [ -n "$_seen" ] && [ "$_seen" != "$_pub" ]; then
        echo "  WARN egress mismatch: expect $_pub OnlyIp got $_seen (priv $_priv)" >&2
    else
        echo "  fetch $_pub via $_priv (REMOTE_ADDR/OnlyIp=$_seen)"
    fi

    _url_ip=$(api_url "sps_cxip")
    _url_user=$(api_url "sps_cxuser")
    curl_via_iface "$_priv" "$_url_ip" > "${_dir}/iplist.txt"
    _user_body=$(curl_via_iface "$_priv" "$_url_user")
    printf '%s' "$_user_body" > "${_dir}/users.txt"

    echo "  --- sps_cxuser url=$_url_user"
    if [ -n "$_user_body" ]; then
        echo "$_user_body" | sed 's/^/  --- /'
    else
        echo "  --- <empty>"
    fi
    echo ""
}

build_eipmap() {
    _tmp="${EIPMAP}.tmp.$$"
    : > "$_tmp"
    _nic_pub=$(public_on_nic)

    if [ -n "$_nic_pub" ]; then
        _priv=$(route_src)
        echo "$_nic_pub ${_priv:-0.0.0.0}" >> "$_tmp"
        echo "setup-sps-v3: NIC public $_nic_pub (single)"
    else
        _privs=$(list_192168)
        _cnt=0
        for _p in $_privs; do _cnt=$((_cnt + 1)); done

        if [ "$_cnt" -le 1 ]; then
            _priv=$(echo "$_privs" | head -n 1)
            [ -z "$_priv" ] && _priv=$(route_src)
            _pub=""
            if [ -n "$_priv" ] && [ "$_priv" != "0.0.0.0" ]; then
                _pub=$(query_public "$_priv")
            fi
            [ -z "$_pub" ] && _pub=$(route_src)
            echo "$_pub $_priv" >> "$_tmp"
            echo "setup-sps-v3: <=1 private 192.168.* -> single public $_pub"
        else
            echo "setup-sps-v3: $_cnt private 192.168.* -> probe OnlyIp (skip unbound ifaces)"
            _ok=0
            _skip=0
            for _priv in $_privs; do
                printf "setup-sps-v3:   probe %s ... " "$_priv" >&2
                if [ -n "$_priv" ] && [ "$_priv" != "0.0.0.0" ] && ! iface_route_ok "$_priv"; then
                    echo "skip (no route)" >&2
                    _skip=$((_skip + 1))
                    continue
                fi
                _pub=$(query_public "$_priv")
                if [ -n "$_pub" ]; then
                    echo "$_pub" >&2
                    echo "$_pub $_priv" >> "$_tmp"
                    _ok=$((_ok + 1))
                else
                    echo "skip (OnlyIp empty/timeout)" >&2
                    _skip=$((_skip + 1))
                fi
            done
            echo "setup-sps-v3: OnlyIp matched ${_ok}, skipped ${_skip}" >&2
        fi
    fi

    sort -u "$_tmp" | awk 'NF>=1 && $1!="" && !seen[$1]++' > "$EIPMAP"
    rm -f "$_tmp"
    [ -s "$EIPMAP" ] || { echo "setup-sps-v3: eipmap empty" >&2; exit 1; }
}

check_duplicate_auth() {
    _rows=$(wc -l < "$EIPMAP" | tr -d ' ')
    _pub_cnt=$(awk '{print $1}' "$EIPMAP" | sort -u | wc -l | tr -d ' ')
    [ "$_pub_cnt" -le 1 ] && return 0

    _umd5=$(md5sum "${AUTH_BASE}"/*/users.txt 2>/dev/null | awk '{print $1}' | sort -u | wc -l | tr -d ' ')
    _imd5=$(md5sum "${AUTH_BASE}"/*/iplist.txt 2>/dev/null | awk '{print $1}' | sort -u | wc -l | tr -d ' ')
    echo "setup-sps-v3: users.txt distinct md5=${_umd5}/${_rows}  iplist.txt distinct md5=${_imd5}/${_rows}"
    if [ "$_umd5" = "1" ] && [ "$_rows" -gt 1 ]; then
        echo "setup-sps-v3: NOTE all users.txt identical - may be normal per REMOTE_ADDR" >&2
        echo "setup-sps-v3: NOTE without --interface REMOTE_ADDR is default route; see baseline" >&2
    fi
}

echo "setup-sps-v3: === build eipmap ==="
build_eipmap
cat "$EIPMAP"

echo "setup-sps-v3: === fetch auth files ==="
print_baseline
while read -r _pub _priv; do
    [ -z "$_pub" ] && continue
    case "$_pub" in \#*) continue ;; esac
    fetch_auth_files "$_pub" "$_priv"
done < "$EIPMAP"

check_duplicate_auth

_rows=$(wc -l < "$EIPMAP" | tr -d ' ')
_pub_cnt=$(awk '{print $1}' "$EIPMAP" | sort -u | wc -l | tr -d ' ')
if [ "$_rows" -gt 1 ] && [ "$_pub_cnt" -gt 1 ]; then
    echo "setup-sps-v3: result=multi-egress (${_pub_cnt} public IPs)"
else
    echo "setup-sps-v3: result=single-egress (public $(awk 'NR==1{print $1}' "$EIPMAP"))"
fi
echo "setup-sps-v3: done. kill -HUP \$(pgrep -f minipwa.pl) after edit denykey"
