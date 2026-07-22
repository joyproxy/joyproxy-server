import socket

TARGET_HOST = "3ef44366.xq0.cn"
TARGET_PORT = 21394

payload = b"ping"

s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.settimeout(5)

s.sendto(payload, (TARGET_HOST, TARGET_PORT))
print("sent", len(payload), "bytes to", TARGET_HOST, TARGET_PORT)

try:
    data, addr = s.recvfrom(4096)
    print("recv", len(data), "from", addr)
    print(data[:64].hex())
except socket.timeout:
    print("timeout: no reply within 5s")

finally:
    s.close()
