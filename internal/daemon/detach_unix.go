//go:build !windows

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// ShouldDetach is true on Unix-like systems where we can leave the controlling terminal.
func ShouldDetach() bool { return true }

// LaunchDetached starts a background session and exits this process with code 0.
// If forever, the child supervises workers (argv without --daemon, EnvInternalSupervisor).
// If !forever, the child uses EnvDaemonized and keeps full argv for single-worker spawn.
func LaunchDetached(forever bool) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable: %w", err)
	}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open /dev/null: %w", err)
	}
	defer null.Close()

	var cmd *exec.Cmd
	var extraEnv []string
	if forever {
		cmd = exec.Command(exe, ArgsWithoutDaemon(os.Args[1:])...)
		extraEnv = []string{EnvInternalSupervisor + "=1"}
	} else {
		cmd = exec.Command(exe, os.Args[1:]...)
		extraEnv = []string{EnvDaemonized + "=1"}
	}
	cmd.Env = append(append([]string{}, os.Environ()...), extraEnv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = null
	cmd.Stdout = null
	cmd.Stderr = null
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start detached process: %w", err)
	}
	os.Exit(0)
	panic("unreachable")
}
