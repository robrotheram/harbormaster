package main

import (
	"context"
	"flag"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	unixSocket := flag.String("unix-socket", "", "Probe health over a Unix socket")
	flag.Parse()
	url := "http://127.0.0.1:8080/healthz"
	if flag.NArg() > 0 {
		url = flag.Arg(0)
	}
	if !healthy(url, *unixSocket) {
		os.Exit(1)
	}
}

func healthy(url, unixSocket string) bool {
	c := http.Client{Timeout: 2 * time.Second}
	if unixSocket != "" {
		transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", unixSocket)
		}}
		defer transport.CloseIdleConnections()
		c.Transport = transport
	}
	r, e := c.Get(url)
	if e != nil {
		return false
	}
	r.Body.Close()
	return r.StatusCode == 200
}
