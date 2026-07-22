package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"joyproxy/internal/config"
	"joyproxy/internal/daemon"
	"joyproxy/internal/logx"
	"joyproxy/internal/sps"
)

func Execute() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: joyproxy sps [flags]")
	}
	switch os.Args[1] {
	case "sps":
		return runSPS(os.Args[2:])
	default:
		return fmt.Errorf("unknown command %q (try: joyproxy sps -h)", os.Args[1])
	}
}

func runSPS(args []string) error {
	fs := flag.NewFlagSet("joyproxy sps", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		cfg       config.SPS
		worker    bool
		restartS  int
		parent    string
		verbose   bool
		quiet     bool
		noDetach  bool
	)
	fs.StringVar(&cfg.ServerType, "S", "http", "upstream type: http|socks5")
	fs.StringVar(&cfg.Transport, "T", "tcp", "transport (reserved)")
	fs.StringVar(&cfg.PortSpec, "p", ":8080", "listen ports e.g. :5001-5999")
	fs.StringVar(&cfg.GatewayIP, "g", "", "public IP for auth local_addr")
	fs.BoolVar(&cfg.Forever, "forever", false, "with --daemon: supervise worker")
	fs.BoolVar(&cfg.Daemon, "daemon", false, "detach: shell returns at once; supervise worker in background (see --forever); logs off unless --verbose")
	fs.BoolVar(&noDetach, "no-detach", false, "with --daemon on Unix: stay attached (for systemd Type=simple); omit terminal detach")
	fs.BoolVar(&worker, "joyproxy-worker", false, "")
	fs.BoolVar(&cfg.AuthNoUser, "auth-nouser", false, "allow empty user/pass")
	fs.StringVar(&cfg.AuthURL, "auth-url", "", "auth API URL")
	fs.IntVar(&cfg.AuthCache, "auth-cache", 0, "auth success cache TTL seconds")
	fs.IntVar(&cfg.AuthFailCache, "auth-fail-cache", 0, "auth fail cache TTL seconds")
	fs.IntVar(&cfg.MaxConnsRate, "max-conns-rate", 0, "global new connections per second")
	fs.StringVar(&cfg.TrafficURL, "traffic-url", "", "traffic report URL")
	fs.StringVar(&cfg.TrafficMode, "traffic-mode", "normal", "traffic mode")
	fs.BoolVar(&cfg.SniffDomain, "sniff-domain", false, "TLS SNI sniff")
	fs.StringVar(&parent, "parent", "", "default upstream if API omits")
	fs.IntVar(&restartS, "restart-delay", 5, "seconds before restarting crashed worker")
	fs.BoolVar(&verbose, "verbose", false, "full logs: listening banner, per-connection trace; with --daemon also supervisor + worker logs")
	fs.BoolVar(&quiet, "quiet", false, "only [err] lines (no [warn], no listening banner)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	cfg.Worker = worker
	cfg.DefaultParent = parent
	if restartS < 1 {
		restartS = 5
	}
	cfg.Restart = time.Duration(restartS) * time.Second

	if verbose && quiet {
		return fmt.Errorf("joyproxy: --verbose and --quiet cannot be used together")
	}
	if cfg.Worker {
		// Child of --daemon: no logx output at all.
		cfg.LogLevel = logx.LevelSilent
	} else {
		switch {
		case quiet:
			cfg.LogLevel = logx.LevelQuiet
		case verbose:
			cfg.LogLevel = logx.LevelVerbose
		default:
			// Foreground: only errors / anomalies ([warn],[err]); no routine success noise.
			cfg.LogLevel = logx.LevelErrorsOnly
		}
	}

	internalSup := os.Getenv(daemon.EnvInternalSupervisor) != ""
	if internalSup {
		_ = os.Unsetenv(daemon.EnvInternalSupervisor)
		if !cfg.Forever || cfg.Worker {
			return fmt.Errorf("joyproxy: invalid internal supervisor state")
		}
		wa := workerArgs()
		daemon.RunSupervised(wa, true, cfg.Restart, !verbose)
		return nil
	}

	daemonized := os.Getenv(daemon.EnvDaemonized) != ""
	if daemonized {
		_ = os.Unsetenv(daemon.EnvDaemonized)
	}

	if cfg.Daemon && !cfg.Worker {
		if !daemonized && daemon.ShouldDetach() && !noDetach {
			if err := daemon.LaunchDetached(cfg.Forever); err != nil {
				return fmt.Errorf("joyproxy: detach to background: %w", err)
			}
		}
		wa := workerArgs()
		if cfg.Forever {
			daemon.RunSupervised(wa, true, cfg.Restart, !verbose)
			return nil
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		c := exec.Command(exe, wa...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Env = os.Environ()
		return c.Start()
	}

	srv := sps.NewServer(&cfg)
	return srv.Run()
}

func workerArgs() []string {
	out := make([]string, 0, len(os.Args))
	out = append(out, "sps")
	hasW := false
	for _, a := range os.Args[2:] {
		if a == "--daemon" || a == "--daemon=true" || strings.HasPrefix(a, "--daemon=") {
			continue
		}
		if a == "--forever" || a == "--forever=true" || strings.HasPrefix(a, "--forever=") {
			continue
		}
		if a == "--joyproxy-worker" || a == "--joyproxy-worker=true" {
			hasW = true
		}
		out = append(out, a)
	}
	if !hasW {
		out = append(out, "--joyproxy-worker")
	}
	return out
}
