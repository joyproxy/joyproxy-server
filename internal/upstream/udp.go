package upstream

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// UDPRelay carries client UDP through a SOCKS5 parent's UDP ASSOCIATE.
// The parent TCP control connection must stay open until Close.
type UDPRelay struct {
	ctrl  net.Conn
	relay *net.UDPAddr
	sock  *net.UDPConn
}

func (r *UDPRelay) Close() error {
	if r == nil {
		return nil
	}
	var err1, err2 error
	if r.sock != nil {
		err1 = r.sock.Close()
	}
	if r.ctrl != nil {
		err2 = r.ctrl.Close()
	}
	if err1 != nil {
		return err1
	}
	return err2
}

// OpenUDP opens a SOCKS5 UDP ASSOCIATE to upstreamRaw (socks5:// only).
// Empty upstreamRaw returns (nil, "", nil) meaning direct UDP dial.
func (d *Dialer) OpenUDP(ctx context.Context, upstreamRaw, outgoing string) (*UDPRelay, string, error) {
	upstreamRaw = strings.TrimSpace(upstreamRaw)
	if upstreamRaw == "" {
		return nil, "", nil
	}
	upstreamRaw = normalizeProxyURL(upstreamRaw)
	u, err := url.Parse(upstreamRaw)
	if err != nil {
		return nil, "", err
	}
	if sentinelProxyHostname(u.Hostname()) {
		return nil, upstreamRaw, fmt.Errorf("upstream denied or invalid host %q", u.Hostname())
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return nil, upstreamRaw, fmt.Errorf("udp requires socks5 upstream, got %q", u.Scheme)
	case "socks5":
		// continue
	default:
		return nil, upstreamRaw, fmt.Errorf("unsupported upstream scheme %q for udp", u.Scheme)
	}

	nd := net.Dialer{}
	if outgoing != "" {
		if ip := net.ParseIP(outgoing); ip != nil {
			nd.LocalAddr = &net.TCPAddr{IP: ip}
		}
	}
	ctrl, err := nd.DialContext(ctx, "tcp", hostPort(u))
	if err != nil {
		return nil, upstreamRaw, err
	}
	if err := socks5Authenticate(ctx, ctrl, u); err != nil {
		ctrl.Close()
		return nil, upstreamRaw, err
	}
	relay, err := socks5UDPAssociate(ctx, ctrl)
	if err != nil {
		ctrl.Close()
		return nil, upstreamRaw, err
	}
	udpSock, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		ctrl.Close()
		return nil, upstreamRaw, err
	}
	if outgoing != "" {
		if ip := net.ParseIP(outgoing); ip != nil {
			_ = udpSock.Close()
			udpSock, err = net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: 0})
			if err != nil {
				ctrl.Close()
				return nil, upstreamRaw, err
			}
		}
	}
	return &UDPRelay{ctrl: ctrl, relay: relay, sock: udpSock}, upstreamRaw, nil
}

func (r *UDPRelay) WriteTo(target string, payload []byte) error {
	if r == nil || r.sock == nil || r.relay == nil {
		return fmt.Errorf("udp relay not open")
	}
	hdr, err := encodeSocksUDPAddr(target)
	if err != nil {
		return err
	}
	pkt := make([]byte, 0, 3+len(hdr)+len(payload))
	pkt = append(pkt, 0, 0, 0)
	pkt = append(pkt, hdr...)
	pkt = append(pkt, payload...)
	_, err = r.sock.WriteToUDP(pkt, r.relay)
	return err
}

func (r *UDPRelay) Read(timeout time.Duration) (peerIP net.IP, peerPort uint16, payload []byte, err error) {
	if r == nil || r.sock == nil {
		return nil, 0, nil, fmt.Errorf("udp relay not open")
	}
	_ = r.sock.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 65535)
	n, _, err := r.sock.ReadFromUDP(buf)
	if err != nil {
		return nil, 0, nil, err
	}
	if n < 10 {
		return nil, 0, nil, fmt.Errorf("short socks udp reply")
	}
	if buf[2] != 0 {
		return nil, 0, nil, fmt.Errorf("non-zero rsv")
	}
	host, port, rest, err := decodeSocksUDPAddr(buf[3:n])
	if err != nil {
		return nil, 0, nil, err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		ip = net.IPv4zero
	}
	return ip, port, rest, nil
}

func socks5UDPAssociate(ctx context.Context, c net.Conn) (*net.UDPAddr, error) {
	deadline, ok := ctx.Deadline()
	if ok {
		_ = c.SetDeadline(deadline)
	}
	// ASSOCIATE to 0.0.0.0:0
	req := []byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0}
	if _, err := c.Write(req); err != nil {
		return nil, err
	}
	buf := make([]byte, 256)
	if _, err := io.ReadFull(c, buf[:4]); err != nil {
		return nil, err
	}
	if buf[0] != 5 {
		return nil, fmt.Errorf("socks5 bad version")
	}
	if buf[1] != 0 {
		return nil, fmt.Errorf("socks5 udp associate failed code=%d", buf[1])
	}
	atyp := buf[3]
	var host string
	var port uint16
	switch atyp {
	case 1:
		if _, err := io.ReadFull(c, buf[:6]); err != nil {
			return nil, err
		}
		host = net.IP(buf[:4]).String()
		port = binary.BigEndian.Uint16(buf[4:6])
	case 3:
		if _, err := io.ReadFull(c, buf[:1]); err != nil {
			return nil, err
		}
		ln := int(buf[0])
		if _, err := io.ReadFull(c, buf[:ln+2]); err != nil {
			return nil, err
		}
		host = string(buf[:ln])
		port = binary.BigEndian.Uint16(buf[ln : ln+2])
	case 4:
		if _, err := io.ReadFull(c, buf[:18]); err != nil {
			return nil, err
		}
		host = net.IP(buf[:16]).String()
		port = binary.BigEndian.Uint16(buf[16:18])
	default:
		return nil, fmt.Errorf("socks5 bad atyp %d", atyp)
	}
	_ = c.SetDeadline(time.Time{})
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("socks5 bad bind host %q", host)
	}
	// A wildcard or loopback BND.ADDR means "reuse the control connection's address"
	// (RFC 1928 leaves this to the client). Taking it literally sends every relayed
	// datagram to our own loopback — note that on a dual-stack socket Go rewrites a
	// 0.0.0.0 destination to ::, so the packets surface as ::1 rather than 127.0.0.1.
	if ip.IsUnspecified() || ip.IsLoopback() {
		ta, ok := c.RemoteAddr().(*net.TCPAddr)
		if !ok || ta.IP == nil {
			return nil, fmt.Errorf("socks5 unusable bind host %q and no control peer address", host)
		}
		ip = ta.IP
	}
	return &net.UDPAddr{IP: ip, Port: int(port)}, nil
}

func encodeSocksUDPAddr(target string) ([]byte, error) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			out := make([]byte, 1+net.IPv4len+2)
			out[0] = 1
			copy(out[1:], ip4)
			binary.BigEndian.PutUint16(out[1+net.IPv4len:], uint16(port))
			return out, nil
		}
		out := make([]byte, 1+net.IPv6len+2)
		out[0] = 4
		copy(out[1:], ip.To16())
		binary.BigEndian.PutUint16(out[1+net.IPv6len:], uint16(port))
		return out, nil
	}
	if len(host) > 255 {
		return nil, fmt.Errorf("host too long")
	}
	out := make([]byte, 1+1+len(host)+2)
	out[0] = 3
	out[1] = byte(len(host))
	copy(out[2:], host)
	binary.BigEndian.PutUint16(out[2+len(host):], uint16(port))
	return out, nil
}

func decodeSocksUDPAddr(b []byte) (host string, port uint16, payload []byte, err error) {
	if len(b) < 4 {
		return "", 0, nil, fmt.Errorf("short")
	}
	atyp := b[0]
	off := 1
	switch atyp {
	case 1:
		if len(b) < off+4+2 {
			return "", 0, nil, fmt.Errorf("short")
		}
		host = net.IP(b[off : off+4]).String()
		off += 4
	case 3:
		if len(b) < off+1 {
			return "", 0, nil, fmt.Errorf("short")
		}
		ln := int(b[off])
		off++
		if len(b) < off+ln+2 {
			return "", 0, nil, fmt.Errorf("short")
		}
		host = string(b[off : off+ln])
		off += ln
	case 4:
		if len(b) < off+16+2 {
			return "", 0, nil, fmt.Errorf("short")
		}
		host = net.IP(b[off : off+16]).String()
		off += 16
	default:
		return "", 0, nil, fmt.Errorf("atyp")
	}
	port = binary.BigEndian.Uint16(b[off : off+2])
	off += 2
	return host, port, b[off:], nil
}
