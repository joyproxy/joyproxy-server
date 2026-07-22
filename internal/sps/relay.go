package sps

import (
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"joyproxy/internal/limit"
	"joyproxy/internal/logx"
)

// listenerBindIP returns the concrete local IP when -p binds a specific interface.
func listenerBindIP(ln net.Listener) string {
	ta, ok := ln.Addr().(*net.TCPAddr)
	if !ok || ta == nil || ta.IP == nil || ta.IP.IsUnspecified() {
		return ""
	}
	return ta.IP.String()
}

// effectiveOutgoing prefers auth API outgoing; falls back to listener bind IP for multi-egress.
func effectiveOutgoing(authOut, bindIP string) string {
	if s := strings.TrimSpace(authOut); s != "" {
		return s
	}
	if ip := net.ParseIP(bindIP); ip != nil && !ip.IsUnspecified() {
		return ip.String()
	}
	return ""
}

func relay(a, b net.Conn, upLim, downLim *limit.Throttle, tag string) (int64, int64) {
	var upBytes int64
	done := make(chan struct{})
	var upErr error
	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			_ = a.Close()
			_ = b.Close()
		})
	}
	go func() {
		n, err := copyOneWay(a, b, upLim)
		atomic.StoreInt64(&upBytes, n)
		upErr = err
		closeBoth()
		close(done)
	}()
	n2, downErr := copyOneWay(b, a, downLim)
	closeBoth()
	<-done
	if tag != "" {
		logx.RelayLeg(tag+" c->remote", atomic.LoadInt64(&upBytes), upErr)
		logx.RelayLeg(tag+" remote->c", n2, downErr)
	} else {
		logx.RelayLeg("c->remote", atomic.LoadInt64(&upBytes), upErr)
		logx.RelayLeg("remote->c", n2, downErr)
	}
	return atomic.LoadInt64(&upBytes), n2
}

func copyOneWay(dst, src net.Conn, t *limit.Throttle) (int64, error) {
	var n int64
	buf := make([]byte, 32*1024)
	var w io.Writer = dst
	var r io.Reader = src
	if t != nil {
		w = &limit.Writer{W: dst, T: t}
	}
	for {
		nr, er := r.Read(buf)
		if nr > 0 {
			nw, ew := w.Write(buf[:nr])
			n += int64(nw)
			if ew != nil {
				return n, ew
			}
			if nw != nr {
				return n, io.ErrShortWrite
			}
		}
		if er != nil {
			return n, er
		}
	}
}
