package daemon

import (
	"io"
	"log"
	"os"
	"os/exec"
	"time"
)

// EnvDaemonized is set on the re-exec'd process for --daemon without --forever.
const EnvDaemonized = "JOYPROXY_DAEMONIZED"

// EnvInternalSupervisor marks the detached long-running supervisor (--daemon --forever).
const EnvInternalSupervisor = "JOYPROXY_INTERNAL_SUPERVISOR"

// RunSupervised respawns the worker. If quietSupervisor is true, no supervisor log lines
// (including worker start/exit errors); use --verbose on the parent to log to stderr.
func RunSupervised(workerArgs []string, forever bool, restart time.Duration, quietSupervisor bool) {
	if restart <= 0 {
		restart = 5 * time.Second
	}
	out := io.Writer(os.Stderr)
	if quietSupervisor {
		out = io.Discard
	}
	sup := log.New(out, "", log.LstdFlags)

	exe, err := os.Executable()
	if err != nil {
		log.New(os.Stderr, "", log.LstdFlags).Fatal(err)
	}
	for {
		cmd := exec.Command(exe, workerArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()
		if !quietSupervisor {
			sup.Printf("joyproxy: starting worker pid ...")
		}
		err := cmd.Start()
		if err != nil {
			sup.Printf("joyproxy: start worker: %v", err)
			if !forever {
				os.Exit(1)
			}
			time.Sleep(restart)
			continue
		}
		err = cmd.Wait()
		if err == nil {
			if !quietSupervisor {
				sup.Printf("joyproxy: worker exited normally")
			}
			if !forever {
				return
			}
		} else {
			sup.Printf("joyproxy: worker exited with error: %v", err)
			if !forever {
				os.Exit(1)
			}
		}
		time.Sleep(restart)
	}
}
