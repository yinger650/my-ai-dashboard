package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"agentboard/internal/api"
	"agentboard/internal/auth"
	"agentboard/internal/shared"
	"agentboard/internal/store"
)

const totpPendingKey = "totp_pending"

func totpEnabled(c *store.AdminCredentials) bool {
	return c != nil && c.TOTPSecretEncrypted != nil && *c.TOTPSecretEncrypted != ""
}

func (s *Server) failLogin(w http.ResponseWriter, r *http.Request, rid string, attemptsSoFar int) {
	attempts := attemptsSoFar + 1
	var lockedUntil *string
	if attempts >= 5 {
		t := shared.FormatTime(time.Now().UTC().Add(15 * time.Minute))
		lockedUntil = &t
		attempts = 0
	}
	_ = s.st.SetFailedAttempts(r.Context(), attempts, lockedUntil)
	s.markAbnormal(r, "unauthorized", "bad credentials")
	api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
}

func (s *Server) verifyTOTPOrRecovery(r *http.Request, creds *store.AdminCredentials, code string) bool {
	if s.secretKey == nil || creds.TOTPSecretEncrypted == nil {
		return false
	}
	if auth.LooksLikeTOTP(code) {
		secret, err := auth.Decrypt(s.secretKey, *creds.TOTPSecretEncrypted)
		if err != nil {
			return false
		}
		return auth.VerifyTOTP(string(secret), code, time.Now())
	}
	if creds.RecoveryCodesHashJSON == nil || *creds.RecoveryCodesHashJSON == "" {
		return false
	}
	var hashes []string
	if err := json.Unmarshal([]byte(*creds.RecoveryCodesHashJSON), &hashes); err != nil {
		return false
	}
	idx := auth.MatchRecoveryHash(code, hashes)
	if idx < 0 {
		return false
	}
	hashes[idx] = ""
	raw, _ := json.Marshal(hashes)
	_ = s.st.SetRecoveryCodesHashJSON(r.Context(), string(raw))
	return true
}

func (s *Server) handleTOTPStatus(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	creds, err := s.st.GetAdminCredentials(r.Context())
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, map[string]any{"enabled": totpEnabled(creds)}, nil)
}

func (s *Server) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	if len(s.secretKey) != 32 {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "secret key not configured", rid)
		return
	}
	creds, err := s.st.GetAdminCredentials(r.Context())
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	if totpEnabled(creds) {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidationFailed, "totp already enabled", rid)
		return
	}
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	enc, err := auth.Encrypt(s.secretKey, []byte(secret))
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	pending, _ := json.Marshal(enc)
	if err := s.st.SetSetting(r.Context(), totpPendingKey, string(pending)); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, map[string]any{
		"secret":      secret,
		"otpauth_url": auth.OTPAuthURL("AgentBoard", "admin", secret),
	}, nil)
}

type totpCodeRequest struct {
	Code     string `json:"code"`
	Password string `json:"password"`
}

func (s *Server) handleTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	var req totpCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalidJSON, "invalid json", rid)
		return
	}
	raw, err := s.st.GetSetting(r.Context(), totpPendingKey)
	if err != nil {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidationFailed, "run totp setup first", rid)
		return
	}
	var enc string
	if err := json.Unmarshal([]byte(raw), &enc); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	secret, err := auth.Decrypt(s.secretKey, enc)
	if err != nil || !auth.VerifyTOTP(string(secret), req.Code, time.Now()) {
		s.markAbnormal(r, "unauthorized", "bad totp confirm")
		api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "invalid totp code", rid)
		return
	}
	plain, hashes, err := auth.GenerateRecoveryCodes()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	hashJSON, _ := json.Marshal(hashes)
	if err := s.st.SetAdminTOTP(r.Context(), enc, string(hashJSON)); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	_ = s.st.SetSetting(r.Context(), totpPendingKey, `""`)
	api.WriteData(w, rid, map[string]any{
		"enabled":        true,
		"recovery_codes": plain,
	}, nil)
}

func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	var req totpCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalidJSON, "invalid json", rid)
		return
	}
	creds, err := s.st.GetAdminCredentials(r.Context())
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	ok, err := auth.VerifyPassword(req.Password, creds.PasswordHash)
	if err != nil || !ok {
		s.markAbnormal(r, "unauthorized", "bad password for totp disable")
		api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
		return
	}
	if totpEnabled(creds) && !s.verifyTOTPOrRecovery(r, creds, req.Code) {
		s.markAbnormal(r, "unauthorized", "bad totp for disable")
		api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
		return
	}
	if err := s.st.ClearAdminTOTP(r.Context()); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, map[string]any{"enabled": false}, nil)
}

func (s *Server) handleTOTPRecovery(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	var req totpCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalidJSON, "invalid json", rid)
		return
	}
	creds, err := s.st.GetAdminCredentials(r.Context())
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	if !totpEnabled(creds) {
		api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidationFailed, "totp not enabled", rid)
		return
	}
	secret, err := auth.Decrypt(s.secretKey, *creds.TOTPSecretEncrypted)
	if err != nil || !auth.VerifyTOTP(string(secret), req.Code, time.Now()) {
		s.markAbnormal(r, "unauthorized", "bad totp for recovery regen")
		api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
		return
	}
	plain, hashes, err := auth.GenerateRecoveryCodes()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	hashJSON, _ := json.Marshal(hashes)
	if err := s.st.SetRecoveryCodesHashJSON(r.Context(), string(hashJSON)); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, map[string]any{"recovery_codes": plain}, nil)
}
