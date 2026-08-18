package server

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"

	"agentboard/internal/shared"
	"agentboard/internal/store"
)

type statusRecorder struct {
	http.ResponseWriter
	status  int
	written int64
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.written += int64(n)
	return n, err
}

func (s *Server) mwRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !shared.IsUUID(id) {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), ctxRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) mwRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic recovered", "request_id", requestID(r.Context()), "panic", rec)
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"code":"internal_error","message":"internal error"}}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) mwSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) mwClientIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := s.resolveClientIP(r)
		ctx := context.WithValue(r.Context(), ctxClientIP, ip)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolveClientIP uses X-Forwarded-For only for trusted proxies (spec 17.1).
func (s *Server) resolveClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if len(s.cfg.TrustedProxyCIDRs) > 0 && s.isTrustedProxy(host) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// Left-most is the original client.
			parts := splitComma(xff)
			if len(parts) > 0 {
				return parts[0]
			}
		}
	}
	return host
}

func (s *Server) isTrustedProxy(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, cidr := range s.cfg.TrustedProxyCIDRs {
		if _, ipnet, err := net.ParseCIDR(cidr); err == nil && ipnet.Contains(parsed) {
			return true
		}
	}
	return false
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, ch := range s {
		if ch == ',' {
			out = append(out, trimSpace(cur))
			cur = ""
			continue
		}
		cur += string(ch)
	}
	if cur != "" {
		out = append(out, trimSpace(cur))
	}
	return out
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// mwAccessLog times the request and writes an access log row afterwards.
func (s *Server) mwAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		ai := &accessInfo{actorType: "anonymous", result: "ok"}
		ctx := context.WithValue(r.Context(), ctxAccess, ai)
		r = r.WithContext(ctx)

		next.ServeHTTP(rec, r)

		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		if rec.status >= 400 && ai.result == "ok" {
			ai.result = "error"
		}
		ip := clientIP(r.Context())
		ua := r.UserAgent()
		entry := &store.AccessLog{
			OccurredAt: shared.FormatTime(start.UTC()),
			RequestID:  requestID(r.Context()),
			ActorType:  ai.actorType,
			ActorID:    ai.actorID,
			Method:     r.Method,
			Path:       r.URL.Path,
			StatusCode: rec.status,
			IP:         strPtr(ip),
			UserAgent:  strPtr(ua),
			BytesIn:    ai.bytesIn,
			DurationMs: time.Since(start).Milliseconds(),
			Result:     ai.result,
			Reason:     ai.reason,
			IsAbnormal: ai.isAbnormal,
		}
		// Write access log in the background so it never blocks the response.
		go func() {
			c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := s.st.InsertAccessLog(c, entry); err != nil {
				s.log.Warn("insert access log failed", "err", err)
			}
		}()
	})
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
