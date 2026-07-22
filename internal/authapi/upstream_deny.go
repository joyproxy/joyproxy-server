package authapi

import (
	"net/url"
	"strings"
)

// UpstreamMeansAuthDeny reports UPSTREAM values that mean “no parent” (e.g. PWA ERR).
// Joyproxy then uses X-Joyproxy-Deny / X-Joyproxy-Reject-Status to choose 407 vs 503/429.
func UpstreamMeansAuthDeny(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.EqualFold(raw, "ERR") {
		return true
	}
	s := raw
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "err", "error", "fail", "failed", "deny", "blocked":
		return true
	default:
		return false
	}
}
