package traffic

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Reporter struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Reporter {
	return &Reporter{
		BaseURL: strings.TrimSpace(baseURL),
		HTTP:    &http.Client{Timeout: 8 * time.Second},
	}
}

func (r *Reporter) Report(q url.Values) {
	if r == nil || r.BaseURL == "" {
		return
	}
	go func() {
		u, err := url.Parse(r.BaseURL)
		if err != nil {
			log.Printf("traffic: bad traffic-url: %v", err)
			return
		}
		qs := u.Query()
		for k, vv := range q {
			for _, v := range vv {
				qs.Add(k, v)
			}
		}
		u.RawQuery = qs.Encode()
		resp, err := r.HTTP.Get(u.String())
		if err != nil {
			log.Printf("traffic: request error: %v", err)
			return
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != http.StatusNoContent {
			log.Printf("traffic: expected 204, got %d for %s", resp.StatusCode, u.Redacted())
		}
	}()
}
