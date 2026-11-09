package firecracker

import (
	"net"
	"sync"
)

type budgetListener struct {
	net.Listener
	local, global chan struct{}
}
type budgetConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *budgetConn) Close() error { err := c.Conn.Close(); c.once.Do(c.release); return err }
func (l *budgetListener) Accept() (net.Conn, error) {
	for {
		c, e := l.Listener.Accept()
		if e != nil {
			return nil, e
		}
		select {
		case l.local <- struct{}{}:
		default:
			c.Close()
			continue
		}
		select {
		case l.global <- struct{}{}:
			return &budgetConn{Conn: c, release: func() { <-l.local; <-l.global }}, nil
		default:
			<-l.local
			c.Close()
		}
	}
}
