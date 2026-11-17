package dashboard

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// These small pages also work when the main frontend bundle is unavailable.
// They never receive a control credential or include a connection ticket.
//
//go:embed connection-page.html
var connectionHTML string

//go:embed connection-page.css
var connectionCSS string

//go:embed connection-page.js
var connectionJS string

var connectionTemplate = template.Must(template.New("connection").Parse(connectionHTML))

type connectionMessage struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func connectionMessages(kind string) map[string]connectionMessage {
	name, action := "preview", "reopen the preview from your project"
	if kind == "worker" {
		name, action = "worker", "select Open T3 on the Workers page"
	}
	return map[string]connectionMessage{
		"OPEN_FROM_WORKBENCH":          {"Open this " + name + " from Workbench", "Return to Workbench and " + action + " to create a new connection link."},
		"OPEN_FROM_WORKSPACE_LAUNCHER": {"Open this worker from Workbench", "Return to Workbench, go to Workers and select Open T3 to create a new connection link."},
		"CONNECTION_EXPIRED":           {"This connection link has expired", "Connection links expire quickly and can be used once. Return to Workbench and " + action + "."},
		"SESSION_EXPIRED":              {"Your " + name + " session has ended", "Return to Workbench and " + action + " to reconnect."},
		"NETWORK_ERROR":                {"Connection interrupted", "Check your network connection. Then return to Workbench and " + action + " using a fresh link."},
		"CONNECTION_TIMEOUT":           {"The connection is taking too long", "Return to Workbench and " + action + " using a fresh link. If this keeps happening, share the support details with your administrator."},
		"ORIGIN_REJECTED":              {"The connection could not be verified", "Open a fresh connection directly from Workbench. If this keeps happening, share the support details with your administrator."},
		"PREVIEW_UNAVAILABLE":          {"Preview unavailable", "The preview may have stopped or expired. Return to your project in Workbench to check its status and reopen it."},
		"INVALID_PREVIEW_RESPONSE":     {"The preview could not be loaded", "Return to Workbench and reopen the preview. If this keeps happening, share the support details with your administrator."},
		"UNSUPPORTED_PREVIEW_CONTENT":  {"This content cannot be previewed", "Return to Workbench to inspect or download the saved output instead."},
		"INVALID_PATH":                 {"This preview address is unavailable", "Return to Workbench and open the preview again from your project."},
		"TOO_MANY_CONNECTIONS":         {"Too many open connections", "Close unused preview or worker tabs, then open a fresh connection from Workbench."},
		"CONNECTION_UNAVAILABLE":       {"Connection unavailable", "Return to Workbench and " + action + " using a fresh link. If this keeps happening, share the support details with your administrator."},
	}
}

func serveConnectionAsset(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	var content, kind string
	switch r.URL.Path {
	case "/connect.js":
		content, kind = connectionJS, "text/javascript; charset=utf-8"
	case "/_workbench/connection.css":
		content, kind = connectionCSS, "text/css; charset=utf-8"
	default:
		return false
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(content))
	return true
}

func serveConnectionPage(w http.ResponseWriter, r *http.Request, kind, code string, status int) {
	messages := connectionMessages(kind)
	message := connectionMessage{"Opening preview", "Checking your preview connection link…"}
	if kind == "worker" {
		message = connectionMessage{"Connecting to your worker", "Checking your worker connection link…"}
	}
	if code != "" {
		if _, ok := messages[code]; !ok {
			code = "CONNECTION_UNAVAILABLE"
		}
		message = messages[code]
	}
	_, port, _ := net.SplitHostPort(r.Host)
	host := "localhost"
	if port != "" {
		host = net.JoinHostPort(host, port)
	}
	workbench := (&url.URL{Scheme: "http", Host: host, Path: "/app/"}).String()
	if label, ok := r.Context().Value(publicWorkerIngressKey{}).(string); ok {
		workbench = (&url.URL{Scheme: "https", Host: strings.TrimPrefix(r.Host, label+"."), Path: "/app/"}).String()
	}
	config, _ := json.Marshal(messages)
	parents := "'none'"
	if kind == "preview" {
		parents = previewParents(r)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors "+parents)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = connectionTemplate.Execute(w, struct {
		Title, Body, Code, Workbench, Messages string
		Status                                 int
		Embedded                               bool
	}{Title: message.Title, Body: message.Body, Code: code, Workbench: workbench, Messages: string(config), Status: status, Embedded: r.Header.Get("Sec-Fetch-Dest") == "iframe"})
}

// Browser navigation gets an actionable page. API/resource requests keep the
// same status and JSON contract, including ticket exchange and origin checks.
func writeConnectionError(w http.ResponseWriter, r *http.Request, status int, code, kind string) {
	destination := r.Header.Get("Sec-Fetch-Dest")
	if r.Method == http.MethodGet && (destination == "document" || destination == "iframe") && strings.Contains(r.Header.Get("Accept"), "text/html") {
		serveConnectionPage(w, r, kind, code, status)
		return
	}
	writeError(w, status, code)
}
