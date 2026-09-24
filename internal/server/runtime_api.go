package server

import (
	"net/http"

	"kino/internal/catalog"
)

func (s *Server) listSourceAdapters(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListSourceAdapters(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeCollection(w, r, items)
}
func (s *Server) createSourceAdapter(w http.ResponseWriter, r *http.Request) {
	var in catalog.NewSourceAdapter
	if !decode(w, r, &in) {
		return
	}
	item, err := s.store.CreateSourceAdapter(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", resourceLocation(r, "source-adapters", item.ID))
	writeJSON(w, http.StatusCreated, item)
}
func (s *Server) getSourceAdapter(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.GetSourceAdapter(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (s *Server) updateSourceAdapter(w http.ResponseWriter, r *http.Request) {
	var in catalog.NewSourceAdapter
	if !decode(w, r, &in) {
		return
	}
	item, err := s.store.UpdateSourceAdapter(r.Context(), r.PathValue("id"), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (s *Server) deleteSourceAdapter(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteSourceAdapter(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listFrontendAdapters(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListFrontendAdapters(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeCollection(w, r, items)
}
func (s *Server) createFrontendAdapter(w http.ResponseWriter, r *http.Request) {
	var in catalog.NewFrontendAdapter
	if !decode(w, r, &in) {
		return
	}
	item, err := s.store.CreateFrontendAdapter(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", resourceLocation(r, "frontend-adapters", item.ID))
	writeJSON(w, http.StatusCreated, item)
}
func (s *Server) getFrontendAdapter(w http.ResponseWriter, r *http.Request) {
	item, err := s.store.GetFrontendAdapter(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (s *Server) updateFrontendAdapter(w http.ResponseWriter, r *http.Request) {
	var in catalog.NewFrontendAdapter
	if !decode(w, r, &in) {
		return
	}
	item, err := s.store.UpdateFrontendAdapter(r.Context(), r.PathValue("id"), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}
func (s *Server) deleteFrontendAdapter(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteFrontendAdapter(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
