# joyproxy 开发说明与运行样例

本文档归纳本项目当前实现范围、发布物与典型启动命令，便于交接与运维查阅。更细的安装、API 与架构见同目录下 `INSTALL.md`、`API.md`、`ARCHITECTURE.md`。**版本变更见 [`CHANGELOG.md`](CHANGELOG.md)。**

## 1. 项目定位

- **joyproxy**：单二进制 SPS 代理（HTTP + SOCKS5），见下文。
- **SPS 长效部署栈**（Perl + shell，约 1000 台拨号/多 IP 机器）：`minipwa.pl`、`minipwl.pl`、`setup-sps-v3.sh`、`start-sps-v3.sh` — **进度与清单见 [`PROGRESS.md`](PROGRESS.md)**，部署步骤见 [`MINIPWA.md`](MINIPWA.md)。
- **模块**：Go **1.17**，根模块名 `joyproxy`，入口 `cmd/joyproxy`，CLI 基于 **cobra**。

## 2. 已实现能力（摘要）

| 领域 | 说明 |
|------|------|
| 监听 | 端口区间 `-p ":5001-5999"`，单进程多端口 mux。 |
| 协议 | HTTP 代理（含 CONNECT）、SOCKS5 TCP；UDP ASSOCIATE（行为与上级类型见 `ARCHITECTURE.md`）。 |
| 认证 | `--auth-url` 拉取策略头；`--auth-nouser`、`--auth-cache`；上游拒绝与 HTTP 状态码规则见 `API.md`。 |
| 流量 | `--traffic-url`、`--traffic-mode`（如 `normal`）。 |
| 限流 | `--max-conns-rate` 等，详情见 CLI help 与 `internal/limit`。 |
| 常驻 | `--forever` 监督循环；`--daemon` 后台脱离 shell（与 `--forever` 联用时监督进程在后台拉起 worker）。 |
| systemd | 需加 **`--no-detach`**，避免父进程退出导致 unit 判死。 |

## 3. 目录与构建

- **源码**：`cmd/joyproxy`、`internal/{cli,daemon,sps,authapi,upstream,traffic,limit,config,logx}`。
- **文档**：`docs/`。
- **发布**：当前正式版 `dist/joyproxy-linux-amd64-v2.2`；历史版本见 `CHANGELOG.md`。

交叉编译示例：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o dist/joyproxy-linux-amd64-v2.2 ./cmd/joyproxy
```

## 4. `--daemon` 与 Go 1.17（重要）

- Unix 侧脱离终端的实现位于 `internal/daemon/detach_unix.go`。
- **Go 1.19 起**才有内置构建约束 `unix`；若使用 **Go 1.17.x** 编译且仍用 `//go:build unix`，Linux 上可能不会编入 detach 逻辑，导致**即使加 `--daemon` 仍会占住前台**。
- **当前约定**：`detach_unix.go` 使用 **`//go:build !windows`**，在 Go 1.17 上对 Linux 生效；Windows 仍用 `detach_windows.go`。发布用 Linux 二进制请用与生产一致的 Go 版本重新构建并验证 shell 能立即返回。

## 5. 长效 SPS 样例（拨号 VPS / 多出口）

与现网 `start-sps-v3.sh` 一致的核心参数：

```bash
/root/joyproxy-linux-amd64-v2.2 sps -p :3829 -g "117.89.88.141" \
  --auth-nouser \
  --auth-url http://127.0.0.1:6301/get \
  --traffic-url http://127.0.0.1:6303/traffic \
  --auth-cache 0 --max-conns-rate 0
```

双端口、多出口绑定见 `start-sps-v3.sh` 与 [`PROGRESS.md`](PROGRESS.md)。

## 6. 生产启动样例（HTTP SPS + 后台，端口段）

以下与现网用法一致：HTTP 模式、端口段、公网 IP 告示、外挂认证、关闭认证缓存、不限新连接速率、流量上报、`normal` 模式、后台常驻。

```bash
/root/joyproxy-linux-amd64 sps -S http -p ":5001-5999" -g "117.89.88.141" \
  --forever --auth-nouser \
  --auth-url http://127.0.0.1:6301/get?spid=40\&uptyp=http \
  --auth-cache 0 --max-conns-rate 0 \
  --traffic-url http://192.168.0.8:6303/traffic --traffic-mode normal \
  --daemon
```

**说明**：

- `auth-url` 中查询串的 `&` 在 shell 里需写成 `\&`（或整段 URL 加引号），否则会被当成后台命令分隔符。
- 需要排查问题时可在同一命令上增加 **`--verbose`**（见 `INSTALL.md` 日志一节）。

## 7. 验证后台是否生效

```bash
ps aux | grep joyproxy
```

shell 应在提交上述命令后**马上回到提示符**；进程中监督端与 worker 的参数应符合 `ARCHITECTURE.md` 描述（worker 不带 `--forever`，监督端不带 `--daemon`）。

---

*文档随实现迭代更新；未列功能以源码与 `ARCHITECTURE.md`「未实现 / 后续」为准。*
