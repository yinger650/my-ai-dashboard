package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"agentboard/internal/api"
	"agentboard/internal/event"
	"agentboard/internal/shared"
	"agentboard/internal/store"
)

func (s *Server) handleServiceDetail(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	svc, err := s.st.GetServiceByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	statuses, _ := s.st.ListStatuses(r.Context(), id)
	var pinned *store.PinnedLog
	if p, perr := s.st.GetPinnedLog(r.Context(), id); perr == nil {
		pinned = p
	}
	machine, _ := s.st.GetMachineByID(r.Context(), svc.MachineID)
	api.WriteData(w, rid, map[string]any{
		"service":  svc,
		"machine":  machine,
		"statuses": statuses,
		"pinned":   pinned,
	}, nil)
}

func (s *Server) handleServiceStatuses(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	statuses, err := s.st.ListStatuses(r.Context(), id)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, statuses, nil)
}

func (s *Server) handleServiceLogs(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	cursor := r.URL.Query().Get("cursor")
	logs, err := s.st.ListServiceLogs(r.Context(), id, cursor, 100)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	var next *string
	if len(logs) == 100 {
		c := logs[len(logs)-1].OccurredAt
		next = &c
	}
	api.WriteData(w, rid, logs, next)
}

func (s *Server) handleServiceRuns(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	runs, err := s.st.ListRuns(r.Context(), id, 50)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, runs, nil)
}

type createServiceRequest struct {
	MachineID   string `json:"machine_id"`
	ServiceKey  string `json:"service_key"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

func (s *Server) handleCreateService(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	var req createServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalidJSON, "invalid json", rid)
		return
	}
	if req.MachineID == "" || req.ServiceKey == "" || req.Name == "" || !event.ValidServiceType(req.Type) {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidationFailed, "invalid service fields", rid)
		return
	}
	if _, err := s.st.GetMachineByID(r.Context(), req.MachineID); errors.Is(err, store.ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "machine not found", rid)
		return
	}
	svc := &store.Service{MachineID: req.MachineID, ServiceKey: req.ServiceKey, Name: req.Name, Type: req.Type, Description: req.Description, Enabled: true}
	if err := s.st.CreateService(r.Context(), svc); err != nil {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidationFailed, "could not create service (duplicate key?)", rid)
		return
	}
	api.WriteCreated(w, rid, svc)
}

type updateServiceRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Enabled     *bool   `json:"enabled"`
	SortOrder   *int    `json:"sort_order"`
}

func (s *Server) handleUpdateService(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	svc, err := s.st.GetServiceByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	var req updateServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalidJSON, "invalid json", rid)
		return
	}
	name, desc, enabled, sortOrder := svc.Name, svc.Description, svc.Enabled, svc.SortOrder
	if req.Name != nil {
		name = *req.Name
	}
	if req.Description != nil {
		desc = *req.Description
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	if err := s.st.UpdateServiceFields(r.Context(), id, name, desc, enabled, sortOrder, svc.MetadataJSON); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	updated, _ := s.st.GetServiceByID(r.Context(), id)
	api.WriteData(w, rid, updated, nil)
}

func (s *Server) handleDeleteService(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	if _, err := s.st.GetServiceByID(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	if err := s.st.SoftDeleteService(r.Context(), id); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, map[string]any{"deleted": true}, nil)
}

var _ = shared.NowUTC
