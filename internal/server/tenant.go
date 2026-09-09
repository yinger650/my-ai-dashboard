package server

import (
	"context"
	"net/http"

	"agentboard/internal/store"
	"agentboard/internal/workspace"
)

const (
	EditionPersonal = "personal"
	EditionFeishu   = "feishu"
)

func (s *Server) feishuMode() bool {
	return s.edition == EditionFeishu
}

func (s *Server) tenant(r *http.Request) *workspace.Tenant {
	if r != nil {
		if v, ok := r.Context().Value(ctxTenant).(*workspace.Tenant); ok && v != nil {
			return v
		}
	}
	return s.defaultTenant
}

func (s *Server) dbFrom(ctx context.Context) *store.Store {
	if ctx != nil {
		if v, ok := ctx.Value(ctxTenant).(*workspace.Tenant); ok && v != nil && v.Store != nil {
			return v.Store
		}
	}
	if s.defaultTenant != nil && s.defaultTenant.Store != nil {
		return s.defaultTenant.Store
	}
	return s.st
}

func (s *Server) db(r *http.Request) *store.Store {
	if r == nil {
		return s.dbFrom(context.Background())
	}
	return s.dbFrom(r.Context())
}

func (s *Server) artifactDir(r *http.Request) string {
	if t := s.tenant(r); t != nil && t.ArtifactDir != "" {
		return t.ArtifactDir
	}
	if s.cfg != nil {
		return s.cfg.ArtifactDir
	}
	return ""
}

func (s *Server) ingestBase(r *http.Request) string {
	if t := s.tenant(r); t != nil {
		return t.IngestBase
	}
	return ""
}
