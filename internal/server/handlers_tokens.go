package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"agentboard/internal/api"
	"agentboard/internal/auth"
	"agentboard/internal/store"
)

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	tokens, err := s.st.ListTokens(r.Context())
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, tokens, nil)
}

type createTokenRequest struct {
	Name                  string   `json:"name"`
	Scope                 string   `json:"scope"`
	MachineID             *string  `json:"machine_id"`
	ServiceID             *string  `json:"service_id"`
	IPAllowlist           []string `json:"ip_allowlist"`
	RequestsPerMinute     int      `json:"requests_per_minute"`
	BytesPerDay           int64    `json:"bytes_per_day"`
	AllowArtifactDownload bool     `json:"allow_artifact_download"`
}

func (s *Server) createTokenFromRequest(r *http.Request, req *createTokenRequest) (*store.Token, string, error, int) {
	switch req.Scope {
	case auth.ScopeMachine:
		if req.MachineID == nil {
			return nil, "", errors.New("machine_id required"), http.StatusUnprocessableEntity
		}
		if _, err := s.st.GetMachineByID(r.Context(), *req.MachineID); err != nil {
			return nil, "", errors.New("machine not found"), http.StatusNotFound
		}
	case auth.ScopeService:
		if req.ServiceID == nil {
			return nil, "", errors.New("service_id required"), http.StatusUnprocessableEntity
		}
		if _, err := s.st.GetServiceByID(r.Context(), *req.ServiceID); err != nil {
			return nil, "", errors.New("service not found"), http.StatusNotFound
		}
	case auth.ScopeViewer:
		// no binding
	default:
		return nil, "", errors.New("invalid scope"), http.StatusUnprocessableEntity
	}

	full, prefix, hash, err := auth.GenerateAPIToken(req.Scope)
	if err != nil {
		return nil, "", err, http.StatusInternalServerError
	}
	allowlistJSON := "[]"
	if len(req.IPAllowlist) > 0 {
		b, _ := json.Marshal(req.IPAllowlist)
		allowlistJSON = string(b)
	}
	tok := &store.Token{
		Name: req.Name, TokenPrefix: prefix, TokenHash: hash, Scope: req.Scope,
		MachineID: req.MachineID, ServiceID: req.ServiceID, IPAllowlistJSON: allowlistJSON,
		RequestsPerMinute: req.RequestsPerMinute, BytesPerDay: req.BytesPerDay,
		AllowArtifactDownload: req.AllowArtifactDownload, Enabled: true,
	}
	if err := s.st.CreateToken(r.Context(), tok); err != nil {
		return nil, "", err, http.StatusInternalServerError
	}
	return tok, full, nil, 0
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalidJSON, "invalid json", rid)
		return
	}
	if req.Name == "" {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidationFailed, "name required", rid)
		return
	}
	tok, full, err, status := s.createTokenFromRequest(r, &req)
	if err != nil {
		code := api.CodeValidationFailed
		if status == http.StatusNotFound {
			code = api.CodeNotFound
		} else if status == http.StatusInternalServerError {
			code = api.CodeInternalError
		}
		api.WriteError(w, status, code, err.Error(), rid)
		return
	}
	api.WriteCreated(w, rid, map[string]any{
		"id": tok.ID, "token": full, "prefix": tok.TokenPrefix, "scope": tok.Scope,
	})
}

func (s *Server) handleRotateToken(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	old, err := s.st.GetTokenByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	req := &createTokenRequest{
		Name: old.Name, Scope: old.Scope, MachineID: old.MachineID, ServiceID: old.ServiceID,
		RequestsPerMinute: old.RequestsPerMinute, BytesPerDay: old.BytesPerDay, AllowArtifactDownload: old.AllowArtifactDownload,
	}
	_ = json.Unmarshal([]byte(old.IPAllowlistJSON), &req.IPAllowlist)
	tok, full, cerr, status := s.createTokenFromRequest(r, req)
	if cerr != nil {
		api.WriteError(w, status, api.CodeInternalError, cerr.Error(), rid)
		return
	}
	if err := s.st.RevokeToken(r.Context(), old.ID); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteCreated(w, rid, map[string]any{
		"id": tok.ID, "token": full, "prefix": tok.TokenPrefix, "scope": tok.Scope, "revoked": old.ID,
	})
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	if _, err := s.st.GetTokenByID(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	if err := s.st.RevokeToken(r.Context(), id); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, map[string]any{"revoked": true}, nil)
}
