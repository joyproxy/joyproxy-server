package limit

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"joyproxy/internal/authapi"
)

type Manager struct {
	maxConnsRate int64
	secondStart  time.Time
	secondCount  int64

	mu          sync.Mutex
	userConns   map[string]int64
	ipConns     map[string]int64
	userQPS     map[string]*qpsWindow
	ipQPS       map[string]*qpsWindow
}

type qpsWindow struct {
	sec int64
	n   int64
}

func NewManager(maxConnsRatePerSec int) *Manager {
	var m int64
	if maxConnsRatePerSec > 0 {
		m = int64(maxConnsRatePerSec)
	}
	return &Manager{
		maxConnsRate: m,
		secondStart:  time.Now(),
		userConns:    make(map[string]int64),
		ipConns:      make(map[string]int64),
		userQPS:      make(map[string]*qpsWindow),
		ipQPS:        make(map[string]*qpsWindow),
	}
}

func (m *Manager) AllowNewConn() bool {
	if m == nil || m.maxConnsRate == 0 {
		return true
	}
	now := time.Now()
	if now.Sub(m.secondStart) >= time.Second {
		atomic.StoreInt64(&m.secondCount, 0)
		m.secondStart = now
	}
	if atomic.LoadInt64(&m.secondCount) >= m.maxConnsRate {
		return false
	}
	atomic.AddInt64(&m.secondCount, 1)
	return true
}

func IPKey(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	h, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return h
}

func (m *Manager) TryAcquire(userKey, ip string, ar authapi.Result) error {
	if m == nil {
		return nil
	}
	if ar.UserQPS > 0 {
		if !m.allowQPS(m.userQPS, userKey, ar.UserQPS) {
			return fmt.Errorf("user qps exceeded")
		}
	}
	if ar.IPQPS > 0 {
		if !m.allowQPS(m.ipQPS, ip, ar.IPQPS) {
			return fmt.Errorf("ip qps exceeded")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ar.UserConns > 0 {
		if m.userConns[userKey] >= ar.UserConns {
			return fmt.Errorf("user max connections")
		}
		m.userConns[userKey]++
	}
	if ar.IPConns > 0 {
		if m.ipConns[ip] >= ar.IPConns {
			if ar.UserConns > 0 {
				m.userConns[userKey]--
			}
			return fmt.Errorf("ip max connections")
		}
		m.ipConns[ip]++
	}
	return nil
}

func (m *Manager) allowQPS(table map[string]*qpsWindow, key string, limit int64) bool {
	sec := time.Now().Unix()
	m.mu.Lock()
	w := table[key]
	if w == nil || w.sec != sec {
		w = &qpsWindow{sec: sec, n: 0}
		table[key] = w
	}
	if w.n >= limit {
		m.mu.Unlock()
		return false
	}
	w.n++
	m.mu.Unlock()
	return true
}

func (m *Manager) Release(userKey, ip string, ar authapi.Result) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ar.UserConns > 0 {
		if m.userConns[userKey] > 0 {
			m.userConns[userKey]--
		}
	}
	if ar.IPConns > 0 {
		if m.ipConns[ip] > 0 {
			m.ipConns[ip]--
		}
	}
}
