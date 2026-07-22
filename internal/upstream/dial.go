package upstream

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Dialer struct {
	ServerType string
}

// normalizeProxyURL turns "host:port" or "host" into "http://..." so url.Parse gets a scheme.
// Empty input stays empty (caller direct-dials).
func normalizeProxyURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		return raw
	}
	return "http://" + raw
}

// sentinelProxyHostname reports placeholder hosts from auth/upstream APIs (e.g. "ERR")
// that must not hit DNS or CONNECT. Treat as "no parent": direct-dial target.
func sentinelProxyHostname(h string) bool {
	h = strings.ToLower(strings.TrimSpace(h))
	if h == "" {
		return true
	}
	switch h {
	case "err", "error", "fail", "failed", "ng", "no", "none", "null",
		"invalid", "bad", "na", "false", "off", "dummy", "deny", "blocked", "timeout":
		return true
	default:
		return false
	}
}

func (d *Dialer) DialTCP(ctx context.Context, target string, upstreamRaw string, outgoing string) (net.Conn, string, error) {
	upstreamRaw = strings.TrimSpace(upstreamRaw)
	dial := net.Dialer{}
	if outgoing != "" {
		if ip := net.ParseIP(outgoing); ip != nil {
			dial.LocalAddr = &net.TCPAddr{IP: ip}
		}
	}
	if upstreamRaw == "" {
		c, err := dial.DialContext(ctx, "tcp", target)
		return c, "", err
	}
	upstreamRaw = normalizeProxyURL(upstreamRaw)
	u, err := url.Parse(upstreamRaw)
	if err != nil {
		return nil, "", err
	}
	if sentinelProxyHostname(u.Hostname()) {
		return nil, upstreamRaw, fmt.Errorf("upstream denied or invalid host %q", u.Hostname())
	}
	st := strings.ToLower(strings.TrimSpace(d.ServerType))
	if st == "" {
		st = "http"
	}
	switch u.Scheme {
	case "http", "https":
		c, err := dial.DialContext(ctx, "tcp", hostPort(u))
		if err != nil {
			return nil, upstreamRaw, err
		}
		if err := httpConnect(ctx, c, u, target); err != nil {
			c.Close()
			return nil, upstreamRaw, err
		}
		return c, upstreamRaw, nil
	case "socks5":
		c, err := dial.DialContext(ctx, "tcp", hostPort(u))
		if err != nil {
			return nil, upstreamRaw, err
		}
		if err := socks5Connect(ctx, c, u, target); err != nil {
			c.Close()
			return nil, upstreamRaw, err
		}
		return c, upstreamRaw, nil
	default:
		return nil, upstreamRaw, fmt.Errorf("unsupported upstream scheme %q", u.Scheme)
	}
}

func hostPort(u *url.URL) string {
	h := u.Hostname()
	p := u.Port()
	if p == "" {
		if u.Scheme == "https" {
			p = "443"
		} else {
			p = "80"
		}
	}
	return net.JoinHostPort(h, p)
}

func httpConnect(ctx context.Context, c net.Conn, parent *url.URL, target string) error {
	user := parent.User.Username()
	pass, _ := parent.User.Password()
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target)
	if user != "" || pass != "" {
		b := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		req += "Proxy-Authorization: Basic " + b + "\r\n"
	}
	req += "\r\n"
	deadline, ok := ctx.Deadline()
	if ok {
		_ = c.SetDeadline(deadline)
	}
	if _, err := io.WriteString(c, req); err != nil {
		return err
	}
	br := bufio.NewReader(c)
	line, err := br.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.Contains(line, "200") {
		return fmt.Errorf("CONNECT failed: %s", strings.TrimSpace(line))
	}
	for {
		hline, err := br.ReadString('\n')
		if err != nil {
			return err
		}
		if hline == "\r\n" || hline == "\n" {
			break
		}
	}
	// 握手阶段用了 SetDeadline(ctx)；隧道建立后必须清除，否则长连接会在「拨号截止时间」到达时误报 i/o timeout。
	_ = c.SetDeadline(time.Time{})
	return nil
}

func socks5Authenticate(ctx context.Context, c net.Conn, parent *url.URL) error {
	deadline, ok := ctx.Deadline()
	if ok {
		_ = c.SetDeadline(deadline)
	}
	user := parent.User.Username()
	pass, _ := parent.User.Password()
	if user != "" || pass != "" {
		if _, err := c.Write([]byte{5, 2, 0, 2}); err != nil {
			return err
		}
	} else {
		if _, err := c.Write([]byte{5, 1, 0}); err != nil {
			return err
		}
	}
	buf := make([]byte, 256)
	if _, err := io.ReadFull(c, buf[:2]); err != nil {
		return err
	}
	if buf[0] != 5 {
		return fmt.Errorf("socks5 bad version")
	}
	if buf[1] == 0xff {
		return fmt.Errorf("socks5 no acceptable method")
	}
	if buf[1] == 2 {
		ub := []byte(user)
		pb := []byte(pass)
		if len(ub) > 255 || len(pb) > 255 {
			return fmt.Errorf("socks5 auth too long")
		}
		pkt := make([]byte, 0, 3+len(ub)+len(pb))
		pkt = append(pkt, 1, byte(len(ub)))
		pkt = append(pkt, ub...)
		pkt = append(pkt, byte(len(pb)))
		pkt = append(pkt, pb...)
		if _, err := c.Write(pkt); err != nil {
			return err
		}
		if _, err := io.ReadFull(c, buf[:2]); err != nil {
			return err
		}
		if buf[1] != 0 {
			return fmt.Errorf("socks5 auth failed")
		}
	} else if buf[1] != 0 {
		return fmt.Errorf("socks5 unexpected method %d", buf[1])
	}
	return nil
}

func socks5Connect(ctx context.Context, c net.Conn, parent *url.URL, target string) error {
	if err := socks5Authenticate(ctx, c, parent); err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	if ok {
		_ = c.SetDeadline(deadline)
	}
	buf := make([]byte, 256)
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return err
	}
	var addr []byte
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			addr = make([]byte, 1+net.IPv4len+2)
			addr[0] = 1
			copy(addr[1:], ip4)
		} else {
			addr = make([]byte, 1+net.IPv6len+2)
			addr[0] = 4
			copy(addr[1:], ip.To16())
		}
	} else {
		if len(host) > 255 {
			return fmt.Errorf("host too long")
		}
		addr = make([]byte, 1+1+len(host)+2)
		addr[0] = 3
		addr[1] = byte(len(host))
		copy(addr[2:], host)
	}
	addr[len(addr)-2] = byte(port >> 8)
	addr[len(addr)-1] = byte(port)
	req := make([]byte, 0, 6+len(addr))
	req = append(req, 5, 1, 0)
	req = append(req, addr...)
	if _, err := c.Write(req); err != nil {
		return err
	}
	if _, err := io.ReadFull(c, buf[:4]); err != nil {
		return err
	}
	if buf[1] != 0 {
		return fmt.Errorf("socks5 connect failed code=%d", buf[1])
	}
	atyp := buf[3]
	var tail int
	switch atyp {
	case 1:
		tail = 4 + 2
	case 3:
		if _, err := io.ReadFull(c, buf[:1]); err != nil {
			return err
		}
		tail = int(buf[0]) + 2
	case 4:
		tail = 16 + 2
	default:
		return fmt.Errorf("socks5 bad atyp")
	}
	if _, err := io.ReadFull(c, buf[:tail]); err != nil {
		return err
	}
	_ = c.SetDeadline(time.Time{})
	return nil
}
