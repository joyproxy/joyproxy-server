package limit

import (
	"io"
	"sync"
	"time"
)

type Throttle struct {
	bytesPerSec int64
	mu          sync.Mutex
	allowance   float64
	last        time.Time
}

func NewThrottle(bytesPerSec int64) *Throttle {
	if bytesPerSec <= 0 {
		return nil
	}
	return &Throttle{bytesPerSec: bytesPerSec, last: time.Now(), allowance: float64(bytesPerSec)}
}

func (t *Throttle) delay(n int) {
	if t == nil || n <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(t.last).Seconds()
	t.last = now
	t.allowance += elapsed * float64(t.bytesPerSec)
	maxBurst := float64(t.bytesPerSec)
	if t.allowance > maxBurst {
		t.allowance = maxBurst
	}
	t.allowance -= float64(n)
	if t.allowance >= 0 {
		return
	}
	need := -t.allowance / float64(t.bytesPerSec)
	time.Sleep(time.Duration(need * float64(time.Second)))
	t.allowance = 0
}

type Reader struct {
	R io.Reader
	T *Throttle
}

func (r *Reader) Read(p []byte) (int, error) {
	n, err := r.R.Read(p)
	if n > 0 && r.T != nil {
		r.T.delay(n)
	}
	return n, err
}

type Writer struct {
	W io.Writer
	T *Throttle
}

func (w *Writer) Write(p []byte) (int, error) {
	if w.T != nil {
		w.T.delay(len(p))
	}
	return w.W.Write(p)
}
