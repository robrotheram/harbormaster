package shared

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func Error(w http.ResponseWriter, status int, code string) {
	JSON(w, status, NewAPIError(status, code))
}
func NewAPIError(status int, code string) APIError {
	return APIError{Code: code, Message: strings.ReplaceAll(strings.ToLower(code), "_", " "), Retryable: status == 503}
}
func Decode(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		Error(w, 400, "INVALID_REQUEST")
		return false
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		Error(w, 400, "INVALID_REQUEST")
		return false
	}
	return true
}
func Env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func Secret(key string) (string, error) {
	if p := os.Getenv(key + "_FILE"); p != "" {
		b, e := os.ReadFile(p)
		return strings.TrimSpace(string(b)), e
	}
	return strings.TrimSpace(os.Getenv(key)), nil
}
func RequireToken(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if token == "" || subtle.ConstantTimeCompare([]byte(got), []byte("Bearer "+token)) != 1 {
			Error(w, 401, "UNAUTHORIZED")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func newHTTPServer(h http.Handler) *http.Server {
	return &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
}

// Serve drains standalone services on SIGINT or SIGTERM.
func Serve(addr string, h http.Handler) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return ServeContext(ctx, addr, h)
}

// ServeHTTPServer preserves service-specific HTTP timeouts while sharing the
// listener, request cancellation and handler-draining lifecycle.
// The caller must not reuse the server after this call.
func ServeHTTPServer(ctx context.Context, server *http.Server) error {
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	slog.Info("listening", "address", server.Addr)
	return serveListener(ctx, listener, server, 10*time.Second)
}

// ServeContext owns its listener and drains handlers before returning on cancellation.
func ServeContext(ctx context.Context, addr string, h http.Handler) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	slog.Info("listening", "address", addr)
	return ServeListener(ctx, listener, h, 10*time.Second)
}

// ServeListener is the same lifecycle for an already-bound listener. On drain
// timeout, Close cancels active requests before their dependencies are released.
func ServeListener(ctx context.Context, listener net.Listener, h http.Handler, grace time.Duration) error {
	return serveListener(ctx, listener, newHTTPServer(h), grace)
}

func serveListener(ctx context.Context, listener net.Listener, server *http.Server, grace time.Duration) error {
	h := server.Handler
	if h == nil {
		h = http.DefaultServeMux
	}
	var handlers sync.WaitGroup
	var handlerMu sync.Mutex
	closing := false
	requestContext, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerMu.Lock()
		if closing {
			handlerMu.Unlock()
			Error(w, http.StatusServiceUnavailable, "SHUTTING_DOWN")
			return
		}
		handlers.Add(1)
		handlerMu.Unlock()
		defer handlers.Done()
		if server.ReadTimeout == 0 && r.Body != nil && r.Body != http.NoBody {
			timeout := 30 * time.Second
			if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/analysis/datasets/upload") {
				timeout = AnalysisUploadTimeout
			}
			controller := http.NewResponseController(w)
			_ = controller.SetReadDeadline(time.Now().Add(timeout))
			defer controller.SetReadDeadline(time.Time{})
		}
		h.ServeHTTP(w, r)
	})
	joinHandlers := func() {
		// Prevent a connection already dispatched by net/http from adding a
		// handler after Wait observes zero. Late arrivals cannot use dependencies.
		handlerMu.Lock()
		closing = true
		handlerMu.Unlock()
		cancelRequests()
		handlers.Wait()
	}
	server.BaseContext = func(net.Listener) context.Context { return requestContext }
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		// An unexpected listener failure must cancel and join existing requests
		// just like an explicit shutdown, before their dependencies are closed.
		cancelRequests()
		_ = server.Close()
		joinHandlers()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		drain, cancel := context.WithTimeout(context.Background(), grace)
		defer cancel()
		err := server.Shutdown(drain)
		if err != nil {
			cancelRequests()
			_ = server.Close()
		}
		serveErr := <-done
		joinHandlers()
		if err != nil {
			return err
		}
		if errors.Is(serveErr, http.ErrServerClosed) {
			return nil
		}
		return serveErr
	}
}

func IntEnv(key string, fallback int64) int64 {
	value, err := strconv.ParseInt(os.Getenv(key), 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
