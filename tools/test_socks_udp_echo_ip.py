"""
经 SOCKS5 代理访问 udp_echo_client_ip_server，打印服务器看到的出口 IP。

先在一台机器上启动:
  python udp_echo_client_ip_server.py --port 21394
  # 目标 3ef44366.xq0.cn:21394

再在本机运行本脚本（改 PROXY / TARGET 为实际地址）。
"""
import socket
import socks

#PROXY_HOST = "117.89.88.141"
#PROXY_PORT = 6000
#PROXY_USER = ""
#PROXY_PASS = ""
PROXY_HOST = "117.89.88.123"
PROXY_PORT = 10001
PROXY_USER = "shouxin"
PROXY_PASS = "123456xJ"

TARGET_HOST = "3ef44366.xq0.cn"
TARGET_PORT = 21394
TIMEOUT_SEC = 5

s = socks.socksocket(socket.AF_INET, socket.SOCK_DGRAM)
if PROXY_USER or PROXY_PASS:
    s.set_proxy(socks.SOCKS5, PROXY_HOST, PROXY_PORT, True, PROXY_USER, PROXY_PASS)
else:
    s.set_proxy(socks.SOCKS5, PROXY_HOST, PROXY_PORT, True)
s.settimeout(TIMEOUT_SEC)

print(f"proxy socks5://{PROXY_HOST}:{PROXY_PORT}")
print(f"target udp://{TARGET_HOST}:{TARGET_PORT}")
s.sendto(b"whoami", (TARGET_HOST, TARGET_PORT))

try:
    data, addr = s.recvfrom(4096)
    print("reply from", addr)
    print(data.decode("utf-8", errors="replace").strip())
except socket.timeout:
    print(f"timeout ({TIMEOUT_SEC}s)")

s.close()
