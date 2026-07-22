package logx

import (
	"errors"
	"io"
	"log"
	"net"
	"os"
	"strings"
)

// Log levels (ordered low → high).
const (
	LevelSilent = iota // no output (daemon worker)
	LevelErrorsOnly    // [warn] + [err] only; no startup / per-conn noise
	LevelQuiet         // [err] only; no [warn]
	LevelNormal        // + Info + Startup; no Debug
	LevelVerbose       // + Debug / per-connection
)

var level int

func Init(l int) {
	if l < LevelSilent {
		l = LevelSilent
	}
	if l > LevelVerbose {
		l = LevelVerbose
	}
	level = l
	if l <= LevelSilent {
		log.SetOutput(io.Discard)
		return
	}
	log.SetOutput(os.Stderr)
	if l >= LevelVerbose {
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	} else {
		log.SetFlags(log.LstdFlags)
	}
	log.SetPrefix("joyproxy ")
}

func Level() int { return level }

// Startup logs listener lines (Normal+ only; not "normal" traffic for ErrorsOnly default).
func Startup(format string, args ...interface{}) {
	if level <= LevelSilent {
		return
	}
	if level < LevelNormal {
		return
	}
	log.Printf(format, args...)
}

// Info: Normal + Verbose.
func Info(format string, args ...interface{}) {
	if level >= LevelNormal {
		log.Printf(format, args...)
	}
}

// Debug: Verbose only.
func Debug(format string, args ...interface{}) {
	if level >= LevelVerbose {
		log.Printf(format, args...)
	}
}

// Warn: ErrorsOnly, Normal, Verbose (not Quiet or Silent).
func Warn(format string, args ...interface{}) {
	if level <= LevelSilent || level == LevelQuiet {
		return
	}
	if level < LevelErrorsOnly {
		return
	}
	log.Printf("[warn] "+format, args...)
}

// Error: any level except Silent.
func Error(format string, args ...interface{}) {
	if level <= LevelSilent {
		return
	}
	log.Printf("[err] "+format, args...)
}

func isEOF(err error) bool {
	return err != nil && errors.Is(err, io.EOF)
}

func isCloseErr(err error) bool {
	if err == nil || isEOF(err) {
		return true
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "use of closed network connection") ||
		strings.Contains(s, "connection reset by peer") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "forcibly closed")
}

func isTimeout(err error) bool {
	var ne net.Error
	return err != nil && errors.As(err, &ne) && ne.Timeout()
}

// RelayLeg logs one direction of a relay when the copy loop ends.
func RelayLeg(direction string, nbytes int64, err error) {
	if err == nil || isEOF(err) {
		if level >= LevelVerbose {
			log.Printf("[relay] %s done bytes=%d (clean)", direction, nbytes)
		}
		return
	}
	if isCloseErr(err) {
		if level >= LevelVerbose {
			log.Printf("[relay] %s done bytes=%d: %v", direction, nbytes, err)
		}
		return
	}
	if isTimeout(err) {
		Warn("relay %s bytes=%d: timeout %v", direction, nbytes, err)
		return
	}
	Warn("relay %s bytes=%d: %v", direction, nbytes, err)
}
