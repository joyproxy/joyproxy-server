package sps

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"joyproxy/internal/authapi"
	"joyproxy/internal/limit"
	"joyproxy/internal/logx"
	"joyproxy/internal/upstream"
)

func parseBasicProxyAuth(h http.Header) (user, pass string) {
	v := h.Get("Proxy-Authorization")
	if v == "" {
		return "", ""
	}
	if !strings.HasPrefix(strings.ToLower(v), "basic ") {
		return "", ""
	}
	raw := strings.TrimSpace(v[6:])
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", ""
	}
	i := strings.IndexByte(string(b), ':')
	if i < 0 {
		return string(b), ""
	}
	return string(b[:i]), string(b[i+1:])
}

type bufConn struct {
	net.Conn
	br *bufio.Reader
}

func (b *bufConn) Read(p []byte) (int, error) {
	if b.br != nil && b.br.Buffered() > 0 {
		return b.br.Read(p)
	}
	return b.Conn.Read(p)
}

func (s *Server) handleHTTP(br *bufio.Reader, c net.Conn, localDisplay, bindIP string) {
	req, err := http.ReadRequest(br)
	if err != nil {
		logx.Debug("http read request from %s: %v", c.RemoteAddr(), err)
		return
	}
	defer req.Body.Close()
	user, pass := parseBasicProxyAuth(req.Header)
	if err := authapi.RequireCreds(s.cfg.AuthURL, s.cfg.AuthNoUser, user, pass); err != nil {
		io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm=\"joyproxy\"\r\n\r\n")
		return
	}
	targetAuth := ""
	if strings.EqualFold(req.Method, http.MethodConnect) {
		targetAuth = targetForAuthHTTP(req.Host, req.Method, "")
	} else if req.URL != nil {
		targetAuth = req.URL.String()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ok, ar, err := s.auth.Authorize(ctx, user, pass, c.RemoteAddr().String(), localDisplay, targetAuth, "http", "1")
	if err != nil {
		logx.Warn("http auth error client=%s err=%v", c.RemoteAddr(), err)
		io.WriteString(c, "HTTP/1.1 503 Service Unavailable\r\n\r\n")
		return
	}
	if !ok {
		st := authapi.ClientHTTPStatus(ar)
		logx.Warn("http auth rejected client=%s status=%d targetAuth=%q", c.RemoteAddr(), st, targetAuth)
		switch st {
		case http.StatusProxyAuthRequired:
			io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm=\"joyproxy\"\r\n\r\n")
		case http.StatusTooManyRequests:
			io.WriteString(c, "HTTP/1.1 429 Too Many Requests\r\n\r\n")
		default:
			io.WriteString(c, "HTTP/1.1 503 Service Unavailable\r\n\r\n")
		}
		return
	}
	if !s.lim.AllowNewConn() {
		io.WriteString(c, "HTTP/1.1 503 Service Unavailable\r\n\r\n")
		return
	}
	ipk := limit.IPKey(c.RemoteAddr())
	ukey := authapi.UserKey(s.cfg.AuthNoUser, user)
	if err := s.lim.TryAcquire(ukey, ipk, ar); err != nil {
		io.WriteString(c, "HTTP/1.1 503 Service Unavailable\r\n\r\n")
		return
	}
	defer s.lim.Release(ukey, ipk, ar)
	upURL := authapi.EffectiveUpstream(ar, s.cfg.DefaultParent)
	out := effectiveOutgoing(ar.Outgoing, bindIP)

	var sniff string
	if s.cfg.SniffDomain && strings.EqualFold(req.Method, http.MethodConnect) {
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		peek, _ := br.Peek(512)
		_ = c.SetReadDeadline(time.Time{})
		if len(peek) > 0 {
			sniff = sniffSNI(peek)
		}
	}

	cc := &bufConn{Conn: c, br: br}
	logx.Debug("http %s %s targetAuth=%q", req.Method, c.RemoteAddr(), targetAuth)
	s.relayHTTP(req, cc, user, pass, localDisplay, upURL, out, ar, sniff)
}

func (s *Server) relayHTTP(req *http.Request, c net.Conn, user, pass, localDisplay, upURL, outgoing string, ar authapi.Result, sniff string) {
	d := &upstream.Dialer{ServerType: s.cfg.ServerType}
	target := ""
	if strings.EqualFold(req.Method, http.MethodConnect) {
		target = req.Host
		if !strings.Contains(target, ":") {
			target = net.JoinHostPort(target, "443")
		}
	} else {
		if req.URL == nil || req.URL.Host == "" {
			io.WriteString(c, "HTTP/1.1 400 Bad Request\r\n\r\n")
			return
		}
		hst := req.URL.Host
		if !strings.Contains(hst, ":") {
			if req.URL.Scheme == "https" {
				hst = net.JoinHostPort(hst, "443")
			} else {
				hst = net.JoinHostPort(hst, "80")
			}
		}
		target = hst
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	remote, usedUp, err := d.DialTCP(ctx, target, upURL, outgoing)
	if err != nil {
		logx.Warn("http client=%s target=%s upstream=%q: %v", c.RemoteAddr(), target, usedUp, err)
		io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\n\r\n")
		s.reportTraffic(user, localDisplay, c.RemoteAddr().String(), target, usedUp, remote, 0, sniff, "http")
		return
	}
	logx.Debug("http dial ok client=%s target=%s out_local=%s out_remote=%s",
		c.RemoteAddr(), target, remote.LocalAddr(), remote.RemoteAddr())
	if strings.EqualFold(req.Method, http.MethodConnect) {
		io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
	} else {
		if err := req.Write(remote); err != nil {
			remote.Close()
			s.reportTraffic(user, localDisplay, c.RemoteAddr().String(), target, usedUp, remote, 0, sniff, "http")
			return
		}
	}
	ut := limit.NewThrottle(pickPerConnRate(ar))
	dt := limit.NewThrottle(pickPerConnRate(ar))
	tag := fmt.Sprintf("http %s->%s", c.RemoteAddr(), target)
	upB, downB := relay(c, remote, ut, dt, tag)
	logx.Debug("http relay done %s up=%d down=%d total=%d", tag, upB, downB, upB+downB)
	s.reportTraffic(user, localDisplay, c.RemoteAddr().String(), target, usedUp, remote, upB+downB, sniff, "http")
}

func pickPerConnRate(ar authapi.Result) int64 {
	if ar.UserRate > 0 {
		return ar.UserRate
	}
	return ar.IPRate
}

func (s *Server) reportTraffic(username, serverAddr, clientAddr, target, upstreamUsed string, remote net.Conn, nbytes int64, sniff, id string) {
	if s.rep == nil {
		return
	}
	v := url.Values{}
	v.Set("act", "traffic")
	v.Set("bytes", fmt.Sprintf("%d", nbytes))
	v.Set("client_addr", clientAddr)
	v.Set("id", id)
	v.Set("server_addr", serverAddr)
	v.Set("target_addr", target)
	v.Set("username", username)
	v.Set("upstream", upstreamUsed)
	if remote != nil && remote.LocalAddr() != nil {
		v.Set("out_local_addr", remote.LocalAddr().String())
	}
	if remote != nil && remote.RemoteAddr() != nil {
		v.Set("out_remote_addr", remote.RemoteAddr().String())
	}
	if sniff != "" {
		v.Set("sniff_domain", sniff)
	}
	s.rep.Report(v)
}
