#!/usr/bin/env node
/**
 * UDP 回显：把服务器看到的访问者地址发回客户端。
 *
 *   node udp_echo_client_ip.js
 *   node udp_echo_client_ip.js --port 21394
 *
 * 代理侧若只允许访问目标 53/123 等端口，可监听 53（Linux 需 root）:
 *   sudo node udp_echo_client_ip.js --port 53
 *
 * 注意: 本机若已跑 systemd-resolved/named，53 可能被占用，先执行:
 *   ss -ulnp | grep ':53 '
 */
const dgram = require("dgram");

const args = process.argv.slice(2);
function getArg(name, fallback) {
  const i = args.indexOf(name);
  if (i >= 0 && args[i + 1] != null) return args[i + 1];
  return fallback;
}

const host = getArg("--host", "0.0.0.0");
const port = parseInt(getArg("--port", "21394"), 10);

const server = dgram.createSocket("udp4");

server.on("error", (err) => {
  console.error("server error:", err);
  process.exit(1);
});

server.on("message", (msg, rinfo) => {
  const ts = new Date().toISOString();
  const preview = msg.length > 80 ? msg.subarray(0, 80) : msg;
  const body =
    `seen=${rinfo.address}:${rinfo.port} ` +
    `bytes=${msg.length} ` +
    `payload=${JSON.stringify(preview.toString("utf8"))} ` +
    `time=${ts}\n`;

  server.send(body, rinfo.port, rinfo.address, (err) => {
    if (err) console.error("send error:", err);
    else console.log(`<- ${rinfo.address}:${rinfo.port}  ${msg.length}B  -> replied`);
  });
});

server.on("listening", () => {
  const a = server.address();
  console.log(`listening udp ${a.address}:${a.port}  (Ctrl+C stop)`);
});

server.bind(port, host);
