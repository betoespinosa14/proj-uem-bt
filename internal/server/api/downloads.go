package api

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/uem/uem/internal/server/installers"
)

var agentFile = regexp.MustCompile(`^uem-agent-(linux|darwin|windows)-(amd64|arm64)(\.exe)?$`)

// download serves cross-compiled agent binaries from the dist directory.
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "file")
	if !agentFile.MatchString(name) || s.cfg.AgentDistDir == "" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.cfg.AgentDistDir, name)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, path)
}

func (s *Server) installScript(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := installers.FS.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType+"; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(body)
	}
}
