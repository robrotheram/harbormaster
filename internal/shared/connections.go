package shared

import (
	"net"
	"slices"
	"strconv"
	"strings"
)

// ConnectionPolicy is persisted workspace configuration, never supplied by a guest.
// Allowlist entries are exact DNS names/IP literals and TCP ports, without wildcards.
type ConnectionPolicy struct {
	Mode         string   `json:"mode"`
	Destinations []string `json:"destinations"`
}

func (p ConnectionPolicy) Valid() bool {
	if p.Mode != "blocked" && p.Mode != "allowlist" && p.Mode != "open" {
		return false
	}
	if len(p.Destinations) > 64 || (p.Mode != "allowlist" && len(p.Destinations) != 0) {
		return false
	}
	for _, entry := range p.Destinations {
		if !ValidDestination(entry) {
			return false
		}
	}
	return true
}
func ValidDestination(entry string) bool {
	host, port, err := net.SplitHostPort(entry)
	if err != nil || host == "" || host != strings.ToLower(host) || len(host) > 253 {
		return false
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
func (p ConnectionPolicy) Allows(destination string) bool {
	if !ValidDestination(destination) {
		return false
	}
	if p.Mode == "open" {
		return true
	}
	if p.Mode != "allowlist" {
		return false
	}
	return slices.Contains(p.Destinations, destination)
}
func (w Workspace) EffectiveConnections() ConnectionPolicy {
	if w.Connections != nil {
		return *w.Connections
	}
	return ConnectionPolicy{Mode: "blocked", Destinations: []string{}}
}
