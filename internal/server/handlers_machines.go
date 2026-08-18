package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"

	"agentboard/internal/api"
	"agentboard/internal/auth"
	"agentboard/internal/shared"
	"agentboard/internal/store"
)

var machineKeyRe = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

func (s *Server) handleMachineDetail(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	m, err := s.st.GetMachineByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (m != nil && m.DeletedAt != nil) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	latest, _ := s.st.LatestMetric(r.Context(), id)
	health, resSev := machineHealth(m, latest, time.Now().UTC())
	api.WriteData(w, rid, map[string]any{
		"machine":           m,
		"latest_metric":     latest,
		"health":            health,
		"resource_severity": resSev,
	}, nil)
}

var rangeToSince = map[string]time.Duration{
	"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour,
	"7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour,
}

func (s *Server) handleMachineMetrics(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	rng := r.URL.Query().Get("range")
	dur, ok := rangeToSince[rng]
	if !ok {
		dur = time.Hour
	}
	since := shared.FormatTime(time.Now().UTC().Add(-dur))
	samples, err := s.st.MetricsSince(r.Context(), id, since, 1000)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, map[string]any{"range": rng, "samples": samples}, nil)
}

func (s *Server) handleMachineServices(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	svcs, err := s.st.ListServicesByMachine(r.Context(), id)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, svcs, nil)
}

func (s *Server) handleMachineLogs(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	logs, err := s.st.RecentMachineLogs(r.Context(), id, 50)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, logs, nil)
}

func (s *Server) handleListMachinesAdmin(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	machines, err := s.st.ListMachines(r.Context(), true)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, machines, nil)
}

type createMachineRequest struct {
	MachineKey         string `json:"machine_key"`
	Name               string `json:"name"`
	Kind               string `json:"kind"`
	Description        string `json:"description"`
	CreateMachineToken bool   `json:"create_machine_token"`
}

func validKind(k string) bool {
	switch k {
	case "physical", "vm", "container_host", "virtual":
		return true
	}
	return false
}

func (s *Server) handleCreateMachine(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	var req createMachineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalidJSON, "invalid json", rid)
		return
	}
	if !machineKeyRe.MatchString(req.MachineKey) {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidationFailed, "invalid machine_key", rid, api.Detail{Field: "machine_key", Reason: "must match [a-z0-9._-]{1,64}"})
		return
	}
	if req.Name == "" || !validKind(req.Kind) {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidationFailed, "invalid name/kind", rid)
		return
	}
	m := &store.Machine{MachineKey: req.MachineKey, Name: req.Name, Kind: req.Kind, Description: req.Description, Enabled: true, AutoCreateServices: true}
	if err := s.st.CreateMachine(r.Context(), m); err != nil {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidationFailed, "could not create machine (duplicate key?)", rid)
		return
	}

	resp := map[string]any{"machine": m}
	if req.CreateMachineToken {
		full, prefix, hash, err := auth.GenerateAPIToken(auth.ScopeMachine)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
			return
		}
		tok := &store.Token{Name: req.Name + " machine token", TokenPrefix: prefix, TokenHash: hash, Scope: auth.ScopeMachine, MachineID: &m.ID, Enabled: true}
		if err := s.st.CreateToken(r.Context(), tok); err != nil {
			api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
			return
		}
		resp["token"] = map[string]any{"id": tok.ID, "token": full, "prefix": prefix, "scope": tok.Scope}
	}
	api.WriteCreated(w, rid, resp)
}

type updateMachineRequest struct {
	Name        *string `json:"name"`
	Kind        *string `json:"kind"`
	Description *string `json:"description"`
	Enabled     *bool   `json:"enabled"`
}

func (s *Server) handleUpdateMachine(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	m, err := s.st.GetMachineByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (m != nil && m.DeletedAt != nil) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	var req updateMachineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalidJSON, "invalid json", rid)
		return
	}
	name, kind, desc, enabled := m.Name, m.Kind, m.Description, m.Enabled
	if req.Name != nil {
		name = *req.Name
	}
	if req.Kind != nil {
		if !validKind(*req.Kind) {
			api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidationFailed, "invalid kind", rid)
			return
		}
		kind = *req.Kind
	}
	if req.Description != nil {
		desc = *req.Description
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if err := s.st.UpdateMachineFields(r.Context(), id, name, kind, desc, enabled, m.MetadataJSON); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	updated, _ := s.st.GetMachineByID(r.Context(), id)
	api.WriteData(w, rid, updated, nil)
}

func (s *Server) handleDeleteMachine(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	if _, err := s.st.GetMachineByID(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	if err := s.st.SoftDeleteMachine(r.Context(), id); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, map[string]any{"deleted": true}, nil)
}
