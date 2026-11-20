package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/uem/uem/docs/kb"
)

// listKB returns every knowledge base article, bodies included: the whole set
// is small, and sending it at once lets the console search it as you type.
func (s *Server) listKB(w http.ResponseWriter, r *http.Request) {
	list, err := kb.All()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "knowledge base: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sections": kb.Sections, "articles": list})
}

func (s *Server) getKB(w http.ResponseWriter, r *http.Request) {
	a, ok := kb.Get(chi.URLParam(r, "slug"))
	if !ok {
		writeErr(w, http.StatusNotFound, "article not found")
		return
	}
	writeJSON(w, http.StatusOK, a)
}
