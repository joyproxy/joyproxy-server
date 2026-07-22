"""
经 SOCKS5 代理测试 UDP：117.89.88.151:5500 -> 3ef44366.xq0.cn:21394

白名单（--auth-nouser）时不要传 USER/PASS。
需要账密时填写 PROXY_USER / PROXY_PASS。
"""
import socket
import socks

PROXY_HOST = "117.89.88.151"
PROXY_PORT = 5500
PROXY_USER = ""
PROXY_PASS = ""

TARGET_HOST = "3ef44366.xq0.cn"
TARGET_PORT = 21394
PAYLOAD = b"ping"
TIMEOUT_SEC = 5

s = socks.socksocket(socket.AF_INET, socket.SOCK_DGRAM)
if PROXY_USER or PROXY_PASS:
    s.set_proxy(socks.SOCKS5, PROXY_HOST, PROXY_PORT, True, PROXY_USER, PROXY_PASS)
else:
    s.set_proxy(socks.SOCKS5, PROXY_HOST, PROXY_PORT, True)
s.settimeout(TIMEOUT_SEC)

print(f"proxy socks5://{PROXY_HOST}:{PROXY_PORT} -> udp {TARGET_HOST}:{TARGET_PORT}")
s.sendto(PAYLOAD, (TARGET_HOST, TARGET_PORT))
print("sent", len(PAYLOAD), "bytes")

try:
    data, addr = s.recvfrom(4096)
    print("recv", len(data), "from", addr)
    print(data[:64].hex())
except socket.timeout:
    print(f"timeout: no reply within {TIMEOUT_SEC}s")

s.close()
