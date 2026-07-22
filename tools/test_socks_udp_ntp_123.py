"""
经 SOCKS5 代理访问 NTP（UDP 123）。

依赖: pip install PySocks
"""
import datetime
import socket
import struct
import socks

PROXY_HOST = "221.225.50.55"
PROXY_PORT = 2829
PROXY_USER = ""
PROXY_PASS = ""

NTP_HOST = "ntp.aliyun.com"
NTP_PORT = 123
TIMEOUT_SEC = 8

# NTP v3 client request, 48 bytes
ntp_request = b"\x1b" + (47 * b"\0")

s = socks.socksocket(socket.AF_INET, socket.SOCK_DGRAM)
s.set_proxy(socks.SOCKS5, PROXY_HOST, PROXY_PORT, True, PROXY_USER, PROXY_PASS)
s.settimeout(TIMEOUT_SEC)

print(f"proxy socks5://{PROXY_USER}@{PROXY_HOST}:{PROXY_PORT}")
print(f"ntp udp -> {NTP_HOST}:{NTP_PORT}")

s.sendto(ntp_request, (NTP_HOST, NTP_PORT))
print("sent", len(ntp_request), "bytes")

try:
    data, addr = s.recvfrom(4096)
    print("recv", len(data), "from", addr)
    if len(data) >= 48:
        ntp_sec = struct.unpack("!I", data[40:44])[0]
        unix_sec = ntp_sec - 2208988800
        print("ntp time (UTC):", datetime.datetime.fromtimestamp(unix_sec, datetime.timezone.utc))
    else:
        print("short reply:", data.hex())
except socket.timeout:
    print(f"timeout ({TIMEOUT_SEC}s)")

s.close()
