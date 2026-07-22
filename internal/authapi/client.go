package authapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	BaseURL      string
	CacheSuccess time.Duration
	CacheFail    time.Duration
	HTTP         *http.Client
	mu           sync.Mutex
	successCache map[string]cacheEntry
	failCache    map[string]cacheEntry
}

type cacheEntry struct {
	res   Result
	until time.Time
}

func New(baseURL string, cacheSuccessSec, cacheFailSec int) *Client {
	c := &Client{
		BaseURL:      strings.TrimSpace(baseURL),
		HTTP:         &http.Client{Timeout: 15 * time.Second},
		successCache: make(map[string]cacheEntry),
		failCache:    make(map[string]cacheEntry),
	}
	if cacheSuccessSec > 0 {
		c.CacheSuccess = time.Duration(cacheSuccessSec) * time.Second
	}
	if cacheFailSec > 0 {
		c.CacheFail = time.Duration(cacheFailSec) * time.Second
	}
	return c
}

func cacheKey(user, pass, clientAddr, localAddr, target, service, sps string) string {
	return user + "\x00" + pass + "\x00" + clientAddr + "\x00" + localAddr + "\x00" + target + "\x00" + service + "\x00" + sps
}

func (c *Client) Authorize(ctx context.Context, user, pass, clientAddr, localAddr, target, service, sps string) (ok bool, res Result, err error) {
	if c.BaseURL == "" {
		return true, Result{}, nil
	}
	key := cacheKey(user, pass, clientAddr, localAddr, target, service, sps)
	c.mu.Lock()
	if c.CacheSuccess > 0 {
		if e, hit := c.successCache[key]; hit && time.Now().Before(e.until) {
			c.mu.Unlock()
			return true, e.res, nil
		}
	}
	if c.CacheFail > 0 {
		if e, hit := c.failCache[key]; hit && time.Now().Before(e.until) {
			c.mu.Unlock()
			return false, e.res, nil
		}
	}
	c.mu.Unlock()
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return false, Result{}, err
	}
	q := u.Query()
	q.Set("user", user)
	q.Set("pass", pass)
	q.Set("client_addr", clientAddr)
	q.Set("local_addr", localAddr)
	q.Set("target", target)
	q.Set("service", service)
	q.Set("sps", sps)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, Result{}, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false, Result{}, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		den := Result{RejectHTTPStatus: http.StatusServiceUnavailable}
		c.mu.Lock()
		if c.CacheFail > 0 {
			c.failCache[key] = cacheEntry{res: den, until: time.Now().Add(c.CacheFail)}
		}
		c.mu.Unlock()
		return false, den, nil
	}
	res = parseAuthHeaders(resp.Header)
	if UpstreamMeansAuthDeny(res.Upstream) {
		// PWA 等对 ERR 的响应头往往与拒绝原因无关；未带 X-Joyproxy-* 时一律 503，避免误发 407。
		if res.RejectHTTPStatus != http.StatusProxyAuthRequired && res.RejectHTTPStatus != http.StatusServiceUnavailable && res.RejectHTTPStatus != http.StatusTooManyRequests {
			res.RejectHTTPStatus = http.StatusServiceUnavailable
		}
		c.mu.Lock()
		if c.CacheFail > 0 {
			c.failCache[key] = cacheEntry{res: res, until: time.Now().Add(c.CacheFail)}
		}
		c.mu.Unlock()
		return false, res, nil
	}
	res.RejectHTTPStatus = 0
	c.mu.Lock()
	if c.CacheSuccess > 0 {
		c.successCache[key] = cacheEntry{res: res, until: time.Now().Add(c.CacheSuccess)}
	}
	c.mu.Unlock()
	return true, res, nil
}

func parseAuthHeaders(h http.Header) Result {
	var r Result
	r.UserConns = parseIntHeader(h, "userconns")
	r.IPConns = parseIntHeader(h, "ipconns")
	r.UserRate = parseIntHeader(h, "userrate")
	r.IPRate = parseIntHeader(h, "iprate")
	r.UserQPS = parseIntHeader(h, "userqps")
	r.IPQPS = parseIntHeader(h, "ipqps")
	r.Upstream = strings.TrimSpace(h.Get("upstream"))
	if r.Upstream == "" {
		r.Upstream = firstHeaderCI(h, "upstream")
	}
	r.Outgoing = strings.TrimSpace(h.Get("outgoing"))
	r.UserTotalRate = parseIntHeader(h, "userTotalRate")
	r.IPTotalRate = parseIntHeader(h, "ipTotalRate")
	r.PortTotalRate = parseIntHeader(h, "portTotalRate")
	r.RotationTimeSec = parseIntHeader(h, "RotationTime")
	r.RejectHTTPStatus = parseJoyproxyRejectFromHeaders(h)
	return r
}

// parseJoyproxyRejectFromHeaders: X-Joyproxy-Reject-Status (407|503|429) or X-Joyproxy-Deny (auth|limit|...).
func parseJoyproxyRejectFromHeaders(h http.Header) int {
	if v := parseIntHeader(h, "X-Joyproxy-Reject-Status"); v == 407 || v == 503 || v == 429 {
		return int(v)
	}
	if v := parseIntHeader(h, "x-joyproxy-reject-status"); v == 407 || v == 503 || v == 429 {
		return int(v)
	}
	s := strings.ToLower(strings.TrimSpace(h.Get("X-Joyproxy-Deny")))
	if s == "" {
		s = strings.ToLower(strings.TrimSpace(firstHeaderCI(h, "X-Joyproxy-Deny")))
	}
	switch s {
	case "auth", "unauthorized", "credential", "whitelist", "forbidden":
		return http.StatusProxyAuthRequired // 407
	case "limit", "concurrent", "rate", "overload", "cache", "ratio", "busy", "throttle":
		return http.StatusServiceUnavailable // 503
	default:
		return 0
	}
}

func firstHeaderCI(h http.Header, name string) string {
	for k, vv := range h {
		if strings.EqualFold(k, name) && len(vv) > 0 {
			return strings.TrimSpace(vv[0])
		}
	}
	return ""
}

func parseIntHeader(h http.Header, name string) int64 {
	v := strings.TrimSpace(h.Get(name))
	if v == "" {
		v = firstHeaderCI(h, name)
	}
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func EffectiveUpstream(api Result, cliDefault string) string {
	if strings.TrimSpace(api.Upstream) != "" {
		return api.Upstream
	}
	return strings.TrimSpace(cliDefault)
}

func RequireCreds(authURL string, authNoUser bool, user, pass string) error {
	if authURL == "" {
		return nil
	}
	if authNoUser {
		return nil
	}
	if user == "" || pass == "" {
		return fmt.Errorf("authentication required")
	}
	return nil
}

func UserKey(authNoUser bool, user string) string {
	if authNoUser && user == "" {
		return ""
	}
	return user
}
