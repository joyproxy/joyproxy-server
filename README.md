# joyproxy-server

[English](#english) | [中文](#中文)

---

<a id="english"></a>

## English

High-performance HTTP / SOCKS5 proxy gateway (SPS) for Linux. Supports optional external authorization API and optional traffic reporting API.

### Features

- HTTP and SOCKS5 proxy on configurable ports (single port or range, e.g. `:5001-5999`)
- Optional **external auth API** (`--auth-url`) — your service decides allow/deny per connection
- Optional **external traffic API** (`--traffic-url`) — async report when a connection ends
- Per-user / per-IP connection limits and bandwidth limits (returned by auth API response headers)
- Global new-connection rate limit (`--max-conns-rate`)
- Background mode with auto-restart (`--daemon --forever`)
- TLS SNI sniffing (`--sniff-domain`, optional)

### Download

Download prebuilt binaries from [GitHub Releases](https://github.com/joyproxy/joyproxy-server/releases).

1. Open the latest Release (e.g. **v2.2**)
2. Download `joyproxy-linux-amd64` for Linux amd64
3. Upload to your server and run `chmod +x joyproxy-linux-amd64`

Optional: `joyproxy-centos7-linux-amd64.tar.gz` includes the binary and a short README (CentOS 7.x compatible, glibc 2.17+).

### Quick Start

```bash
./joyproxy sps -h          # show all flags
./joyproxy sps -S http -p ":8080" -g "YOUR_PUBLIC_IP"
```

| Flag | Required | Description |
|------|----------|-------------|
| `-S` | No (default `http`) | Upstream relay type: `http` or `socks5` |
| `-p` | Yes | Listen port(s), e.g. `:8080` or `:5001-5999` |
| `-g` | Recommended | Public IP of this server; used as `local_addr` when calling auth API |
| `-parent` | No | Default upstream URL if auth API does not return `upstream` |
| `--daemon` | No | Run in background (shell returns immediately) |
| `--forever` | No | With `--daemon`: auto-restart worker on crash |
| `--no-detach` | No | With `--daemon`: stay attached (use with systemd) |
| `--verbose` | No | Full logs |
| `--quiet` | No | Errors only |
| `--max-conns-rate` | No | Global max new connections per second (0 = unlimited) |
| `--sniff-domain` | No | Sniff TLS SNI on HTTP CONNECT |

---

### Startup Modes

#### 1. Open proxy (no password)

No `--auth-url`. Anyone who can reach the port may use the proxy. Client credentials are not required.

```bash
./joyproxy sps -S http -p ":8080" -g "YOUR_PUBLIC_IP"
```

#### 2. Whitelist authorization (no client password, external API decides)

Use `--auth-nouser` + `--auth-url`. Clients do **not** need to send a username/password; joyproxy calls **your** auth API for every connection. Your API allows or denies based on `client_addr`, `target`, etc. (IP whitelist, target whitelist, etc.).

```bash
./joyproxy sps -S http -p ":8080" -g "YOUR_PUBLIC_IP" \
  --auth-nouser --auth-url "https://your-api.example.com/auth"
```

#### 3. Username / password authorization

Do **not** use `--auth-nouser`. Clients must send `Proxy-Authorization` (HTTP) or SOCKS5 username/password. joyproxy returns **407** if credentials are missing.

Without `--auth-url`, credentials are only checked locally (non-empty user and pass).

```bash
./joyproxy sps -S http -p ":8080" -g "YOUR_PUBLIC_IP"
```

With `--auth-url`, credentials are forwarded to your API for validation (see mode 4).

#### 4. External auth API (optional)

`--auth-url` is **optional**. When set, joyproxy sends an **HTTP GET** to your endpoint for each connection.

**Query parameters sent:**

| Parameter | Description |
|-----------|-------------|
| `user` | Client username (empty if `--auth-nouser`) |
| `pass` | Client password |
| `client_addr` | Client address `IP:port` |
| `local_addr` | Proxy listen address `IP:port` (from `-g` + listen port) |
| `target` | Destination: HTTP URL or `host:port` for SOCKS5 |
| `service` | `http` or `socks` |
| `sps` | Always `1` |

**Your API should respond:**

- **200** or **204** with header `upstream: http://...` or `socks5://...` → allow (optional limit headers below)
- **Deny** → return `upstream: ERR` or non-200/204; client gets **503** (or **407** / **429** if you set `X-Joyproxy-Reject-Status` / `X-Joyproxy-Deny`)

**Optional response headers (limits):**

| Header | Meaning |
|--------|---------|
| `upstream` | Upstream proxy URL for this connection |
| `outgoing` | Bind source IP hint |
| `userconns` / `ipconns` | Max concurrent connections |
| `userrate` / `iprate` | Per-connection bandwidth (bytes/s) |
| `userqps` / `ipqps` | Max new connections per second |

Cache: `--auth-cache` (success TTL seconds), `--auth-fail-cache` (fail TTL seconds).

#### 5. External traffic API (optional)

`--traffic-url` is **optional**. When set, joyproxy sends an **async HTTP GET** to your endpoint when each connection **ends**. Your API should return **204 No Content**.

**Query parameters sent:**

| Parameter | Description |
|-----------|-------------|
| `act` | `traffic` |
| `bytes` | Total bytes transferred (up + down) |
| `client_addr` | Client `IP:port` |
| `server_addr` | Proxy service `IP:port` |
| `target_addr` | Target host or `IP:port` |
| `username` | Proxy auth username (if any) |
| `upstream` | Upstream URL used (if any) |
| `out_local_addr` | Outbound TCP local address |
| `out_remote_addr` | Outbound TCP remote address |
| `id` | `http` or `socks` |
| `sniff_domain` | TLS SNI (only when `--sniff-domain` enabled) |

Example (traffic reporting only, no auth API):

```bash
./joyproxy sps -S http -p ":8080" -g "YOUR_PUBLIC_IP" \
  --auth-nouser --traffic-url "https://your-api.example.com/traffic"
```

You can combine auth API and traffic API in one command.

### Background run

```bash
./joyproxy sps -S http -p ":5001-5999" -g "YOUR_PUBLIC_IP" \
  --auth-nouser --daemon --forever
```

With **systemd**, add `--no-detach` so the main process does not exit immediately.

### License

GPLv3

---

<a id="中文"></a>

## 中文

面向 Linux 的高性能 HTTP / SOCKS5 代理网关（SPS）。支持可选的外部鉴权 API 与可选的流量上报 API。

### 功能特性

- HTTP / SOCKS5 代理监听，支持单端口或端口段（如 `:5001-5999`）
- 可选 **外部鉴权 API**（`--auth-url`）— 由你的服务按连接决定是否放行
- 可选 **外部流量 API**（`--traffic-url`）— 连接结束时异步上报
- 按用户 / IP 的连接数与带宽限速（由鉴权 API 响应头下发）
- 全局新建连接速率限制（`--max-conns-rate`）
- 后台守护与崩溃自动重启（`--daemon --forever`）
- TLS SNI 嗅探（`--sniff-domain`，可选）

### 下载

从 [GitHub Releases](https://github.com/joyproxy/joyproxy-server/releases) 下载预编译二进制。

1. 打开最新 Release（如 **v2.2**）
2. 下载 Linux amd64 的 `joyproxy-linux-amd64`
3. 上传到服务器后执行 `chmod +x joyproxy-linux-amd64`

可选：`joyproxy-centos7-linux-amd64.tar.gz` 包含二进制与简要说明（兼容 CentOS 7.x，glibc 2.17+）。

### 快速开始

```bash
./joyproxy sps -h          # 查看所有参数
./joyproxy sps -S http -p ":8080" -g "你的公网IP"
```

| 参数 | 必填 | 说明 |
|------|------|------|
| `-S` | 否（默认 `http`） | 上游中继类型：`http` 或 `socks5` |
| `-p` | 是 | 监听端口，如 `:8080` 或 `:5001-5999` |
| `-g` | 建议填写 | 本机公网 IP；调用鉴权 API 时作为 `local_addr` |
| `-parent` | 否 | 鉴权 API 未返回 `upstream` 时的默认上级代理 |
| `--daemon` | 否 | 后台运行（shell 立即返回） |
| `--forever` | 否 | 配合 `--daemon`：崩溃后自动重启 |
| `--no-detach` | 否 | 配合 `--daemon`：不脱离终端（systemd 使用） |
| `--verbose` | 否 | 完整日志 |
| `--quiet` | 否 | 仅错误日志 |
| `--max-conns-rate` | 否 | 全局每秒最大新建连接数（0 不限制） |
| `--sniff-domain` | 否 | 对 HTTP CONNECT 嗅探 TLS SNI |

---

### 启动模式

#### 1. 无密码启动（开放代理）

不配置 `--auth-url`。能访问端口的客户端均可使用，不要求账密。

```bash
./joyproxy sps -S http -p ":8080" -g "你的公网IP"
```

#### 2. 白名单授权启动（客户端无密码，由外部 API 判定）

使用 `--auth-nouser` + `--auth-url`。客户端**无需**发送用户名密码；每条连接 joyproxy 会请求**你的**鉴权 API，由 API 根据 `client_addr`、`target` 等决定是否放行（IP 白名单、目标白名单等）。

```bash
./joyproxy sps -S http -p ":8080" -g "你的公网IP" \
  --auth-nouser --auth-url "https://你的域名/auth"
```

#### 3. 账密授权启动

**不要**加 `--auth-nouser`。客户端必须发送 HTTP `Proxy-Authorization` 或 SOCKS5 用户名密码，否则返回 **407**。

未配置 `--auth-url` 时，仅在本地校验账密非空；配置 `--auth-url` 后，账密会转发给你的 API 校验（见模式 4）。

```bash
./joyproxy sps -S http -p ":8080" -g "你的公网IP"
```

#### 4. 外部 API 鉴权（可选）

`--auth-url` **非必填**。配置后，每条连接向你的地址发起 **HTTP GET**。

**发出的查询参数：**

| 参数 | 说明 |
|------|------|
| `user` | 客户端用户名（`--auth-nouser` 时可为空） |
| `pass` | 客户端密码 |
| `client_addr` | 客户端地址 `IP:端口` |
| `local_addr` | 代理对外地址 `IP:端口`（由 `-g` 与监听端口组成） |
| `target` | 目标：HTTP 为 URL；SOCKS5 为 `host:port` |
| `service` | `http` 或 `socks` |
| `sps` | 固定为 `1` |

**你的 API 应返回：**

- **200** 或 **204**，且响应头 `upstream: http://...` 或 `socks5://...` → 放行（可附带下方限速头）
- **拒绝** → 返回 `upstream: ERR` 或非 200/204；客户端收到 **503**（也可通过 `X-Joyproxy-Reject-Status` / `X-Joyproxy-Deny` 返回 **407** / **429**）

**可选响应头（限速）：**

| 响应头 | 含义 |
|--------|------|
| `upstream` | 本连接使用的上级代理 URL |
| `outgoing` | 出站源 IP 绑定提示 |
| `userconns` / `ipconns` | 最大并发连接数 |
| `userrate` / `iprate` | 单连接带宽（字节/秒） |
| `userqps` / `ipqps` | 每秒最大新建连接数 |

缓存：`--auth-cache`（成功缓存秒数）、`--auth-fail-cache`（失败缓存秒数）。

#### 5. 外部流量 API（可选）

`--traffic-url` **非必填**。配置后，每条连接**结束**时异步向你的地址发起 **HTTP GET**，你的 API 应返回 **204 No Content**。

**发出的查询参数：**

| 参数 | 说明 |
|------|------|
| `act` | `traffic` |
| `bytes` | 本连接传输字节数（上行 + 下行） |
| `client_addr` | 客户端 `IP:端口` |
| `server_addr` | 代理服务地址 `IP:端口` |
| `target_addr` | 目标 `IP:端口` 或主机名 |
| `username` | 代理认证用户名（如有） |
| `upstream` | 实际上级 URL（如有） |
| `out_local_addr` | 出站 TCP 本地地址 |
| `out_remote_addr` | 出站 TCP 远端地址 |
| `id` | `http` 或 `socks` |
| `sniff_domain` | TLS SNI（仅启用 `--sniff-domain` 时可能有） |

仅流量上报、不鉴权的示例：

```bash
./joyproxy sps -S http -p ":8080" -g "你的公网IP" \
  --auth-nouser --traffic-url "https://你的域名/traffic"
```

鉴权 API 与流量 API 可在同一条启动命令中组合使用。

### 后台运行

```bash
./joyproxy sps -S http -p ":5001-5999" -g "你的公网IP" \
  --auth-nouser --daemon --forever
```

使用 **systemd** 时请加 `--no-detach`，避免主进程立即退出。

### 许可证

GPLv3
