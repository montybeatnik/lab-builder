package app

import (
	"bytes"
	"net/http"
)

func (h *Handlers) renderPage(w http.ResponseWriter, page string) {
	var buf bytes.Buffer
	data := pageData{BaseDir: h.cfg.BaseDir, Page: page}
	if err := h.templates.ExecuteTemplate(&buf, "layout", data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "index")
}

func (h *Handlers) Lab(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "health")
}

func (h *Handlers) Viewer(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "viewer")
}
