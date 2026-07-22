joyproxy — Linux amd64 二进制（CGO 关闭，适合 CentOS 7.2 / 7.x，glibc 2.17）

当前正式版
  joyproxy-linux-amd64-v2.1    当前推荐发布版本（latest，SOCKS5 UDP 经 socks5 上级）
  joyproxy-linux-amd64-v2.0    上一稳定版

版本功能说明见 ../docs/CHANGELOG.md

包内文件（与历史一致）
  joyproxy-linux-amd64                    可执行文件，拷到 CentOS 7.2 上 chmod +x 即用
  joyproxy-linux-amd64-v2.1               正式版二进制（推荐）
  joyproxy-linux-amd64-v2.0               上一稳定版
  joyproxy-centos7-linux-amd64.tar.gz     本包（解压后得到上面二进制 + 本 README）

本包版本要点（新版）
  - SOCKS5 请求头解析修正（VER/CMD/RSV 与 ATYP 分离）
  - 经上级 HTTP/SOCKS 建链后清除 SetDeadline，避免长连接误报 i/o timeout
  - relay 任一侧结束后双向关闭，减少客户端读超时
  - 上级 URL 无 scheme 时自动补全为 http://；错误日志带出真实 upstream 串
  - 鉴权：200/204 + 有效 upstream 为通过；UPSTREAM:ERR 时对客户端默认 503（不改 PWA）；407 仅本地缺凭证等
  - Unix 下 --daemon 脱离终端（re-exec + setsid），关 SSH 后仍运行；systemd 请加 --no-detach
  - 日志：前台默认仅 [warn]/[err]；--quiet 仅 [err]；--verbose 全量；--daemon 下 worker+监督默认无日志，需排障时父进程加 --verbose
  - 流量上报 bytes = 上行+下行，单位 byte

文件说明
  joyproxy-linux-amd64      可执行文件
  README-CentOS7.txt        本说明
  build-linux-amd64.ps1     Windows 上交叉编译打包（需本机已装 Go）
  build-linux-amd64.sh      Linux/macOS 上编译打包

下载后（在 CentOS 7.x 上）
  chmod +x joyproxy-linux-amd64
  ./joyproxy-linux-amd64 sps -h

前台（默认只打异常： [warn] / [err]，不写日志文件）
  ./joyproxy-linux-amd64 sps -p ":18080" -S http -g "你的公网IP"

需要完整连接跟踪时
  ./joyproxy-linux-amd64 sps ... --verbose

后台正式跑（Linux 下命令立即返回，关终端不退出）
  ./joyproxy-linux-amd64 sps -S http -T tcp -p ":5001-5999" -g "公网IP" \
    --forever --auth-nouser --auth-url "http://..." --auth-cache 0 \
    --max-conns-rate 0 --traffic-url "http://..." --traffic-mode normal --daemon

systemd（勿用终端脱离，否则主进程秒退）
  在 ExecStart 中加 --no-detach

常用参数
  --verbose   全量日志；与 --daemon 同时打开监督进程 + worker 日志
  --quiet     仅 [err]
  --no-detach Unix + systemd 时使用

注意
  - amd64（x86_64）专用；防火墙放行端口段。
  - 日志只输出 stderr；verbose 勿长期写入无轮转大文件。

从源码打包（本机安装 Go 1.17+）
  Windows（在 joyproxy 仓库根目录）:
    powershell -ExecutionPolicy Bypass -File dist/build-linux-amd64.ps1
  Linux / macOS:
    chmod +x dist/build-linux-amd64.sh && ./dist/build-linux-amd64.sh
  生成: dist/joyproxy-linux-amd64 与 dist/joyproxy-centos7-linux-amd64.tar.gz

构建参数
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0
