#!/usr/bin/env python3
"""
UDP 回显服务：把「服务器看到的访问者地址」发回客户端。

用法（在要当目标的机器上，例如 3ef44366.xq0.cn 那台或内网测试机）:
  python udp_echo_client_ip_server.py
  python udp_echo_client_ip_server.py --host 0.0.0.0 --port 21394
"""
from __future__ import annotations

import argparse
import socket
import sys
from datetime import datetime, timezone


def main() -> None:
    p = argparse.ArgumentParser(description="UDP echo server: reply with client source address")
    p.add_argument("--host", default="0.0.0.0", help="listen address (default 0.0.0.0)")
    p.add_argument("--port", type=int, default=21394, help="listen port (default 21394)")
    args = p.parse_args()

    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    try:
        sock.bind((args.host, args.port))
    except OSError as e:
        print(f"bind {args.host}:{args.port} failed: {e}", file=sys.stderr)
        sys.exit(1)

    print(f"listening udp {args.host}:{args.port}  (Ctrl+C stop)")
    while True:
        try:
            data, addr = sock.recvfrom(65535)
        except KeyboardInterrupt:
            print("\nstop")
            break
        client_ip, client_port = addr[0], addr[1]
        ts = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S UTC")
        msg = (
            f"seen={client_ip}:{client_port} "
            f"bytes={len(data)} "
            f"payload={data[:80]!r} "
            f"time={ts}\n"
        ).encode("utf-8", errors="replace")
        sock.sendto(msg, addr)
        print(f"<- {client_ip}:{client_port}  {len(data)}B  -> replied {len(msg)}B")


if __name__ == "__main__":
    main()
