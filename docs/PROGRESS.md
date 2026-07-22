# joyproxy SPS 开发进度

最后更新：2026-05-28

长效 SPS 部署（拨号 VPS、多弹性 IP、~1000 台 Linux）。部署步骤见 [`MINIPWA.md`](MINIPWA.md)；joyproxy 二进制版本见 [`CHANGELOG.md`](CHANGELOG.md)。

---

## 1. 当前状态（一句话）

**可上线**：`minipwa.pl` + `minipwl.pl` + `setup-sps-v3.sh` + `start-sps-v3.sh` + `joyproxy-linux-amd64-v2.2`，支持单/多出口、双端口 2829/3829、按出口绑出站与白名单/账密鉴权。

---

## 2. 推荐上线文件（`/root`）

| 文件 | 说明 |
|------|------|
| `minipwa.pl` | 鉴权（`127.0.0.1:6301`），仅写 **NO** 日志 |
| `minipwl.pl` | 流量（`127.0.0.1:6303`），连接结束写 **OK** 日志 |
| `setup-sps-v3.sh` | 生成 `eipmap.txt`，拉各出口 `iplist.txt` / `users.txt` |
| `start-sps-v3.sh` | 按 eipmap 起 joyproxy，默认 **2829 + 3829** |
| `joyproxy-linux-amd64-v2.2` | SPS 代理二进制 |

仓库源码路径：`tools/` 下同名文件；`dist/` 下二进制。

**遗留（单端口 3829，仍可用）：** `setup-sps.sh`、`start-sps.sh`。

**已删除/合并：** `discover-eipmap.sh`、`update-sps-auth.sh`、`start-multi-sps.sh` → 并入 setup/start。

---

## 3. 架构

```text
                    ┌─ minipwa.pl :6301  (鉴权, NO 日志)
客户端 ──► joyproxy ─┤
 :2829     :3829     └─ minipwl.pl :6303  (OK 日志, 连接结束)

/root/eipmap.txt                 公网IP  内网IP
/root/auth/denykey.txt           公用黑名单
/root/auth/<公网IP>/iplist.txt   白名单（API sps_cxip）
/root/auth/<公网IP>/users.txt    账密（API sps_cxuser）
/root/s9cron.log                 NO + OK 合并
/root/joyproxy.stdout            joyproxy 标准输出
```

**joyproxy 长效 SPS 参数：**

| 参数 | 值 |
|------|-----|
| `--auth-nouser` | 空账密可走白名单 |
| `--auth-url` | `http://127.0.0.1:6301/get` |
| `--traffic-url` | `http://127.0.0.1:6303/traffic` |
| `--auth-cache` | `0` |
| `--max-conns-rate` | `0` |
| `-g` | 公网 IP（鉴权 `local_addr` 展示，非 dial bind 地址） |

**出站源 IP：** 鉴权响应头 `outgoing: <内网IP>`（minipwa 读 eipmap）；若缺失，joyproxy 源码回退为 `-p` 监听地址（需重新 build 后部署，见 CHANGELOG）。

---

## 4. 外部 API（setup 拉取 auth）

| 用途 | URL |
|------|-----|
| OnlyIp（探测出口） | `http://118.178.178.30:2808/VAD/OnlyIp.aspx?yyy=ip` |
| 白名单 | `http://118.178.178.30:2808/VAD/Pwc.aspx?act=sps_cxip` |
| 账密 | `http://118.178.178.30:2808/VAD/Pwc.aspx?act=sps_cxuser` |

**识别方式：** 服务端 `Request.ServerVariables["REMOTE_ADDR"]`，**无** URL 公网参数（如 `vpsip`）。多出口时用 `curl -4 --interface <内网IP>` 使 REMOTE_ADDR 对应该线公网 IP。

---

## 5. 已完成功能

### 5.1 setup-sps-v3.sh

- [x] 自动判断单出口 / 多出口 → `eipmap.txt`
- [x] 网卡直挂公网 → 单出口一行
- [x] 多个 `192.168.*` → 每内网口 OnlyIp 探测
- [x] **跳过无弹性公网绑定的网卡**（`ip route get from` + 短超时），避免 10+ 网卡卡住
- [x] eipmap 按公网 IP 去重
- [x] baseline：无 `--interface` 时 OnlyIp / sps_cxuser 对比输出
- [x] 注释与 echo **纯 ASCII**（避免 Windows 上传中文变 `???`）
- [x] 环境变量：`PROBE_CONNECT_TIMEOUT`、`PROBE_MAX_TIME`、`FETCH_*` 等

### 5.2 start-sps-v3.sh

- [x] 无 eipmap 时自动执行 `setup-sps-v3.sh`（`SETUP_SCRIPT` 可改）
- [x] 单出口：`:2829` + `:3829` 两个进程
- [x] 多出口：每条 `内网IP:2829` + `内网IP:3829`
- [x] 日志统一 ` /root/joyproxy.stdout`
- [x] `PORTS`、`JOYPROXY_BIN`、`JOYPROXY_LOG` 可环境变量覆盖

### 5.3 minipwa.pl

- [x] **Mode A**（单出口）：`--whitelist` + `--users` + `--denykey`
- [x] **Mode B**（多出口）：`--auth-dir`，按 `local_addr` 公网 IP 选子目录
- [x] 鉴权：账密 → 白名单 IP → denykey → 拒绝
- [x] 拒绝写 **NO** 日志，最后一列 **bytes=0**
- [x] 多出口返回 **`outgoing: <内网IP>`**（`--eipmap`，默认 `/root/eipmap.txt`）
- [x] **`users.txt` 同名多行、不同密码均可通过**
- [x] `SIGHUP` / 定时 reload

### 5.4 minipwl.pl

- [x] 连接成功结束写 OK 行（含 bytes、出口端口等）
- [x] 长效：`--traffic-url` 基址可加 `?act=sd` 避免计入隧道计费（见 `API.md`）

### 5.5 joyproxy 源码（二进制标 v2.2）

- [x] HTTP + SOCKS5 TCP；SOCKS5 UDP（v2.1+ 经 socks5 上级）
- [x] SOCKS5 UDP 回报地址优先 `-g` 公网 IP（v2.2 已发布）
- [x] `effectiveOutgoing`：无 `outgoing` 头时用监听 IP 作出站 bind（**需 rebuild 部署**）

---

## 6. 单/多出口判定（setup）

| 条件 | 结论 | eipmap 行数 |
|------|------|-------------|
| 网卡有公网 IP | 单出口 | 1 |
| ≤1 个 `192.168.*` | 单出口 | 1 |
| 多个 `192.168.*`，OnlyIp 多个不同公网 | 多出口 | N（仅有效绑定的口） |
| 多个 `192.168.*`，OnlyIp 同一公网 | 单出口 | 1 |

---

## 7. 日志格式（`/root/s9cron.log`）

```text
时间戳 NO  HTTP TCP 客户端IP:端口  local_addr  -:-  目标  用户  0
时间戳 OK  S5   TCP 客户端IP:端口  local_addr  出口IP:端口  目标  用户  bytes
```

- **NO**：minipwa 拒绝  
- **OK**：minipwl 连接结束  
- `local_addr`：joyproxy `-g` 公网 IP + 端口（如 `119.84.148.36:3829`）

---

## 8. 运维速查

```bash
# 初始化 / 更新白名单与账密
sh /root/setup-sps-v3.sh
kill -HUP $(pgrep -f minipwa.pl)

# 启动 minipwa / minipwl（见 MINIPWA.md）
# 启动 joyproxy（先停旧进程）
pkill -f 'joyproxy-linux-amd64-v2.2 sps' 2>/dev/null
sh /root/start-sps-v3.sh

# 检查
cat /root/eipmap.txt
ss -lntp | grep -E '2829|3829|6301|6303'
curl -s 'http://127.0.0.1:6301/get?user=&pass=&client_addr=1.2.3.4:5&local_addr=公网IP:3829&target=https://x/&service=http&sps=1'
tail -f /root/s9cron.log
```

---

## 9. 已知现象

| 现象 | 说明 |
|------|------|
| 各线 `users.txt` 内容相同 | API 按 REMOTE_ADDR 返回；各弹性 IP 后台可能只配同一套账密 |
| 无 `--interface` 的 curl 与绑口结果不同 | 默认路由 REMOTE_ADDR ≠ 各拨号线公网 IP |
| 代理出口 IP 不对 | 确认 minipwa 返回 `outgoing`；joyproxy 已 rebuild；多出口用 `-p 内网IP:端口` |
| `6301 connection refused` | minipwa 未启动或参数错误（`--auth-dir` 必须是目录） |
| 旧 Node `s5.js` 占 3829 | 与 joyproxy 冲突，应停掉 `socksv5` |
| setup 中文 `???` | 使用 **setup-sps-v3.sh**（ASCII），二进制方式上传 |

---

## 10. 待办

- [ ] 与 API 方确认各弹性 IP 账密分配规则（REMOTE_ADDR 维度）
- [ ] 将 `effectiveOutgoing` 修复 build 进现网二进制并批量替换
- [ ] ~1000 台批量部署/ansible
- [ ] v2.3 功能（当前决定继续 v2.2 基线）

---

## 11. 文档索引

| 文档 | 内容 |
|------|------|
| [`MINIPWA.md`](MINIPWA.md) | 部署步骤 |
| [`CHANGELOG.md`](CHANGELOG.md) | joyproxy 二进制版本 |
| [`API.md`](API.md) | 鉴权 / 流量 API |
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | Go 模块与协议 |
| [`DEVELOPMENT.md`](DEVELOPMENT.md) | 构建与样例 |
| [`INSTALL.md`](INSTALL.md) | 安装 |
