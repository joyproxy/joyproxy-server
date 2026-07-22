package sps

import (
	"fmt"
	"strconv"
	"strings"
)

type PortRange struct {
	Host  string
	Start int
	End   int
}

func ParsePortSpec(spec string) ([]PortRange, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty -p")
	}
	host := ""
	rest := spec
	if strings.Contains(spec, ":") {
		idx := strings.LastIndex(spec, ":")
		possibleHost := spec[:idx]
		rest = spec[idx+1:]
		if possibleHost != "" {
			host = possibleHost
		}
	}
	if strings.HasPrefix(rest, ":") {
		rest = rest[1:]
	}
	if strings.Contains(rest, "-") {
		parts := strings.SplitN(rest, "-", 2)
		a, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return nil, err
		}
		b, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, err
		}
		if a > b {
			a, b = b, a
		}
		return []PortRange{{Host: host, Start: a, End: b}}, nil
	}
	p, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil {
		return nil, err
	}
	return []PortRange{{Host: host, Start: p, End: p}}, nil
}
