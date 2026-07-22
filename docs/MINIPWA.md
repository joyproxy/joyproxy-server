# mini-PWA / mini-PWL 部署

当前推荐文件（`/root`）：

| 文件 | 作用 |
|------|------|
| `minipwa.pl` | 鉴权 + 仅记 NO |
| `minipwl.pl` | 仅记 OK 完整日志 |
| `setup-sps-v3.sh` | 建 eipmap + `/root/auth/<公网IP>/` 下 iplist/users |
| `start-sps-v3.sh` | 按 eipmap 启动 joyproxy（默认 **2829 + 3829**） |
| `joyproxy-linux-amd64-v2.2` | SPS 二进制 |

## 目录

```text
/root/eipmap.txt
/root/auth/denykey.txt
/root/auth/117.89.88.141/iplist.txt
/root/auth/117.89.88.141/users.txt
```

单出口也可用 Mode A 扁平文件：`/root/auth/iplist.txt`、`users.txt`、`denykey.txt`。

## 服务器执行顺序

```bash
cd /root
chmod +x minipwa.pl minipwl.pl setup-sps-v3.sh start-sps-v3.sh joyproxy-linux-amd64-v2.2
sed -i 's/\r$//' minipwa.pl minipwl.pl setup-sps-v3.sh start-sps-v3.sh

sh setup-sps-v3.sh
vi /root/auth/denykey.txt

nohup perl /root/minipwa.pl \
  --listen 127.0.0.1:6301 \
  --auth-dir /root/auth \
  --denykey /root/auth/denykey.txt \
  --log /root/s9cron.log >> /root/minipwa.stdout 2>&1 &

nohup perl /root/minipwl.pl \
  --listen 127.0.0.1:6303 \
  --log /root/s9cron.log >> /root/minipwl.stdout 2>&1 &

sh start-sps-v3.sh
```

单出口 Mode A 示例：

```bash
nohup perl /root/minipwa.pl --listen 127.0.0.1:6301 \
  --whitelist /root/auth/iplist.txt \
  --users /root/auth/users.txt \
  --denykey /root/auth/denykey.txt \
  --log /root/s9cron.log >> /root/minipwa.stdout 2>&1 &
```

## 日常更新

```bash
sh /root/setup-sps-v3.sh
kill -HUP $(pgrep -f minipwa.pl)
```

## 鉴权顺序

1. 账密正确 → 通过  
2. 否则客户端 IP 在白名单 → 通过  
3. 查公用 denykey  
4. 拒绝 → NO 日志，最后一列 bytes=0  

多出口时 minipwa 按 `local_addr` 选 `/root/auth/<公网IP>/`，并通过 `eipmap.txt` 返回 `outgoing` 供 joyproxy 绑定出站。

`users.txt` 同一用户名可写多行不同密码，均可通过。

## 检查

```bash
ps aux | grep -E 'minipwa|minipwl|joyproxy'
cat /root/eipmap.txt
ss -lntp | grep -E '2829|3829'
tail -f /root/s9cron.log
```

完整开发进度见 [`PROGRESS.md`](PROGRESS.md)。
