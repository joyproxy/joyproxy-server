package sps

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"joyproxy/internal/authapi"
	"joyproxy/internal/limit"
	"joyproxy/internal/logx"
	"sync/atomic"
)

func appendPort(b []byte, port uint16) []byte {
	return append(b, byte(port>>8), byte(port))
}

const (
	socks5Ver       = 5
	cmdConnect      = 1
	cmdUDPAssociate = 3
	atypIPv4        = 1
	atypDomain      = 3
	atypIPv6        = 4
)

func (s *Server) handleSOCKS5(br *bufio.Reader, c net.Conn, localDisplay, bindIP string) {
	cl := c.RemoteAddr().String()
	if _, err := br.Peek(2); err != nil {
		logx.Debug("socks5 %s peek greet: %v", cl, err)
		return
	}
	b1, _ := br.ReadByte()
	b2, _ := br.ReadByte()
	if b1 != socks5Ver {
		logx.Debug("socks5 %s bad ver %d", cl, b1)
		return
	}
	nmeth := int(b2)
	methods := make([]byte, nmeth)
	if _, err := io.ReadFull(br, methods); err != nil {
		logx.Debug("socks5 %s read methods: %v", cl, err)
		return
	}
	logx.Debug("socks5 %s greet nmeth=%d methods=%v", cl, nmeth, methods)
	wantUser := s.cfg.AuthURL != "" && !s.cfg.AuthNoUser
	hasAuthURL := strings.TrimSpace(s.cfg.AuthURL) != ""
	hasUser := false
	for _, m := range methods {
		if m == 2 {
			hasUser = true
			break
		}
	}
	var user, pass string
	if wantUser && !hasUser {
		logx.Warn("socks5 %s client offered no username/password method", cl)
		_, _ = c.Write([]byte{5, 0xff})
		return
	}
	// --auth-nouser: still negotiate RFC 1929 when the client offers 0x02 so user/pass reach authapi (same as HTTP Proxy-Authorization). Clients that only offer 0x00 keep no-auth.
	useSocksPassword := (wantUser && hasUser) || (s.cfg.AuthNoUser && hasAuthURL && hasUser)
	if useSocksPassword {
		if _, err := c.Write([]byte{5, 2}); err != nil {
			logx.Debug("socks5 %s write method sel: %v", cl, err)
			return
		}
		// Must use = not := here — := would shadow outer user/pass and leave them empty for Authorize().
		u, p, e := socksReadAuth(br)
		if e != nil {
			logx.Debug("socks5 %s subauth read: %v", cl, e)
			return
		}
		user, pass = u, p
		if err := authapi.RequireCreds(s.cfg.AuthURL, s.cfg.AuthNoUser, user, pass); err != nil {
			logx.Warn("socks5 %s creds required: %v", cl, err)
			_, _ = c.Write([]byte{1, 1})
			return
		}
		_, _ = c.Write([]byte{1, 0})
	} else {
		if _, err := c.Write([]byte{5, 0}); err != nil {
			logx.Debug("socks5 %s write no-auth: %v", cl, err)
			return
		}
	}

	// VER, CMD, RSV only — ATYP begins the address field (read by readSocksAddr).
	hd := make([]byte, 3)
	if _, err := io.ReadFull(br, hd); err != nil {
		logx.Debug("socks5 %s read req head: %v", cl, err)
		return
	}
	if hd[0] != socks5Ver {
		logx.Debug("socks5 %s bad req ver %d", cl, hd[0])
		return
	}
	cmd := hd[1]
	if hd[2] != 0 {
		logx.Debug("socks5 %s non-zero rsv %d", cl, hd[2])
		return
	}
	logx.Debug("socks5 %s cmd=%d", cl, cmd)
	if cmd == cmdUDPAssociate {
		s.handleSocksUDPAssociate(br, c, user, pass, localDisplay, bindIP)
		return
	}
	if cmd != cmdConnect {
		logx.Warn("socks5 %s unsupported cmd=%d", cl, cmd)
		socks5Rep(c, 7)
		return
	}
	dst, err := readSocksAddr(br)
	if err != nil {
		logx.Warn("socks5 %s bad address field: %v", cl, err)
		socks5Rep(c, 8)
		return
	}
	target := dst.String()
	logx.Debug("socks5 %s CONNECT target=%s", cl, target)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ok, ar, err := s.auth.Authorize(ctx, user, pass, c.RemoteAddr().String(), localDisplay, target, "socks", "1")
	if err != nil {
		logx.Warn("socks5 %s auth err target=%s: %v", cl, target, err)
		socks5Rep(c, 1)
		return
	}
	if !ok {
		logx.Warn("socks5 %s auth rejected target=%s reply=%d", cl, target, authapi.Socks5RejectReply(ar))
		socks5Rep(c, authapi.Socks5RejectReply(ar))
		return
	}
	if !s.lim.AllowNewConn() {
		logx.Warn("socks5 %s max global new-conn rate", cl)
		socks5Rep(c, 1)
		return
	}
	ipk := limit.IPKey(c.RemoteAddr())
	ukey := authapi.UserKey(s.cfg.AuthNoUser, user)
	if err := s.lim.TryAcquire(ukey, ipk, ar); err != nil {
		logx.Warn("socks5 %s limit acquire: %v", cl, err)
		socks5Rep(c, 1)
		return
	}
	defer s.lim.Release(ukey, ipk, ar)
	upURL := authapi.EffectiveUpstream(ar, s.cfg.DefaultParent)
	out := effectiveOutgoing(ar.Outgoing, bindIP)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()
	logx.Debug("socks5 %s dial target=%s upstream=%q outgoing=%q timeout=30s", cl, target, upURL, out)
	remote, usedUp, err := s.dial.DialTCP(ctx2, target, upURL, out)
	if err != nil {
		logx.Warn("socks5 %s dial failed target=%s upstream=%q: %v", cl, target, usedUp, err)
		socks5Rep(c, 5)
		s.repTrafficSocks(user, localDisplay, c.RemoteAddr().String(), target, usedUp, remote, 0, "")
		return
	}
	logx.Debug("socks5 %s dial ok out_local=%s out_remote=%s", cl, remote.LocalAddr(), remote.RemoteAddr())
	if err := socks5RepOK(c, remote.RemoteAddr()); err != nil {
		logx.Warn("socks5 %s reply OK to client: %v", cl, err)
		remote.Close()
		return
	}
	ut := limit.NewThrottle(pickPerConnRate(ar))
	dt := limit.NewThrottle(pickPerConnRate(ar))
	tag := fmt.Sprintf("socks %s->%s", cl, target)
	t0 := time.Now()
	upB, downB := relay(c, remote, ut, dt, tag)
	logx.Debug("socks5 relay done %s up=%d down=%d total=%d dur=%s", tag, upB, downB, upB+downB, time.Since(t0))
	s.repTrafficSocks(user, localDisplay, c.RemoteAddr().String(), target, usedUp, remote, upB+downB, "")
}

type socksAddr struct {
	host string
	port uint16
}

func (a socksAddr) String() string {
	return net.JoinHostPort(a.host, fmt.Sprintf("%d", a.port))
}

func readSocksAddr(br *bufio.Reader) (socksAddr, error) {
	var a socksAddr
	atyp, err := br.ReadByte()
	if err != nil {
		return a, err
	}
	switch atyp {
	case atypIPv4:
		b := make([]byte, 4)
		if _, err := io.ReadFull(br, b); err != nil {
			return a, err
		}
		a.host = net.IP(b).String()
	case atypDomain:
		n, err := br.ReadByte()
		if err != nil {
			return a, err
		}
		b := make([]byte, n)
		if _, err := io.ReadFull(br, b); err != nil {
			return a, err
		}
		a.host = string(b)
	case atypIPv6:
		b := make([]byte, 16)
		if _, err := io.ReadFull(br, b); err != nil {
			return a, err
		}
		a.host = net.IP(b).String()
	default:
		return a, fmt.Errorf("atyp")
	}
	var pt [2]byte
	if _, err := io.ReadFull(br, pt[:]); err != nil {
		return a, err
	}
	a.port = binary.BigEndian.Uint16(pt[:])
	return a, nil
}

func socksReadAuth(br *bufio.Reader) (user, pass string, err error) {
	h := make([]byte, 2)
	if _, err = io.ReadFull(br, h); err != nil {
		return "", "", err
	}
	if h[0] != 1 {
		return "", "", fmt.Errorf("subnegotiation")
	}
	ulen := int(h[1])
	ub := make([]byte, ulen)
	if _, err = io.ReadFull(br, ub); err != nil {
		return "", "", err
	}
	pl, err := br.ReadByte()
	if err != nil {
		return "", "", err
	}
	pb := make([]byte, int(pl))
	if _, err = io.ReadFull(br, pb); err != nil {
		return "", "", err
	}
	return string(ub), string(pb), nil
}

func socks5Rep(c net.Conn, code byte) {
	_, _ = c.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
}

func socks5RepOK(c net.Conn, ra net.Addr) error {
	ip := net.IPv4zero.To4()
	var port uint16
	if ta, ok := ra.(*net.TCPAddr); ok {
		if ip4 := ta.IP.To4(); ip4 != nil {
			ip = ip4
		}
		port = uint16(ta.Port)
	}
	pkt := []byte{5, 0, 0, 1}
	pkt = append(pkt, ip...)
	pkt = appendPort(pkt, port)
	_, err := c.Write(pkt)
	return err
}

func (s *Server) repTrafficSocks(username, serverAddr, clientAddr, target, upstreamUsed string, remote net.Conn, nbytes int64, sniff string) {
	s.reportTraffic(username, serverAddr, clientAddr, target, upstreamUsed, remote, nbytes, sniff, "socks")
}

func (s *Server) handleSocksUDPAssociate(br *bufio.Reader, c net.Conn, user, pass, localDisplay, bindIP string) {
	cl := c.RemoteAddr().String()
	if _, err := readSocksAddr(br); err != nil {
		logx.Warn("socks5 UDP %s bad relay addr in request: %v", cl, err)
		socks5Rep(c, 1)
		return
	}
	logx.Debug("socks5 UDP ASSOCIATE tcp=%s", cl)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	ok, ar, err := s.auth.Authorize(ctx, user, pass, c.RemoteAddr().String(), localDisplay, "", "socks", "1")
	cancel()
	if err != nil {
		logx.Warn("socks5 UDP %s tcp auth err=%v", cl, err)
		socks5Rep(c, 1)
		return
	}
	if !ok {
		logx.Warn("socks5 UDP %s tcp auth rejected reply=%d", cl, authapi.Socks5RejectReply(ar))
		socks5Rep(c, authapi.Socks5RejectReply(ar))
		return
	}
	if !s.lim.AllowNewConn() {
		logx.Warn("socks5 UDP %s max new-conn rate", cl)
		socks5Rep(c, 1)
		return
	}
	ipk := limit.IPKey(c.RemoteAddr())
	ukey := authapi.UserKey(s.cfg.AuthNoUser, user)
	if err := s.lim.TryAcquire(ukey, ipk, ar); err != nil {
		logx.Warn("socks5 UDP %s limit: %v", cl, err)
		socks5Rep(c, 1)
		return
	}
	defer s.lim.Release(ukey, ipk, ar)

	upURL := authapi.EffectiveUpstream(ar, s.cfg.DefaultParent)
	out := effectiveOutgoing(ar.Outgoing, bindIP)
	ctxUp, cancelUp := context.WithTimeout(context.Background(), 30*time.Second)
	upRelay, usedUp, errUp := s.dial.OpenUDP(ctxUp, upURL, out)
	cancelUp()
	if errUp != nil {
		logx.Warn("socks5 UDP %s upstream open: %v upstream=%q", cl, errUp, usedUp)
		socks5Rep(c, 5)
		return
	}
	if upRelay != nil {
		defer upRelay.Close()
		logx.Debug("socks5 UDP %s relay via upstream=%q", cl, usedUp)
	}

	ip := s.socksClientBindIP(c)
	if ip == nil {
		socks5Rep(c, 8)
		return
	}
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		logx.Warn("socks5 UDP %s ListenUDP: %v", cl, err)
		socks5Rep(c, 1)
		return
	}
	uap := udpConn.LocalAddr().(*net.UDPAddr)
	logx.Debug("socks5 UDP %s relay udp bind :%d (tcp bnd ip %v)", cl, uap.Port, ip)
	pkt := []byte{5, 0, 0, 1}
	pkt = append(pkt, ip.To4()...)
	pkt = appendPort(pkt, uint16(uap.Port))
	if _, err := c.Write(pkt); err != nil {
		logx.Warn("socks5 UDP %s write reply: %v", cl, err)
		udpConn.Close()
		return
	}

	done := make(chan struct{})
	var total int64
	go func() {
		defer close(done)
		buf := make([]byte, 65535)
		for {
			_ = udpConn.SetReadDeadline(time.Now().Add(2 * time.Minute))
			n, raddr, err := udpConn.ReadFromUDP(buf)
			if err != nil {
				logx.Debug("socks5 UDP %s ReadFrom: %v", cl, err)
				return
			}
			if n < 10 {
				continue
			}
			hdr := buf[:n]
			if hdr[2] != 0 {
				continue
			}
			dst, rest, err := parseSocksUDPDest(hdr[3:])
			if err != nil || len(rest) == 0 {
				continue
			}
			ctxa, ca := context.WithTimeout(context.Background(), 15*time.Second)
			ok2, _, _ := s.auth.Authorize(ctxa, user, pass, raddr.String(), localDisplay, dst, "socks", "1")
			ca()
			if !ok2 {
				logx.Debug("socks5 UDP packet auth fail dst=%s from=%s", dst, raddr)
				continue
			}
			var rep []byte
			if upRelay != nil {
				if err := upRelay.WriteTo(dst, rest); err != nil {
					logx.Debug("socks5 UDP upstream write %s: %v", dst, err)
					continue
				}
				peerIP, peerPort, payload, err := upRelay.Read(30 * time.Second)
				if err != nil || len(payload) == 0 {
					logx.Debug("socks5 UDP upstream read %s: n=%d err=%v", dst, len(payload), err)
					continue
				}
				rep = buildSocksUDPReply(peerIP, peerPort, payload)
				atomic.AddInt64(&total, int64(n+len(payload)+len(rep)))
			} else {
				rmt, err := net.Dial("udp", dst)
				if err != nil {
					logx.Debug("socks5 UDP dial %s: %v", dst, err)
					continue
				}
				_, _ = rmt.Write(rest)
				_ = rmt.SetReadDeadline(time.Now().Add(30 * time.Second))
				rb := make([]byte, 65535)
				nr, err := rmt.Read(rb)
				peer := rmt.RemoteAddr()
				rmt.Close()
				if err != nil || nr <= 0 {
					logx.Debug("socks5 UDP read from %s: n=%d err=%v", dst, nr, err)
					continue
				}
				if ua, ok := peer.(*net.UDPAddr); ok {
					rep = buildSocksUDPReply(ua.IP, uint16(ua.Port), rb[:nr])
				} else {
					rep = buildSocksUDPReply(net.IPv4(0, 0, 0, 0), 0, rb[:nr])
				}
				atomic.AddInt64(&total, int64(n+nr+len(rep)))
			}
			_, _ = udpConn.WriteToUDP(rep, raddr)
		}
	}()
	buf := make([]byte, 1)
	_, _ = c.Read(buf)
	logx.Debug("socks5 UDP %s tcp control closed, stop relay", cl)
	udpConn.Close()
	<-done
	tb := atomic.LoadInt64(&total)
	logx.Debug("socks5 UDP %s session end total_bytes=%d", cl, tb)
	s.repTrafficSocks(user, localDisplay, c.RemoteAddr().String(), "", usedUp, nil, tb, "")
}

func parseSocksUDPDest(b []byte) (addr string, payload []byte, err error) {
	if len(b) < 4 {
		return "", nil, fmt.Errorf("short")
	}
	atyp := b[0]
	off := 1
	var host string
	switch atyp {
	case atypIPv4:
		if len(b) < off+4+2 {
			return "", nil, fmt.Errorf("short")
		}
		host = net.IP(b[off : off+4]).String()
		off += 4
	case atypDomain:
		l := int(b[off])
		off++
		if len(b) < off+l+2 {
			return "", nil, fmt.Errorf("short")
		}
		host = string(b[off : off+l])
		off += l
	case atypIPv6:
		if len(b) < off+16+2 {
			return "", nil, fmt.Errorf("short")
		}
		host = net.IP(b[off : off+16]).String()
		off += 16
	default:
		return "", nil, fmt.Errorf("atyp")
	}
	port := binary.BigEndian.Uint16(b[off : off+2])
	off += 2
	return net.JoinHostPort(host, fmt.Sprintf("%d", port)), b[off:], nil
}

// socksClientBindIP is the IPv4 address told to the client in UDP ASSOCIATE replies.
// Prefer -g (public IP); falling back to the listener address breaks remote clients on NAT hosts.
func (s *Server) socksClientBindIP(c net.Conn) net.IP {
	if g := strings.TrimSpace(s.cfg.GatewayIP); g != "" {
		if ip := net.ParseIP(g); ip != nil {
			if ip4 := ip.To4(); ip4 != nil {
				return ip4
			}
		}
	}
	if ta, ok := c.LocalAddr().(*net.TCPAddr); ok {
		if ip4 := ta.IP.To4(); ip4 != nil {
			return ip4
		}
	}
	return nil
}

func buildSocksUDPReply(dstIP net.IP, dstPort uint16, payload []byte) []byte {
	ip4 := dstIP.To4()
	if ip4 == nil {
		ip4 = net.IPv4(127, 0, 0, 1)
	}
	out := make([]byte, 0, 10+len(payload))
	out = append(out, 0, 0, 0, 1)
	out = append(out, ip4...)
	out = appendPort(out, dstPort)
	out = append(out, payload...)
	return out
}
