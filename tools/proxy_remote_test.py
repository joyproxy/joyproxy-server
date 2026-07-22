#!/usr/bin/env python3
"""Remote joyproxy test: HTTP/HTTPS/SOCKS5 TCP + SOCKS5 UDP (DNS) per port + concurrent load."""
from __future__ import annotations

import concurrent.futures
import random
import socket
import struct
import subprocess
import sys
import time
from dataclasses import dataclass
from typing import Optional

HOST = "117.89.88.141"
PORTS = list(range(15001, 15011))
TIMEOUT = 12
CONCURRENCY = 40  # parallel HTTPS tasks per wave
ROUNDS = 4  # repeat concurrent wave


@dataclass
class Result:
    name: str
    port: int
    ok: bool
    detail: str
    ms: float = 0.0


def curl_code(proxy_arg: list[str], url: str) -> tuple[bool, str, float]:
    t0 = time.perf_counter()
    try:
        r = subprocess.run(
            [
                "curl.exe",
                "-sS",
                "-m",
                str(TIMEOUT),
                "-o",
                "NUL",
                "-w",
                "%{http_code}",
                *proxy_arg,
                url,
            ],
            capture_output=True,
            text=True,
            timeout=TIMEOUT + 3,
        )
        ms = (time.perf_counter() - t0) * 1000
        out = (r.stdout or "").strip()
        if r.returncode != 0:
            err = (r.stderr or r.stdout or "").strip()[:200]
            return False, err or f"exit {r.returncode}", ms
        if out.isdigit() and 200 <= int(out) < 400:
            return True, f"http {out}", ms
        return False, f"bad code {out!r}", ms
    except subprocess.TimeoutExpired:
        return False, "curl timeout", (time.perf_counter() - t0) * 1000
    except Exception as e:
        return False, str(e)[:200], (time.perf_counter() - t0) * 1000


def test_http(port: int) -> Result:
    ok, d, ms = curl_code(["-x", f"http://{HOST}:{port}"], "http://www.baidu.com")
    return Result("HTTP->baidu", port, ok, d, ms)


def test_https_baidu_http_proxy(port: int) -> Result:
    ok, d, ms = curl_code(["-x", f"http://{HOST}:{port}"], "https://www.baidu.com")
    return Result("HTTPS(baidu) http-proxy", port, ok, d, ms)


def test_https_xiequ_http_proxy(port: int) -> Result:
    ok, d, ms = curl_code(
        ["-x", f"http://{HOST}:{port}"],
        "https://www.xiequ.cn/OnlyIp.aspx",
    )
    return Result("HTTPS(xiequ) http-proxy", port, ok, d, ms)


def test_https_baidu_socks5(port: int) -> Result:
    ok, d, ms = curl_code(
        ["--socks5-hostname", f"{HOST}:{port}"],
        "https://www.baidu.com",
    )
    return Result("HTTPS(baidu) socks5", port, ok, d, ms)


def test_https_xiequ_socks5(port: int) -> Result:
    ok, d, ms = curl_code(
        ["--socks5-hostname", f"{HOST}:{port}"],
        "https://www.xiequ.cn/OnlyIp.aspx",
    )
    return Result("HTTPS(xiequ) socks5", port, ok, d, ms)


def dns_query_a(name: str) -> bytes:
    tid = random.randint(1, 65535)
    hdr = struct.pack("!HHHHHH", tid, 0x0100, 1, 0, 0, 0)
    q = b""
    for p in name.split("."):
        q += bytes([len(p)]) + p.encode()
    q += b"\x00" + struct.pack("!HH", 1, 1)
    return hdr + q


def socks5_udp_dns(port: int) -> Result:
    try:
        import socks  # type: ignore
    except ImportError:
        return Result("SOCKS5 UDP DNS", port, False, "no pysocks")

    t0 = time.perf_counter()
    sock = socks.socksocket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.set_proxy(socks.SOCKS5, HOST, port)
    sock.settimeout(TIMEOUT)
    pkt = dns_query_a("www.baidu.com")
    try:
        sock.sendto(pkt, ("223.5.5.5", 53))
        data, _ = sock.recvfrom(2048)
        ms = (time.perf_counter() - t0) * 1000
        sock.close()
        if len(data) > 12:
            return Result("SOCKS5 UDP DNS", port, True, f"reply {len(data)}b", ms)
        return Result("SOCKS5 UDP DNS", port, False, f"short reply {len(data)}", ms)
    except Exception as e:
        try:
            sock.close()
        except Exception:
            pass
        return Result(
            "SOCKS5 UDP DNS",
            port,
            False,
            str(e)[:180],
            (time.perf_counter() - t0) * 1000,
        )


def run_per_port():
    rows: list[Result] = []
    for p in PORTS:
        rows.append(test_http(p))
        rows.append(test_https_baidu_http_proxy(p))
        rows.append(test_https_xiequ_http_proxy(p))
        rows.append(test_https_baidu_socks5(p))
        rows.append(test_https_xiequ_socks5(p))
        rows.append(socks5_udp_dns(p))
    return rows


def concurrent_wave(wave: int) -> tuple[int, int, list[str]]:
    """Many parallel HTTPS via random ports (HTTP proxy mode)."""
    ok = 0
    fail = 0
    errs: list[str] = []

    def one(_i: int) -> tuple[bool, str]:
        port = random.choice(PORTS)
        r = test_https_baidu_http_proxy(port)
        return r.ok, f"p{port} {r.detail}"

    with concurrent.futures.ThreadPoolExecutor(max_workers=CONCURRENCY) as ex:
        futs = [ex.submit(one, i) for i in range(CONCURRENCY)]
        for f in concurrent.futures.as_completed(futs):
            good, msg = f.result()
            if good:
                ok += 1
            else:
                fail += 1
                if len(errs) < 12:
                    errs.append(msg)
    return ok, fail, errs


def main() -> int:
    print(f"Target {HOST} ports {PORTS[0]}-{PORTS[-1]} timeout={TIMEOUT}s")
    print("=== Per-port sequential (HTTP / HTTPS http-proxy / HTTPS socks5 / UDP DNS) ===")
    rows = run_per_port()
    by_port: dict[int, list[Result]] = {p: [] for p in PORTS}
    for r in rows:
        by_port[r.port].append(r)

    good_ports = []
    tcp_ok_ports = []
    for p in PORTS:
        rs = by_port[p]
        bad = [x for x in rs if not x.ok]
        tcp_bad = [x for x in bad if "UDP" not in x.name]
        if not bad:
            good_ports.append(p)
            tcp_ok_ports.append(p)
            print(f"  :{p}  ALL OK  ({len(rs)} checks)")
        else:
            print(f"  :{p}  FAIL {len(bad)}/{len(rs)}")
            if not tcp_bad:
                tcp_ok_ports.append(p)
            for b in bad:
                print(f"       - {b.name}: {b.detail}")

    print()
    print(f"=== Concurrent load: {CONCURRENCY} parallel HTTPS(baidu) per wave, {ROUNDS} waves ===")
    total_ok = total_fail = 0
    for w in range(ROUNDS):
        o, f, es = concurrent_wave(w)
        total_ok += o
        total_fail += f
        print(f"  wave {w+1}: ok={o} fail={f}")
        for e in es:
            print(f"    sample fail: {e}")
    print(f"  TOTAL concurrent: ok={total_ok} fail={total_fail}")

    print()
    print(f"Ports ALL checks OK (incl. UDP): {good_ports}")
    print(f"Ports TCP-only OK (HTTP+HTTPS http-proxy+HTTPS socks5): {tcp_ok_ports}")
    return 0 if total_fail == 0 else 0  # exit 0: report only


if __name__ == "__main__":
    sys.exit(main())
