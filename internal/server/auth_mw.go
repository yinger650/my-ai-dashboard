package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"agentboard/internal/api"
	"agentboard/internal/auth"
	"agentboard/internal/shared"
	"agentboard/internal/store"
)

// sessionCookieName returns the session cookie name. The __Host- prefix
// requires the Secure attribute, so it is only used when secure cookies are
// enabled (production); dev over plain HTTP uses a plain name.
func (s *Server) sessionCookieName() string {
	if s.cfg.SecureCookies {
		return "__Host-abp_session"
	}
	return "abp_session"
}

func (s *Server) markAbnormal(r *http.Request, result, reason string) {
	if ai := accessFrom(r.Context()); ai != nil {
		ai.result = result
		ai.reason = reason
		ai.isAbnormal = true
	}
}

func (s *Server) setActor(r *http.Request, actorType string, actorID *string) {
	if ai := accessFrom(r.Context()); ai != nil {
		ai.actorType = actorType
		ai.actorID = actorID
	}
}

// mwTokenAuth authenticates ingest requests via Bearer API token.
func (s *Server) mwTokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := requestID(r.Context())
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			s.markAbnormal(r, "unauthorized", "missing bearer token")
			api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
			return
		}
		full := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
		hash := auth.HashToken(full)

		tok, err := s.st.GetTokenByHash(r.Context(), hash)
		if errors.Is(err, store.ErrNotFound) {
			s.markAbnormal(r, "unauthorized", "unknown token")
			api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
			return
		}
		if err != nil {
			s.log.Error("token lookup failed", "err", err)
			api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
			return
		}
		if !tok.Enabled || tok.RevokedAt != nil {
			s.markAbnormal(r, "unauthorized", "token disabled/revoked")
			api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
			return
		}
		if tok.ExpiresAt != nil {
			if exp, perr := shared.ParseTime(*tok.ExpiresAt); perr == nil && time.Now().After(exp) {
				s.markAbnormal(r, "unauthorized", "token expired")
				api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
				return
			}
		}

		// IP allowlist (additive condition).
		if !ipAllowed(tok.IPAllowlistJSON, clientIP(r.Context())) {
			s.markAbnormal(r, "forbidden", "ip not allowed")
			api.WriteError(w, http.StatusForbidden, api.CodeForbidden, "forbidden", rid)
			return
		}

		// Per-token rate limit.
		if !s.limiter.Allow("tok:"+tok.ID, tok.RequestsPerMinute) {
			s.markAbnormal(r, "rate_limited", "requests per minute exceeded")
			api.WriteError(w, http.StatusTooManyRequests, api.CodeRateLimited, "rate limited", rid)
			return
		}

		actorType := "machine"
		if tok.Scope == auth.ScopeService {
			actorType = "service"
		} else if tok.Scope == auth.ScopeViewer {
			actorType = "viewer"
		}
		s.setActor(r, actorType, &tok.ID)

		// Async last-used update.
		go func(id, ip string) {
			c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = s.st.TouchTokenUsed(c, id, ip)
		}(tok.ID, clientIP(r.Context()))

		ctx := context.WithValue(r.Context(), ctxToken, tok)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ipAllowed(allowlistJSON, ip string) bool {
	var list []string
	if err := json.Unmarshal([]byte(allowlistJSON), &list); err != nil || len(list) == 0 {
		return true // empty allowlist = allow all
	}
	for _, entry := range list {
		if entry == ip {
			return true
		}
	}
	return false
}

// mwAdminSession requires a valid admin session cookie.
func (s *Server) mwAdminSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := requestID(r.Context())
		sess, ok := s.loadSession(r)
		if !ok {
			s.markAbnormal(r, "unauthorized", "no valid session")
			api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
			return
		}
		s.setActor(r, "admin", &sess.ID)
		ctx := context.WithValue(r.Context(), ctxSession, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) loadSession(r *http.Request) (*store.Session, bool) {
	c, err := r.Cookie(s.sessionCookieName())
	if err != nil || c.Value == "" {
		return nil, false
	}
	sess, err := s.st.GetSessionByTokenHash(r.Context(), auth.HashToken(c.Value))
	if err != nil {
		return nil, false
	}
	exp, err := shared.ParseTime(sess.ExpiresAt)
	if err != nil || time.Now().After(exp) {
		return nil, false
	}
	return sess, true
}

// mwCSRF enforces CSRF on state-changing admin requests.
func (s *Server) mwCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := requestID(r.Context())
		sess := sessionFrom(r.Context())
		if sess == nil {
			// Session middleware should run first; be safe.
			if loaded, ok := s.loadSession(r); ok {
				sess = loaded
			}
		}
		if sess == nil {
			api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
			return
		}
		provided := r.Header.Get("X-CSRF-Token")
		if provided == "" || !auth.ConstantTimeEqual(auth.HashToken(provided), sess.CSRFTokenHash) {
			s.markAbnormal(r, "forbidden", "csrf failed")
			api.WriteError(w, http.StatusForbidden, api.CodeForbidden, "forbidden", rid)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// mwViewerOrAdmin allows either an admin session or a viewer token.
func (s *Server) mwViewerOrAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := requestID(r.Context())
		if sess, ok := s.loadSession(r); ok {
			s.setActor(r, "admin", &sess.ID)
			ctx := context.WithValue(r.Context(), ctxSession, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		// Try viewer token.
		authz := r.Header.Get("Authorization")
		if strings.HasPrefix(authz, "Bearer ") {
			full := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
			tok, err := s.st.GetTokenByHash(r.Context(), auth.HashToken(full))
			if err == nil && tok.Enabled && tok.RevokedAt == nil && tok.Scope == auth.ScopeViewer {
				if !s.limiter.Allow("tok:"+tok.ID, 60) {
					api.WriteError(w, http.StatusTooManyRequests, api.CodeRateLimited, "rate limited", rid)
					return
				}
				s.setActor(r, "viewer", &tok.ID)
				ctx := context.WithValue(r.Context(), ctxToken, tok)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		s.markAbnormal(r, "unauthorized", "no admin session or viewer token")
		api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
	})
}
