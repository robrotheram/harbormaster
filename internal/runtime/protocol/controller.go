package protocol

import (
	"context"
	"io"
	"net/http"
	"strings"

	"agent-workbench/internal/shared"
)

// ControllerError identifies a backend failure on the controller HTTP surface.
type ControllerError struct {
	Status int
	Code   string
}

func Failure(status int, code string) *ControllerError {
	return &ControllerError{Status: status, Code: code}
}

// Backend supplies controller operations after HTTP dispatch and decoding.
// Each implementation remains responsible for runtime ownership and readiness.
type Backend struct {
	Token, GatewayServiceToken string
	Health, VerifyGrant        http.HandlerFunc
	RegisterManagement         func(*http.ServeMux)

	Start             func(context.Context, string, WorkspaceSpec) (shared.Observation, *ControllerError)
	Stop              func(context.Context, string) *ControllerError
	Inspect           func(context.Context, string) (shared.Observation, *ControllerError)
	BindNetworkPolicy func(context.Context, string) func(*shared.ConnectionPolicy) *ControllerError
	Reconcile         func(context.Context, string) *ControllerError
	Capabilities      func() RuntimeCapability
	Proxy             func(http.ResponseWriter, *http.Request, string, string, string)
	Agent             func(context.Context, string, string, string, io.Reader) (int, []byte, *ControllerError)

	StartBodyLimit  int64
	AllowEmptyStart bool
	ValidateAgentID bool
	AgentNoStore    bool
	// WorkspaceActionFallback retains Firecracker's catch-all dispatch, ID
	// validation and JSON 404s, including decoded slashes in the action.
	WorkspaceActionFallback bool
}

func NewControllerHandler(b Backend) http.Handler {
	root, management := http.NewServeMux(), http.NewServeMux()
	root.HandleFunc("GET /healthz", b.Health)
	root.Handle("POST /v1/grants/verify", shared.RequireToken(b.GatewayServiceToken, b.VerifyGrant))
	scope := "/v1/"
	if b.WorkspaceActionFallback {
		scope = "/"
	}
	root.Handle(scope, shared.RequireToken(b.Token, management))
	if b.RegisterManagement != nil {
		b.RegisterManagement(management)
	}
	management.HandleFunc("GET /v1/capabilities", func(w http.ResponseWriter, r *http.Request) {
		shared.JSON(w, 200, b.Capabilities())
	})
	const workspace = "/v1/workspaces/{id}"
	management.HandleFunc("GET "+workspace, func(w http.ResponseWriter, r *http.Request) {
		o, failure := b.Inspect(r.Context(), r.PathValue("id"))
		controllerResult(w, o, failure)
	})

	actions := map[string]http.HandlerFunc{
		"start": func(w http.ResponseWriter, r *http.Request) {
			var spec WorkspaceSpec
			if (!b.AllowEmptyStart || r.ContentLength != 0) && !shared.Decode(w, r, &spec, b.StartBodyLimit) {
				return
			}
			o, failure := b.Start(r.Context(), r.PathValue("id"), spec)
			controllerResult(w, o, failure)
		},
		"stop": func(w http.ResponseWriter, r *http.Request) {
			controllerResult(w, map[string]bool{"stopped": true}, b.Stop(r.Context(), r.PathValue("id")))
		},
		"network-policy": func(w http.ResponseWriter, r *http.Request) {
			apply := b.BindNetworkPolicy(r.Context(), r.PathValue("id"))
			var input struct {
				Connections *shared.ConnectionPolicy `json:"connections"`
			}
			if shared.Decode(w, r, &input, 32768) {
				controllerResult(w, map[string]bool{"applied": true}, apply(input.Connections))
			}
		},
		"reconcile-network": func(w http.ResponseWriter, r *http.Request) {
			controllerResult(w, map[string]bool{"reconciled": true}, b.Reconcile(r.Context(), r.PathValue("id")))
		},
	}
	streams := map[string]http.HandlerFunc{
		"agent": b.serveAgent,
	}
	for route, prefix := range map[string]string{
		"console": "/v1/t3/", "desktop": "/v1/t3-desktop/", "application-proxy": "/v1/application-proxy/",
	} {
		streams[route] = func(w http.ResponseWriter, r *http.Request) {
			b.Proxy(w, r, r.PathValue("id"), r.PathValue("path"), prefix)
		}
	}
	if b.WorkspaceActionFallback {
		management.HandleFunc(workspace+"/{action...}", func(w http.ResponseWriter, r *http.Request) {
			if !shared.ValidID(r.PathValue("id")) {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			action := r.PathValue("action")
			if handler := actions[action]; r.Method == http.MethodPost && handler != nil {
				handler(w, r)
				return
			}
			if name, path, ok := strings.Cut(action, "/"); ok && streams[name] != nil {
				r.SetPathValue("path", path)
				streams[name](w, r)
				return
			}
			shared.Error(w, 404, "NOT_FOUND")
		})
	} else {
		for action, handler := range actions {
			management.HandleFunc("POST "+workspace+"/"+action, handler)
		}
		for route, handler := range streams {
			management.HandleFunc(workspace+"/"+route+"/{path...}", handler)
		}
	}
	return root
}

func controllerResult(w http.ResponseWriter, value any, failure *ControllerError) {
	if failure != nil {
		shared.Error(w, failure.Status, failure.Code)
		return
	}
	shared.JSON(w, 200, value)
}

func (b Backend) serveAgent(w http.ResponseWriter, r *http.Request) {
	id, path := r.PathValue("id"), "/"+r.PathValue("path")
	if (b.ValidateAgentID && !shared.ValidID(id)) || !AllowedAgentPath(r.Method, path) {
		shared.Error(w, 404, "NOT_FOUND_OR_FORBIDDEN")
		return
	}
	query, err := AgentQuery(r.Method, path, r.URL.RawQuery)
	if err != nil {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	body := http.MaxBytesReader(w, r.Body, AgentBodyLimit(path))
	if query != "" {
		path += "?" + query
	}
	status, out, failure := b.Agent(r.Context(), id, r.Method, path, body)
	if failure != nil {
		shared.Error(w, failure.Status, failure.Code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if b.AgentNoStore {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	_, _ = w.Write(out)
}

// BufferedAgent validates the full request body before calling a backend that
// sends buffered requests to the guest. Firecracker consumes the capped reader
// directly after checking readiness.
func BufferedAgent(agent func(context.Context, string, string, string, []byte) (int, []byte, *ControllerError)) func(context.Context, string, string, string, io.Reader) (int, []byte, *ControllerError) {
	return func(ctx context.Context, id, method, path string, body io.Reader) (int, []byte, *ControllerError) {
		input, err := io.ReadAll(body)
		if err != nil {
			return 0, nil, Failure(400, "INVALID_REQUEST")
		}
		return agent(ctx, id, method, path, input)
	}
}

// BindNetworkPolicy adapts backends that resolve their target after decoding.
func BindNetworkPolicy(apply func(context.Context, string, *shared.ConnectionPolicy) *ControllerError) func(context.Context, string) func(*shared.ConnectionPolicy) *ControllerError {
	return func(ctx context.Context, id string) func(*shared.ConnectionPolicy) *ControllerError {
		return func(policy *shared.ConnectionPolicy) *ControllerError {
			return apply(ctx, id, policy)
		}
	}
}
