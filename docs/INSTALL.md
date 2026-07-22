# joyproxy 安装手册

**拨号 VPS / 多弹性 IP 长效 SPS 整套部署**（minipwa + minipwl + setup-sps-v3 + start-sps-v3）见 [`PROGRESS.md`](PROGRESS.md) 与 [`MINIPWA.md`](MINIPWA.md)。下文主要为 joyproxy 二进制安装。

## 环境要求

- Linux x86_64 / ARM64（推荐）
- Go 1.17+（仅编译时需要）
- 可写防火墙与 `ulimit` 调整权限

## 从源码编译

```bash
cd joyproxy
go build -o joyproxy ./cmd/joyproxy
sudo install -m755 joyproxy /usr/local/bin/
```

交叉编译示例：

```bash
GOOS=linux GOARCH=amd64 go build -o joyproxy ./cmd/joyproxy
```

## 日志（不写磁盘文件）

- 所有 **logx** 日志输出到 **stderr**，默认不创建 `.log` 文件，避免占满磁盘。
- **前台**（不加 `--daemon`）：默认只打 **`[warn]`** 与 **`[err]`**（异常与错误），不打监听横幅、不打每条连接的「正常」调试信息。
- **`--daemon`**：worker 与监督进程 **默认完全不写日志**；需要排查时父进程加 **`--verbose`**（监督进程 + worker 都会恢复详细输出）。
- **`--quiet`**：前台仅 **`[err]`**，不打 **`[warn]`**。
- **`--verbose`**：前台完整日志（含 `listening …`、每条连接 trace）；与 **`--daemon`** 联用时同时打开监督进程与 worker 的日志。

正式环境建议用 **systemd/journald** 收 stderr，由系统策略限制日志体积；不要将大量 verbose 重定向到无轮转的普通文件。

## 运行示例

前台调试（关闭终端即退出）：

```bash
joyproxy sps -S http -T tcp -p ":5001-5002" -g "203.0.113.10" \
  --auth-nouser --auth-url "http://127.0.0.1:6301/auth" --auth-cache 0 \
  --max-conns-rate 0 --traffic-url "http://127.0.0.1:6303/traffic" \
  --traffic-mode normal
```

后台 + 崩溃后自动重启（监督进程在后台拉起 worker，崩溃后等待 `--restart-delay` 再拉起）。加 **`--daemon`** 时：**当前 shell 立刻返回**（Linux/macOS：`setsid`；Windows：脱离控制台的新进程组）；监督进程与 worker 在后台跑，`ps` 中监督进程 **不带** `--daemon`、worker **不带** `--forever`（与常见 goproxy 用法一致）。

```bash
joyproxy sps ... --daemon --forever
```

若由 **systemd** 拉起，请加上 **`--no-detach`**，否则父进程会立即退出，systemd 会认为服务已结束：

```bash
joyproxy sps ... --daemon --forever --no-detach
```

## 系统调优（高并发）

```bash
ulimit -n 1048576
# 视内核调整 net.core.somaxconn、nf_conntrack 等
```

## systemd 示例

```ini
[Unit]
Description=joyproxy SPS
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/joyproxy sps -S http -T tcp -p ":5001-5999" -g YOUR_PUBLIC_IP --auth-url ... --daemon --forever --no-detach
Restart=no
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
```

说明：`--daemon --forever` 由 joyproxy 自带监督循环。手工跑后台用默认 detach；**systemd 必须加 `--no-detach`**。也可去掉 `--forever`，仅用 systemd 的 `Restart=always` 管理单进程。

