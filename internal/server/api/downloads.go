package api

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/uem/uem/internal/agentdist"
	"github.com/uem/uem/internal/server/installers"
)

var agentFile = regexp.MustCompile(`^uem-agent-(linux|darwin|windows)-(amd64|arm64)(\.exe)?$`)

// download serves cross-compiled agent binaries from the dist directory.
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "file")
	if name == agentdist.ManifestName && s.cfg.AgentDistDir != "" {
		m, err := agentdist.Load(s.cfg.AgentDistDir)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, m)
		return
	}
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

// latestAgent tells the console which agent version the server offers.
func (s *Server) latestAgent(w http.ResponseWriter, r *http.Request) {
	m, err := agentdist.Load(s.cfg.AgentDistDir)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"version": nil, "self_update_since": agentdist.SelfUpdateSince})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": m.Version, "self_update_since": agentdist.SelfUpdateSince})
}
