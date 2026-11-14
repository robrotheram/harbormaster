package workspace

import (
	"agent-workbench/internal/shared"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type detectedPort struct {
	Port     int    `json:"port"`
	Listener string `json:"listener"`
	Address  string `json:"-"`
	Inode    string `json:"-"`
}

// Only loopback-reachable listeners in this workspace are eligible. Management
// services are excluded before probing; callers can never choose a target host.
func parseListeningPorts(text string, ipv6 bool) []detectedPort {
	ports := []detectedPort{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 || fields[3] != "0A" {
			continue
		}
		address, portHex, ok := strings.Cut(fields[1], ":")
		if !ok {
			continue
		}
		port, err := strconv.ParseUint(portHex, 16, 16)
		if err != nil || !shared.ValidApplicationPort(int(port)) {
			continue
		}
		host := "127.0.0.1"
		if ipv6 {
			if address != "00000000000000000000000000000000" && address != "00000000000000000000000001000000" {
				continue
			}
			host = "::1"
		} else if address != "00000000" && address != "0100007F" {
			continue
		}
		if fields[9] == "0" {
			continue
		}
		sum := sha256.Sum256([]byte(fields[1] + ":" + fields[9]))
		ports = append(ports, detectedPort{int(port), "listener_" + hex.EncodeToString(sum[:16]), net.JoinHostPort(host, strconv.Itoa(int(port))), fields[9]})
	}
	return ports
}
func listeningPorts() ([]detectedPort, error) {
	ports := []detectedPort{}
	for _, protocol := range []string{"tcp", "tcp6"} {
		data, err := os.ReadFile("/proc/net/" + protocol)
		if err != nil {
			if protocol == "tcp6" && os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		ports = append(ports, parseListeningPorts(string(data), protocol == "tcp6")...)
	}
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Port == ports[j].Port {
			return ports[i].Address < ports[j].Address
		}
		return ports[i].Port < ports[j].Port
	})
	return ports, nil
}
func probeHTTP(ctx context.Context, port detectedPort) bool {
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, MaxResponseHeaderBytes: 16384}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 350 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, "HEAD", "http://"+port.Address+"/", nil)
	if err != nil {
		return false
	}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	return true
}
func (m *Manager) listPorts(w http.ResponseWriter, r *http.Request) {
	ports, err := listeningPorts()
	if err != nil {
		shared.Error(w, 503, "PORT_DISCOVERY_UNAVAILABLE")
		return
	}
	// Bound probes so a busy guest cannot hold the operator request indefinitely.
	if len(ports) > 32 {
		ports = ports[:32]
	}
	type result struct {
		port detectedPort
		ok   bool
	}
	results := make(chan result, len(ports))
	for _, port := range ports {
		go func(p detectedPort) { results <- result{p, probeHTTP(r.Context(), p)} }(port)
	}
	items := []detectedPort{}
	seen := map[int]bool{}
	for range ports {
		result := <-results
		if result.ok && !seen[result.port.Port] {
			items = append(items, result.port)
			seen[result.port.Port] = true
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Port < items[j].Port })
	shared.JSON(w, 200, map[string]any{"items": items})
}
func forwardTarget(config shared.ApplicationConfig) (detectedPort, bool) {
	ports, err := listeningPorts()
	if err != nil {
		return detectedPort{}, false
	}
	for _, p := range ports {
		if p.Port == config.Port && p.Listener == config.Listener {
			return p, true
		}
	}
	return detectedPort{}, false
}
func listenerOwner(inode string) (int, string) {
	processes, _ := os.ReadDir("/proc")
	for _, process := range processes {
		pid, err := strconv.Atoi(process.Name())
		if err != nil {
			continue
		}
		fds, _ := os.ReadDir("/proc/" + process.Name() + "/fd")
		for _, fd := range fds {
			link, err := os.Readlink("/proc/" + process.Name() + "/fd/" + fd.Name())
			if err == nil && link == "socket:["+inode+"]" {
				return pid, processStart(pid)
			}
		}
	}
	return 0, ""
}
func processStart(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	end := strings.LastIndex(string(data), ") ")
	if end < 0 {
		return ""
	}
	fields := strings.Fields(string(data)[end+2:])
	if len(fields) < 20 {
		return ""
	}
	return fields[19]
}
