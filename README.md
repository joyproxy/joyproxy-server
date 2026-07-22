# joyproxy-server

JoyProxy 服务端：高性能 HTTP / SOCKS5 代理网关（SPS），支持鉴权回调、流量上报、连接限速与后台守护。

## 功能

- HTTP / SOCKS5 代理监听
- 端口段监听（如 `:5001-5999`）
- 鉴权 API 回调（`--auth-url`）
- 流量上报（`--traffic-url`）
- 连接数 / 带宽限速
- 后台守护与崩溃自动重启（`--daemon --forever`）

## 下载

### 从源码编译（推荐）

环境要求：Go 1.17+

```bash
git clone https://github.com/joyproxy/joyproxy-server.git
cd joyproxy-server
go build -o joyproxy ./cmd/joyproxy
```

交叉编译 Linux amd64：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o joyproxy ./cmd/joyproxy
```

或使用 Makefile 打包：

```bash
make dist-linux
```

产物位于 `dist/` 目录。

## 使用

### 查看帮助

```bash
./joyproxy sps -h
```

### 前台调试

```bash
./joyproxy sps -S http -T tcp -p ":5001-5002" -g "203.0.113.10" \
  --auth-nouser --auth-url "http://127.0.0.1:6301/auth" --auth-cache 0 \
  --max-conns-rate 0 --traffic-url "http://127.0.0.1:6303/traffic" \
  --traffic-mode normal
```

### 后台运行（关终端不退出）

```bash
./joyproxy sps -S http -T tcp -p ":5001-5999" -g "你的公网IP" \
  --auth-nouser --auth-url "http://127.0.0.1:6301/auth" \
  --traffic-url "http://127.0.0.1:6303/traffic" --traffic-mode normal \
  --daemon --forever
```

### systemd 部署

由 systemd 管理时，需加 `--no-detach`，否则主进程会立即退出：

```bash
./joyproxy sps ... --daemon --forever --no-detach
```

### 常用参数

| 参数 | 说明 |
|------|------|
| `-S` | 上游类型：`http` 或 `socks5` |
| `-p` | 监听端口，支持范围如 `:5001-5999` |
| `-g` | 公网 IP，用于鉴权回调中的 `local_addr` |
| `--auth-url` | 鉴权 API 地址 |
| `--auth-nouser` | 允许空用户名密码 |
| `--traffic-url` | 流量上报 API 地址 |
| `--daemon` | 后台运行 |
| `--forever` | 崩溃后自动重启（配合 `--daemon`） |
| `--verbose` | 输出完整日志 |
| `--quiet` | 仅输出错误日志 |

## 文档

- [安装手册](docs/INSTALL.md)
- [API 文档](docs/API.md)
- [架构说明](docs/ARCHITECTURE.md)
- [变更日志](docs/CHANGELOG.md)

## 许可证

本项目遵循 GPLv3 许可证。
