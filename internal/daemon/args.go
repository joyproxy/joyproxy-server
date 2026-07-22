package daemon

import "strings"

// ArgsWithoutDaemon returns a copy of argv with --daemon / --daemon=* removed.
func ArgsWithoutDaemon(argv []string) []string {
	out := make([]string, 0, len(argv))
	for _, a := range argv {
		if a == "--daemon" || a == "--daemon=true" {
			continue
		}
		if strings.HasPrefix(a, "--daemon=") {
			continue
		}
		out = append(out, a)
	}
	return out
}
