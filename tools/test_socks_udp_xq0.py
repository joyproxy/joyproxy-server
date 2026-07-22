import socket
import socks

PROXY = "bj.edge.xiequ.cn"
PORT = 10592
USER = "parker20201899"
PASS = "test11199"

TARGET_HOST = "3ef44366.xq0.cn"
TARGET_PORT = 21394

# 按实际 UDP 服务协议修改载荷；未知协议时可先发探测包
payload = b"ping"

s = socks.socksocket(socket.AF_INET, socket.SOCK_DGRAM)
s.set_proxy(socks.SOCKS5, PROXY, PORT, True, USER, PASS)
s.settimeout(5)

s.sendto(payload, (TARGET_HOST, TARGET_PORT))
print("sent", len(payload), "bytes to", TARGET_HOST, TARGET_PORT)

try:
    data, addr = s.recvfrom(4096)
    print("recv", len(data), "from", addr)
    print(data[:64].hex())
except socket.timeout:
    print("timeout: no reply within 5s (server may be silent or payload wrong)")
