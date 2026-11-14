package shared

import "strings"

type ApplicationConfig struct {
	Listener  string `json:"listener,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Directory string `json:"directory"`
	Command   string `json:"command"`
	Port      int    `json:"port"`
}

func ValidApplicationPort(port int) bool {
	if port < 1024 || port > 65535 {
		return false
	}
	switch port {
	case 3773, 8080, 8099, 18081, 18082, 18083, 18084, 18085:
		return false
	}
	return true
}
func (a ApplicationConfig) Valid() bool {
	if !ValidApplicationPort(a.Port) {
		return false
	}
	if a.Kind == "forward" {
		return a.Directory == "." && a.Command == "" && strings.HasPrefix(a.Listener, "listener_") && ValidID(a.Listener)
	}
	return a.Listener == "" && (a.Kind == "" || a.Kind == "web" || (a.Kind == "desktop" && a.Command == "workbench-desktop" && a.Directory == ".")) && (a.Directory == "." || ValidProjectFile(a.Directory)) && strings.TrimSpace(a.Command) != "" && len(a.Command) <= 4096 && !strings.ContainsRune(a.Command, 0)
}
