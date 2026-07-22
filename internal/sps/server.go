package sps

import (
	"bufio"
	"fmt"
	"net"
	"strings"

	"joyproxy/internal/authapi"
	"joyproxy/internal/config"
	"joyproxy/internal/limit"
	"joyproxy/internal/logx"
	"joyproxy/internal/traffic"
	"joyproxy/internal/upstream"
)

type Server struct {
	cfg  *config.SPS
	auth *authapi.Client
	rep  *traffic.Reporter
	lim  *limit.Manager
	dial *upstream.Dialer
}

func NewServer(cfg *config.SPS) *Server {
	s := &Server{
		cfg:  cfg,
		auth: authapi.New(cfg.AuthURL, cfg.AuthCache, cfg.AuthFailCache),
		lim:  limit.NewManager(cfg.MaxConnsRate),
		dial: &upstream.Dialer{ServerType: cfg.ServerType},
	}
	if cfg.TrafficURL != "" {
		s.rep = traffic.New(cfg.TrafficURL)
	}
	return s
}

func (s *Server) Run() error {
	logx.Init(s.cfg.LogLevel)
	ranges, err := ParsePortSpec(s.cfg.PortSpec)
	if err != nil {
		return err
	}
	for _, pr := range ranges {
		host := pr.Host
		if host == "" {
			host = "0.0.0.0"
		}
		for port := pr.Start; port <= pr.End; port++ {
			addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("listen %s: %w", addr, err)
			}
			localDisplay := listenerDisplay(s.cfg.GatewayIP, port)
			bindIP := listenerBindIP(ln)
			logx.Startup("listening tcp %s (auth local_addr %s bind_out %s) log=%s", addr, localDisplay, bindIP, logLevelName(s.cfg.LogLevel))
			go s.acceptLoop(ln, localDisplay, bindIP)
		}
	}
	block := make(chan struct{})
	<-block
	return nil
}

func listenerDisplay(gateway string, port int) string {
	gateway = strings.TrimSpace(gateway)
	if gateway != "" {
		return net.JoinHostPort(gateway, fmt.Sprintf("%d", port))
	}
	return fmt.Sprintf("0.0.0.0:%d", port)
}

func (s *Server) acceptLoop(ln net.Listener, localDisplay, bindIP string) {
	for {
		c, err := ln.Accept()
		if err != nil {
			logx.Error("accept: %v", err)
			return
		}
		go s.handleConn(c, localDisplay, bindIP)
	}
}

func logLevelName(l int) string {
	switch l {
	case logx.LevelSilent:
		return "silent"
	case logx.LevelErrorsOnly:
		return "errors"
	case logx.LevelQuiet:
		return "quiet"
	case logx.LevelNormal:
		return "normal"
	case logx.LevelVerbose:
		return "verbose"
	default:
		return "?"
	}
}

func (s *Server) handleConn(c net.Conn, localDisplay, bindIP string) {
	defer c.Close()
	br := bufio.NewReader(c)
	b, err := br.Peek(1)
	if err != nil {
		logx.Debug("peek first byte from %s: %v", c.RemoteAddr(), err)
		return
	}
	if b[0] == 0x05 {
		logx.Debug("proto=socks5 client=%s", c.RemoteAddr())
		s.handleSOCKS5(br, c, localDisplay, bindIP)
		return
	}
	logx.Debug("proto=http client=%s first=%q", c.RemoteAddr(), string(b[0]))
	s.handleHTTP(br, c, localDisplay, bindIP)
}
