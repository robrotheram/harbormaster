package dashboard

import (
	"net/http"
	"os"
	"path"
	"strings"
)

func frontendDir() string {
	if dir := os.Getenv("FRONTEND_DIR"); dir != "" {
		return dir
	}
	return "frontend/dist"
}

func serveFrontend(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "GET" || (!strings.HasPrefix(r.URL.Path, "/app") && r.URL.Path != "/workspaces") {
		return false
	}
	dir := frontendDir()
	if _, err := os.Stat(dir + "/index.html"); err != nil {
		if r.URL.Path == "/workspaces" {
			return false
		}
		writeError(w, 503, "FRONTEND_NOT_BUILT")
		return true
	}
	if r.URL.Path == "/workspaces" {
		http.Redirect(w, r, "/app/", http.StatusTemporaryRedirect)
		return true
	}
	if r.URL.Path == "/app" {
		http.Redirect(w, r, "/app/", http.StatusTemporaryRedirect)
		return true
	}
	if !strings.HasPrefix(r.URL.Path, "/app/") {
		return false
	}
	frames := "http://*.localhost:*"
	if origin, ok := r.Context().Value(authenticatedOriginKey{}).(string); ok && strings.HasPrefix(origin, "https://") {
		frames = "https://*." + strings.TrimPrefix(origin, "https://")
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data: blob:; connect-src 'self'; frame-src "+frames+"; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
	name := strings.TrimPrefix(r.URL.Path, "/app/")
	if name == "" || path.Ext(name) == "" {
		http.ServeFile(w, r, dir+"/index.html")
		return true
	}
	http.StripPrefix("/app/", http.FileServer(http.Dir(dir))).ServeHTTP(w, r)
	return true
}
