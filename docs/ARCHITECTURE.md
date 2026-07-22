# joyproxy 架构说明

## 组件

- `cmd/joyproxy`：入口。
- `internal/cli`：命令行解析（`sps` 子命令）。
- `internal/daemon`：`--daemon --forever` 时先 detach（父进程退出），监督循环在后台进程里跑；子进程参数里去掉了 `--forever`。Unix 用 `setsid`；Windows 用 `DETACHED_PROCESS`。`--no-detach` 关闭 detach，供 systemd 使用。
- `internal/sps`：监听端口范围，首字节区分 HTTP 与 SOCKS5，分别处理 CONNECT/普通代理、SOCKS5 TCP/UDP ASSOCIATE。
- `internal/authapi`：调用认证 URL，解析响应头，TTL 缓存。
- `internal/traffic`：连接结束上报。
- `internal/upstream`：经 HTTP CONNECT 或 SOCKS5 连接上级，或直接直连目标。
- `internal/limit`：全局限连、`userconns`/`ipconns`、`userqps`/`ipqps`、单连接简单限速。

## SPS 长效部署栈（Perl + shell）

与 Go 二进制配合的现网方案见 **`docs/PROGRESS.md`**：

- `minipwa.pl` / `minipwl.pl`：本地鉴权与日志（6301 / 6303）
- `setup-sps-v3.sh` / `start-sps-v3.sh`：eipmap、拉 auth、双端口启动 joyproxy
- 多出口时 minipwa 返回 `outgoing`；joyproxy 用其或监听地址绑定出站源 IP

## 未实现 / 后续

- 用户/端口总带宽（userTotalRate 等）的严格全局令牌桶。
- 认证文件、ip.limit 文件优先级链。
- SOCKS5 UDP：上级为 `socks5://` 时经上级 UDP ASSOCIATE 转发；无上级时直连目标；`http://` 上级不支持 UDP。

## Shadowsocks

按产品约定，**不提供 SS 入站**。

