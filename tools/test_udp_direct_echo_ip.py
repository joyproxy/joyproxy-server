"""直连 UDP echo 服务（对照组，不走代理）。"""
import socket

TARGET_HOST = "3ef44366.xq0.cn"
TARGET_PORT = 21394
TIMEOUT_SEC = 5

s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(TIMEOUT_SEC)

print(f"direct udp -> {TARGET_HOST}:{TARGET_PORT}")
s.sendto(b"whoami", (TARGET_HOST, TARGET_PORT))

try:
    data, addr = s.recvfrom(4096)
    print("reply from", addr)
    print(data.decode("utf-8", errors="replace").strip())
except socket.timeout:
    print(f"timeout ({TIMEOUT_SEC}s)")

s.close()
