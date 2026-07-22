# joyproxy API 文档

## 认证 API（`--auth-url`）

对每个需要鉴权的连接，joyproxy 会发起 **HTTP GET**。

### 成功

响应 **`200 OK`** 或 **`204 No Content`**，且 **`upstream` 为可用的上级 URL**（`http://...` / `socks5://...`），视为鉴权通过，并读取下方业务响应头。

### 拒绝（`upstream: ERR` 等）

当 **`upstream` 为 ERR 等占位**（见 `UpstreamMeansAuthDeny`）时，joyproxy **不会去连 ERR**，并对 **HTTP 代理客户端一律返回 `503 Service Unavailable`**。

这样 **无需改鉴权服务（PWA）**：常见实现里「未授权 / 并发超限 / 流控拒绝」对上游返回的头相同，joyproxy **无法区分**，若对其中一部分发 **407** 会误导客户端以为要换账号。统一 **503** 最合适。

**`407 Proxy Authentication Required`** 仅在 joyproxy **本地**判断需要：例如未启 `--auth-nouser` 时客户端未带或未带齐 **`Proxy-Authorization`**（在调用你的鉴权 URL **之前** 就会返回 407）。

### 可选扩展（仅当你能在不改变 PWA 源码的前提下，在中间层加响应头）

若在到达 joyproxy 之前能为响应加上：

- `X-Joyproxy-Reject-Status: 407` 或 `X-Joyproxy-Deny: auth`（及 `unauthorized`、`credential`、`whitelist`、`forbidden`）→ 对客户端 **407**  
- `503` / `429` 或 `X-Joyproxy-Deny: limit`（及 `concurrent`、`rate`、`overload` 等）→ **503** / **429**

未设置时仍按上一节默认为 **503**。

### 其它 HTTP 状态

鉴权 URL 返回非 **200 / 204**：视为失败，对客户端 **503**。

### 查询参数

| 参数 | 说明 |
|------|------|
| user | 客户端用户名（`--auth-nouser` 时可为空） |
| pass | 客户端密码 |
| client_addr | 客户端地址 `IP:端口` |
| local_addr | 代理对外地址 `IP:端口`（由 `-g` 与监听端口拼出） |
| target | HTTP(S) 为 URL；SOCKS5 TCP 为目标 `host:port`；UDP 首次可为空，数据到达后为 `host:port` |
| service | `http` 或 `socks` |
| sps | 固定传 `1`（SPS 模式） |

### 成功响应头（可选，不区分大小写）

- userconns / ipconns：最大连接数，0 或未设为不限制  
- userrate / iprate：单 TCP 连接限速（字节/秒）  
- userqps / ipqps：每秒新建连接数  
- upstream：上级代理 URL（`http://...` / `socks5://...`，可含用户名密码）  
- outgoing：源 IP 绑定提示（若可解析为 IP 则用于 dial 绑定）  
- userTotalRate / ipTotalRate / portTotalRate：总带宽（字节/秒），当前版本 **尚未完全强制**，以单连接限速与连接数为主  

## 流量上报 API（`--traffic-url`）

连接结束时 **异步 GET**，必须返回 **204** 视为成功。

### PWL `act` 与计费（长效 SPS 必读）

joyproxy 每条连接还会附带 `act=traffic` 查询参数；若 `--traffic-url` 基址已含 `act=sd`，PWL 以**第一个** `act` 为准。

| PWL 收到的 `act` | 行为 |
|------------------|------|
| `pwa` | 写入 MySQL `h_ip_traffic_log`，参与隧道代理计费 |
| `sd` | 仅 stdout 日志，**不入库、不计费** |

长效 SPS（PWA-LONG）应配置：

`--traffic-url "http://192.168.0.8:6303/traffic?act=sd"`

避免长效流量混入隧道代理计费统计。

### 查询参数

| 参数 | 说明 |
|------|------|
| act | joyproxy 默认传 `traffic`；PWL 侧长效部署在 URL 基址写 `act=sd` |
| bytes | 本连接字节数（双向合计） |
| client_addr | 客户端 `IP:端口` |
| server_addr | 代理服务地址 `IP:端口` |
| target_addr | 目标 `IP:端口` 或主机形式 |
| username | 代理认证用户 |
| upstream | 实际上级 URL，无则空 |
| out_local_addr / out_remote_addr | 出站到上级或目标的 TCP 本地/远端地址 |
| id | `http` 或 `socks` |
| sniff_domain | 启用 `--sniff-domain` 且为 TLS 时可能包含 SNI |
