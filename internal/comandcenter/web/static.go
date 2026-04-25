package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Abraxas-365/claudio/internal/config"
	"github.com/Abraxas-365/claudio/internal/tools"
)

// handleServiceWorker serves /sw.js at root scope so the SW controls all pages.
func (ws *WebServer) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	content, err := staticFS.ReadFile("static/sw.js")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(content)
}

// handleDesignStatic serves static assets from the legacy designs directory.
// Route: GET /designs/static/{id}/{rest...}
// Serves from: ~/.claudio/designs/{id}/{rest}
// Path traversal is prevented by verifying the resolved path stays under designsDir.
func (ws *WebServer) handleDesignStatic(w http.ResponseWriter, r *http.Request) {
	designsDir := config.GetPaths().Designs
	id := r.PathValue("id")
	rest := r.PathValue("rest")

	// Reject any path component that looks like a traversal attempt early.
	if strings.Contains(id, "..") || strings.Contains(rest, "..") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	fp := filepath.Join(designsDir, id, rest)
	cleaned := filepath.Clean(designsDir) + string(os.PathSeparator)
	if !strings.HasPrefix(fp, cleaned) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, fp)
}

// handleDesignProject serves static assets from project-scoped design dirs.
// Route: GET /designs/project/{slug}/{id}/{rest...}
// Serves from: ~/.claudio/projects/{slug}/designs/{id}/{rest}
// Path traversal is prevented identically to handleDesignStatic.
func (ws *WebServer) handleDesignProject(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	id := r.PathValue("id")
	rest := r.PathValue("rest")

	if strings.Contains(slug, "..") || strings.Contains(id, "..") || strings.Contains(rest, "..") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	projectsDir := config.GetPaths().Projects
	designsDir := filepath.Join(projectsDir, slug, "designs")
	fp := filepath.Join(designsDir, id, rest)
	cleaned := filepath.Clean(designsDir) + string(os.PathSeparator)
	if !strings.HasPrefix(fp, cleaned) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// For HTML files inject the latest canvas shell at serve time so old
	// bundles on disk always get the current toolbar/JS without re-bundling.
	if strings.HasSuffix(strings.ToLower(fp), ".html") {
		raw, err := os.ReadFile(fp)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		html := string(raw)
		if !strings.Contains(html, "cc-canvas-root") {
			html = tools.InjectInfiniteCanvas(html)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(html))
		return
	}

	http.ServeFile(w, r, fp)
}
