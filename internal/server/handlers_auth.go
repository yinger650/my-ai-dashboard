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

type loginRequest struct {
	Password string `json:"password"`
	TOTPCode string `json:"totp_code"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())

	// Login rate limit: 10/min per IP (spec 17.3).
	if !s.limiter.Allow("login:"+clientIP(r.Context()), 10) {
		s.markAbnormal(r, "rate_limited", "login rate limited")
		api.WriteError(w, http.StatusTooManyRequests, api.CodeRateLimited, "rate limited", rid)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, api.CodeInvalidJSON, "invalid json", rid)
		return
	}

	creds, err := s.st.GetAdminCredentials(r.Context())
	if errors.Is(err, store.ErrNotFound) {
		s.markAbnormal(r, "unauthorized", "admin not initialized")
		api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
		return
	}
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}

	// Account lockout.
	if creds.LockedUntil != nil {
		if until, perr := shared.ParseTime(*creds.LockedUntil); perr == nil && time.Now().Before(until) {
			s.markAbnormal(r, "unauthorized", "account locked")
			api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
			return
		}
	}

	ok, err := auth.VerifyPassword(req.Password, creds.PasswordHash)
	if err != nil || !ok {
		s.failLogin(w, r, rid, creds.FailedAttempts)
		return
	}

	if creds.TOTPSecretEncrypted != nil && *creds.TOTPSecretEncrypted != "" {
		if req.TOTPCode == "" {
			api.WriteError(w, http.StatusUnauthorized, api.CodeTOTPRequired, "totp_required", rid)
			return
		}
		if !s.verifyTOTPOrRecovery(r, creds, req.TOTPCode) {
			s.failLogin(w, r, rid, creds.FailedAttempts)
			return
		}
	}

	// Reset failed attempts on success.
	_ = s.st.SetFailedAttempts(r.Context(), 0, nil)

	// Create session.
	sessTok, sessHash, err := auth.GenerateSessionToken()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	csrfTok, csrfHash, err := auth.GenerateSessionToken()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	now := time.Now().UTC()
	ip := clientIP(r.Context())
	ua := r.UserAgent()
	sess := &store.Session{
		ID:            shared.NewID(),
		TokenHash:     sessHash,
		CSRFTokenHash: csrfHash,
		CreatedAt:     shared.FormatTime(now),
		ExpiresAt:     shared.FormatTime(now.Add(time.Duration(s.cfg.SessionHours) * time.Hour)),
		LastSeenAt:    shared.FormatTime(now),
		IP:            strPtr(ip),
		UserAgent:     strPtr(ua),
	}
	if err := s.st.CreateSession(r.Context(), sess); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}

	http.SetCookie(w, s.sessionCookie(sessTok, sess.ExpiresAt))
	s.setActor(r, "admin", &sess.ID)
	api.WriteData(w, rid, map[string]any{
		"authenticated": true,
		"expires_at":    sess.ExpiresAt,
		"csrf_token":    csrfTok,
	}, nil)
}

func (s *Server) sessionCookie(value, expiresAt string) *http.Cookie {
	exp, _ := shared.ParseTime(expiresAt)
	c := &http.Cookie{
		Name:     s.sessionCookieName(),
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.cfg.SecureCookies,
		Expires:  exp,
	}
	return c
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	if sess := sessionFrom(r.Context()); sess != nil {
		_ = s.st.DeleteSession(r.Context(), sess.ID)
	}
	// Clear cookie.
	http.SetCookie(w, &http.Cookie{
		Name: s.sessionCookieName(), Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: s.cfg.SecureCookies, MaxAge: -1,
	})
	api.WriteData(w, rid, map[string]any{"authenticated": false}, nil)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	enabled := false
	if creds, err := s.st.GetAdminCredentials(r.Context()); err == nil {
		enabled = totpEnabled(creds)
	}
	sess, ok := s.loadSession(r)
	if !ok {
		api.WriteData(w, rid, map[string]any{"authenticated": false, "totp_enabled": enabled}, nil)
		return
	}
	// Issue a fresh CSRF token bound to this session.
	csrfTok, csrfHash, err := auth.GenerateSessionToken()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	if err := s.updateSessionCSRF(r, sess.ID, csrfHash); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	api.WriteData(w, rid, map[string]any{
		"authenticated": true,
		"expires_at":    sess.ExpiresAt,
		"totp_enabled":  enabled,
		"csrf_token":    csrfTok,
	}, nil)
}

func (s *Server) updateSessionCSRF(r *http.Request, sessionID, csrfHash string) error {
	_, err := s.st.DB().ExecContext(r.Context(), `UPDATE admin_sessions SET csrf_token_hash = ? WHERE id = ?`, csrfHash, sessionID)
	return err
}
