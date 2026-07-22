# joyproxy 版本说明

发布二进制位于 `dist/`，命名规则：`joyproxy-linux-amd64-vX.Y`。

---

## v2.2（当前推荐）

**文件**：`dist/joyproxy-linux-amd64-v2.2`

### 修复

- **SOCKS5 UDP ASSOCIATE 回报地址错误**：此前用本机监听网卡 IP（如 `192.168.0.141`）作为客户端发包地址，外网用户无法把 UDP 打到代理；现优先使用 `-g` 公网 IP（如 `117.89.88.141`），与鉴权 `local_addr` 一致。

### 源码已改、需重新 build 后生效

- **多出口出站源地址**：鉴权未返回 `outgoing` 时，使用 `-p` 绑定地址（如 `192.168.0.21`）作为 TCP/UDP dial 的 local bind，避免全部走默认路由。见 `internal/sps/relay.go` 中 `effectiveOutgoing`。

### SPS 部署栈（Perl + shell，见 `docs/PROGRESS.md`）

- `setup-sps-v3.sh` / `start-sps-v3.sh`（双端口 2829+3829，OnlyIp 跳过无公网网卡）
- `minipwa.pl`：`outgoing` 响应头、同名用户多密码、`--auth-dir` 多出口
- `minipwl.pl`：OK 日志

### 现象（v2.1 及更早）

- 日志有 `relay via upstream=...`、`relay udp bind :端口`，但 `session end total_bytes=0`
- 客户端 `recvfrom` 超时

---

## v2.1

**文件**：`dist/joyproxy-linux-amd64-v2.1`

### 新增

- **SOCKS5 UDP 经上级转发**：客户端 `UDP ASSOCIATE` 时，若鉴权返回的上级为 `socks5://...`，则向上级发起 SOCKS5 UDP ASSOCIATE，UDP 数据经上级中继，与 TCP 一样走指定线路。
- 仍与 HTTP / SOCKS5 TCP **共用同一监听端口**，无需单独开 UDP 端口（需放行代理机动态 UDP 中继端口）。

### 行为说明

| 鉴权 `upstream` | SOCKS5 UDP |
|-----------------|------------|
| `socks5://...` | 经上级 UDP 转发 |
| 空 | 直连目标（与旧版相同） |
| `http://` / `https://` | **不支持**，拒绝 UDP ASSOCIATE |

### 运维注意

- 6302（或 `--auth-url`）若要对 UDP 生效，需对 SOCKS 会话返回 **`socks5://` 上级**；仅返回 `http://` 时 UDP 无法走上级。
- 上级 SOCKS5 需本身支持 UDP ASSOCIATE。

---

## v2.0

**文件**：`dist/joyproxy-linux-amd64-v2.0`

### 修复

- **SOCKS5 账密未传到鉴权接口**：修复子协商后 `user/pass` 未正确带入 `Authorize()` 的问题（历史上 `:=` 变量遮蔽导致空账密打到 `--auth-url`）。HTTP 代理不受影响。
- 未开 `--auth-nouser` 时，SOCKS5 客户端未提供用户名密码方法（0x02）会明确拒绝。

### 说明

- 生产环境若仍出现 `service=socks` 且 `user` 为空，请确认已部署 **v2.0 及之后** 二进制，而非旧 `joyproxy-linux-amd64`。

---

## v2.0 之前（`dist/joyproxy-linux-amd64` 无版本号）

基线能力（详见 `dist/README-CentOS7.txt`、`docs/ARCHITECTURE.md`）：

- 同端口 HTTP + SOCKS5 TCP；`--auth-url` / `--traffic-url`；`--daemon --forever`。
- SOCKS5 请求头解析（VER/CMD/RSV 与 ATYP 分离）；上级 CONNECT 后清除 deadline；`UPSTREAM:ERR` 时对客户端默认 503。
- SOCKS5 UDP ASSOCIATE：**仅直连目标**，不经 HTTP/SOCKS5 上级。

---

## 文档索引

| 文档 | 内容 |
|------|------|
| `docs/PROGRESS.md` | **SPS 部署开发进度（当前）** |
| `docs/MINIPWA.md` | minipwa / minipwl / setup / start 部署 |
| `docs/INSTALL.md` | 安装与启动 |
| `docs/API.md` | 鉴权 / 流量 API |
| `docs/ARCHITECTURE.md` | 架构与协议行为 |
| `docs/DEVELOPMENT.md` | 开发构建与样例命令 |

---

## 构建当前版本

```bash
cd joyproxy_sps
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o dist/joyproxy-linux-amd64-v2.2 ./cmd/joyproxy
```

每次发版请在本文件顶部追加新版本条目，并更新 `docs/DEVELOPMENT.md`、`dist/README-CentOS7.txt` 中的「当前正式版」行。
