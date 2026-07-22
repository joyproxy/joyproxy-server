package authapi

import "net/http"

// ClientHTTPStatus is the HTTP status to return to the proxy client when Authorize returned !ok.
func ClientHTTPStatus(ar Result) int {
	switch ar.RejectHTTPStatus {
	case http.StatusProxyAuthRequired, http.StatusServiceUnavailable, http.StatusTooManyRequests:
		return ar.RejectHTTPStatus
	default:
		return http.StatusServiceUnavailable
	}
}

// Socks5RejectReply maps auth rejection to SOCKS5 response code (auth-ish → 2, else → 1).
func Socks5RejectReply(ar Result) byte {
	switch ClientHTTPStatus(ar) {
	case http.StatusProxyAuthRequired:
		return 2
	default:
		return 1
	}
}
