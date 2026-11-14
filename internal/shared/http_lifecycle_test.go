package shared

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeListenerDrainsActiveHandlerBeforeReturning(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- ServeListener(ctx, listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(entered)
			<-release
			_, _ = io.WriteString(w, "complete")
		}), time.Second)
	}()
	reply := make(chan error, 1)
	go func() {
		res, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			_, err = io.ReadAll(res.Body)
			res.Body.Close()
		}
		reply <- err
	}()
	<-entered
	cancel()
	select {
	case err := <-finished:
		t.Fatalf("returned before draining handler: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-reply; err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestServeListenerFailureCancelsAndJoinsActiveHandler(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered, joined := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- ServeListener(context.Background(), listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(entered)
			<-r.Context().Done()
			close(joined)
		}), time.Second)
	}()
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		response, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			response.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("listener failure was hidden")
		}
		select {
		case <-joined:
		default:
			t.Fatal("returned before joining handler")
		}
	case <-time.After(time.Second):
		t.Fatal("listener failure did not cancel active handler")
	}
	<-clientDone
}

func TestServeListenerCancelsAndJoinsHandlerAfterDrainTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, handlerDone := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- ServeListener(ctx, listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(handlerDone) }), 20*time.Millisecond)
	}()
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		res, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			res.Body.Close()
		}
	}()
	<-entered
	cancel()
	select {
	case err := <-finished:
		if err != context.DeadlineExceeded {
			t.Fatalf("expected drain timeout, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed-out drain did not cancel handler")
	}
	select {
	case <-handlerDone:
	default:
		t.Fatal("server returned before handler stopped")
	}
	<-clientDone
}

// A refactor to the common lifecycle must not discard gateway/broker body
// deadlines: an authenticated slow client must not hold a handler indefinitely.
func TestServeListenerPreservesReadTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readErr := make(chan error, 1)
	server := newHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		readErr <- err
	}))
	server.ReadTimeout = 50 * time.Millisecond
	done := make(chan error, 1)
	go func() { done <- serveListener(ctx, listener, server, time.Second) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = io.WriteString(conn, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: 10\r\n\r\nx"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-readErr:
		timeout, ok := err.(net.Error)
		if !ok || !timeout.Timeout() {
			t.Fatalf("expected body timeout, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("configured read timeout was lost")
	}
}
