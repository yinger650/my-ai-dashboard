// Package server wires the HTTP router, middleware and handlers for board-server.
package server

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"agentboard/internal/config"
	"agentboard/internal/store"
)

// Server holds shared dependencies for HTTP handlers.
type Server struct {
	cfg       *config.Config
	st        *store.Store
	log       *slog.Logger
	limiter   *rateLimiter
	webFS     fs.FS // optional embedded frontend (may be nil in dev)
	secretKey []byte
}

// New constructs a Server. webFS may be nil when the frontend is served
// externally (e.g. Vite dev server). secretKey is a 32-byte AES key used for TOTP.
func New(cfg *config.Config, st *store.Store, log *slog.Logger, webFS fs.FS, secretKey []byte) *Server {
	return &Server{cfg: cfg, st: st, log: log, limiter: newRateLimiter(), webFS: webFS, secretKey: secretKey}
}

// Router builds the chi router with the full middleware chain.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Global middleware chain (spec 13.1).
	r.Use(s.mwRequestID)
	r.Use(s.mwRecover)
	r.Use(s.mwSecurityHeaders)
	r.Use(s.mwClientIP)
	r.Use(s.mwAccessLog)

	// Health endpoints (no auth).
	r.Get("/health/live", s.handleLive)
	r.Get("/health/ready", s.handleReady)
	r.Get("/client-updates/{name}", s.handleGetClientUpdate)
	r.Put("/client-updates/{name}", s.handlePutClientUpdate)

	// Ingest API (token auth).
	r.Route("/ingest/v1", func(r chi.Router) {
		r.Use(s.mwTokenAuth)
		r.Get("/ping", s.handlePing)
		r.Post("/events", s.handleIngestEvents)
		r.Post("/artifacts", s.handleIngestArtifact)
	})

	// Admin auth.
	r.Post("/auth/login", s.handleLogin)
	r.Method(http.MethodPost, "/auth/logout", s.mwAdminSession(s.mwCSRF(http.HandlerFunc(s.handleLogout))))
	r.Get("/auth/session", s.handleSession)

	// Public/viewer + admin query API.
	r.Route("/api/v1", func(r chi.Router) {
		// board.txt allows admin session or viewer token.
		r.Method(http.MethodGet, "/board.txt", s.mwViewerOrAdmin(http.HandlerFunc(s.handleBoardTxt)))

		// Admin-only JSON API.
		r.Group(func(r chi.Router) {
			r.Use(func(next http.Handler) http.Handler { return s.mwAdminSession(next) })
			r.Get("/board", s.handleBoard)
			r.Get("/machines/{id}", s.handleMachineDetail)
			r.Get("/machines/{id}/metrics", s.handleMachineMetrics)
			r.Get("/machines/{id}/services", s.handleMachineServices)
			r.Get("/machines/{id}/logs", s.handleMachineLogs)
			r.Get("/machines/{id}/ports", s.handleMachinePorts)
			r.Get("/services/{id}", s.handleServiceDetail)
			r.Get("/services/{id}/statuses", s.handleServiceStatuses)
			r.Get("/services/{id}/logs", s.handleServiceLogs)
			r.Get("/services/{id}/runs", s.handleServiceRuns)
			r.Get("/services/{id}/artifacts", s.handleListArtifacts)
			r.Get("/artifacts/{id}/content", s.handleArtifactContent)
			r.Get("/admin/access-logs", s.handleAccessLogs)
			r.Get("/admin/settings", s.handleGetSettings)
			r.Get("/admin/tokens", s.handleListTokens)
			r.Get("/admin/machines", s.handleListMachinesAdmin)
			r.Get("/admin/totp", s.handleTOTPStatus)

			// Mutations require CSRF.
			r.Group(func(r chi.Router) {
				r.Use(func(next http.Handler) http.Handler { return s.mwCSRF(next) })
				r.Post("/admin/machines", s.handleCreateMachine)
				r.Patch("/admin/machines/{id}", s.handleUpdateMachine)
				r.Delete("/admin/machines/{id}", s.handleDeleteMachine)
				r.Post("/admin/services", s.handleCreateService)
				r.Patch("/admin/services/{id}", s.handleUpdateService)
				r.Delete("/admin/services/{id}", s.handleDeleteService)
				r.Post("/admin/tokens", s.handleCreateToken)
				r.Post("/admin/tokens/{id}/rotate", s.handleRotateToken)
				r.Delete("/admin/tokens/{id}", s.handleRevokeToken)
				r.Patch("/admin/settings", s.handlePatchSettings)
				r.Post("/admin/maintenance/run", s.handleMaintenanceRun)
				r.Post("/admin/totp/setup", s.handleTOTPSetup)
				r.Post("/admin/totp/confirm", s.handleTOTPConfirm)
				r.Post("/admin/totp/disable", s.handleTOTPDisable)
				r.Post("/admin/totp/recovery", s.handleTOTPRecovery)
				r.Post("/services/{id}/artifacts", s.handleUploadArtifact)
				r.Delete("/artifacts/{id}", s.handleDeleteArtifact)
				r.Post("/services/{id}/summarize", s.handleSummarizeLogs)
			})
		})
	})

	// Frontend (SPA) fallback when embedded.
	if s.webFS != nil {
		r.NotFound(s.serveSPA)
	}

	return r
}

// serveSPA serves embedded static files and falls back to index.html.
func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if f, err := s.webFS.Open(p); err == nil {
		_ = f.Close()
		http.FileServer(http.FS(s.webFS)).ServeHTTP(w, r)
		return
	}
	// SPA fallback.
	data, err := fs.ReadFile(s.webFS, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

var _ = context.Background
