package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"agentboard/internal/api"
	"agentboard/internal/auth"
	"agentboard/internal/shared"
	"agentboard/internal/store"
)

func (s *Server) handleAuthMeta(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	api.WriteData(w, rid, map[string]any{
		"edition": s.edition,
	}, nil)
}

func (s *Server) handleSlugRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) mwWorkspaceSlug(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := requestID(r.Context())
		if s.hub == nil {
			api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
			return
		}
		slug := chi.URLParam(r, "slug")
		t, err := s.hub.Open(r.Context(), slug)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
			return
		}
		ctx := context.WithValue(r.Context(), ctxTenant, t)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) attachSessionTenant(r *http.Request, sess *store.Session) (*http.Request, error) {
	if s.hub == nil || sess == nil || sess.WorkspaceSlug == "" {
		return r, nil
	}
	if t := s.tenant(r); t != nil && t.Slug == sess.WorkspaceSlug {
		return r, nil
	}
	t, err := s.hub.Open(r.Context(), sess.WorkspaceSlug)
	if err != nil {
		return r, err
	}
	return r.WithContext(context.WithValue(r.Context(), ctxTenant, t)), nil
}

func (s *Server) handleFeishuLogin(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	if s.oauth == nil || s.hub == nil {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	if !s.limiter.Allow("feishu-login:"+clientIP(r.Context()), 20) {
		s.markAbnormal(r, "rate_limited", "feishu login rate limited")
		api.WriteError(w, http.StatusTooManyRequests, api.CodeRateLimited, "rate limited", rid)
		return
	}
	state, _, err := auth.GenerateSessionToken()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	if err := s.hub.Control().PutOAuthState(r.Context(), state, 10*time.Minute); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	http.Redirect(w, r, s.oauth.AuthCodeURL(state), http.StatusFound)
}

func (s *Server) handleFeishuCallback(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	if s.oauth == nil || s.hub == nil {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	q := r.URL.Query()
	if q.Get("error") != "" {
		s.markAbnormal(r, "unauthorized", "feishu oauth error")
		http.Redirect(w, r, "/login?error=feishu", http.StatusFound)
		return
	}
	code := strings.TrimSpace(q.Get("code"))
	state := strings.TrimSpace(q.Get("state"))
	if code == "" || state == "" {
		http.Redirect(w, r, "/login?error=feishu", http.StatusFound)
		return
	}
	if err := s.hub.Control().ConsumeOAuthState(r.Context(), state); err != nil {
		s.markAbnormal(r, "unauthorized", "invalid oauth state")
		http.Redirect(w, r, "/login?error=feishu", http.StatusFound)
		return
	}
	tok, err := s.oauth.Exchange(code)
	if err != nil {
		s.log.Warn("feishu token exchange failed", "err", err)
		http.Redirect(w, r, "/login?error=feishu", http.StatusFound)
		return
	}
	info, err := s.oauth.UserInfo(tok)
	if err != nil {
		s.log.Warn("feishu user_info failed", "err", err)
		http.Redirect(w, r, "/login?error=feishu", http.StatusFound)
		return
	}
	if want := strings.TrimSpace(s.cfg.FeishuTenantKey); want != "" && info.TenantKey != want {
		s.markAbnormal(r, "forbidden", "feishu tenant mismatch")
		http.Redirect(w, r, "/login?error=tenant", http.StatusFound)
		return
	}
	_, ws, err := s.hub.EnsureUser(r.Context(), info.OpenID, info.UnionID, info.TenantKey, info.DisplayName, info.AvatarURL)
	if err != nil {
		s.log.Error("ensure feishu user failed", "err", err)
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	sessTok, sessHash, err := auth.GenerateSessionToken()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	_, csrfHash, err := auth.GenerateSessionToken()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	now := time.Now().UTC()
	ip := clientIP(r.Context())
	ua := r.UserAgent()
	sess := &store.Session{
		ID:            shared.NewID(),
		UserID:        ws.OwnerUserID,
		WorkspaceID:   ws.ID,
		WorkspaceSlug: ws.Slug,
		TokenHash:     sessHash,
		CSRFTokenHash: csrfHash,
		CreatedAt:     shared.FormatTime(now),
		ExpiresAt:     shared.FormatTime(now.Add(time.Duration(s.cfg.SessionHours) * time.Hour)),
		LastSeenAt:    shared.FormatTime(now),
		IP:            strPtr(ip),
		UserAgent:     strPtr(ua),
	}
	if err := s.hub.Control().CreateSession(r.Context(), sess); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	http.SetCookie(w, s.sessionCookie(sessTok, sess.ExpiresAt))
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) sessionPayload(sess *store.Session, csrfTok string, totpEnabled bool) map[string]any {
	out := map[string]any{
		"authenticated": true,
		"expires_at":    sess.ExpiresAt,
		"csrf_token":    csrfTok,
		"edition":       s.edition,
		"totp_enabled":  totpEnabled,
	}
	if sess.DisplayName != "" {
		out["display_name"] = sess.DisplayName
	}
	if sess.WorkspaceSlug != "" {
		out["workspace_slug"] = sess.WorkspaceSlug
	}
	return out
}
