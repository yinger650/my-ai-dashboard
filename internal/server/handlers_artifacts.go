package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"agentboard/internal/api"
	"agentboard/internal/event"
	"agentboard/internal/shared"
	"agentboard/internal/store"
)

var allowedArtifactMIME = map[string]bool{
	"image/png":                true,
	"image/jpeg":               true,
	"image/gif":                true,
	"image/webp":               true,
	"text/plain":               true,
	"text/markdown":            true,
	"text/csv":                 true,
	"application/json":         true,
	"application/pdf":          true,
	"application/zip":          true,
	"application/octet-stream": true,
}

func (s *Server) artifactQuota() int64 {
	return s.cfg.ArtifactQuotaBytes
}

func (s *Server) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	if _, err := s.st.GetServiceByID(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	list, err := s.st.ListArtifactsByService(r.Context(), id, 50)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	if list == nil {
		list = []*store.Artifact{}
	}
	used, _ := s.st.ArtifactBytesUsed(r.Context())
	api.WriteData(w, rid, map[string]any{
		"artifacts":   list,
		"bytes_used":  used,
		"quota_bytes": s.artifactQuota(),
		"max_upload":  s.cfg.MaxUploadBytes,
	}, nil)
}

func (s *Server) handleUploadArtifact(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	svc, err := s.st.GetServiceByID(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	art, code, msg, err := s.saveUploadedArtifact(r, svc.MachineID, &svc.ID, svc.ServiceKey)
	if err != nil {
		s.log.Error("artifact upload failed", "err", err)
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	if code != "" {
		status := http.StatusUnprocessableEntity
		switch code {
		case api.CodePayloadTooLarge:
			status = http.StatusRequestEntityTooLarge
		case api.CodeUnsupportedMedia:
			status = http.StatusUnsupportedMediaType
		case api.CodeQuotaExceeded:
			status = http.StatusConflict
		case api.CodeInvalidJSON, api.CodeValidationFailed:
			status = http.StatusBadRequest
		}
		s.markAbnormal(r, code, msg)
		api.WriteError(w, status, code, msg, rid)
		return
	}
	api.WriteCreated(w, rid, art)
}

func (s *Server) handleIngestArtifact(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	tok := tokenFrom(r.Context())
	if tok == nil {
		api.WriteError(w, http.StatusUnauthorized, api.CodeUnauthorized, "unauthorized", rid)
		return
	}
	ingestAuth, reason := s.resolveIngestAuth(r, tok)
	if ingestAuth == nil {
		s.markAbnormal(r, "forbidden", reason)
		api.WriteError(w, http.StatusForbidden, api.CodeForbidden, "forbidden", rid)
		return
	}
	var serviceID *string
	serviceKey := r.FormValue("service_key")
	if ingestAuth.ServiceID != nil {
		serviceID = ingestAuth.ServiceID
	} else if serviceKey != "" {
		svc, err := s.st.GetServiceByKey(r.Context(), ingestAuth.MachineID, serviceKey)
		if errors.Is(err, store.ErrNotFound) {
			api.WriteError(w, http.StatusUnprocessableEntity, api.CodeValidationFailed, "service not found", rid)
			return
		}
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
			return
		}
		serviceID = &svc.ID
	}
	art, code, msg, err := s.saveUploadedArtifact(r, ingestAuth.MachineID, serviceID, serviceKey)
	if err != nil {
		s.log.Error("ingest artifact failed", "err", err)
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	if code != "" {
		status := http.StatusUnprocessableEntity
		switch code {
		case api.CodePayloadTooLarge:
			status = http.StatusRequestEntityTooLarge
		case api.CodeUnsupportedMedia:
			status = http.StatusUnsupportedMediaType
		case api.CodeQuotaExceeded:
			status = http.StatusConflict
		}
		s.markAbnormal(r, code, msg)
		api.WriteError(w, status, code, msg, rid)
		return
	}
	api.WriteCreated(w, rid, art)
}

func (s *Server) saveUploadedArtifact(r *http.Request, machineID string, serviceID *string, serviceKey string) (*store.Artifact, string, string, error) {
	if err := r.ParseMultipartForm(s.cfg.MaxUploadBytes); err != nil {
		return nil, api.CodePayloadTooLarge, "payload too large or invalid multipart", nil
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		return nil, api.CodeValidationFailed, "file field required", nil
	}
	defer file.Close()
	if hdr.Size > s.cfg.MaxUploadBytes {
		return nil, api.CodePayloadTooLarge, "file exceeds max upload size", nil
	}

	used, err := s.st.ArtifactBytesUsed(r.Context())
	if err != nil {
		return nil, "", "", err
	}
	if used+hdr.Size > s.artifactQuota() {
		return nil, api.CodeQuotaExceeded, "artifact quota exceeded", nil
	}

	original := sanitizeFilename(hdr.Filename)
	mimeType := hdr.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		if ext := filepath.Ext(original); ext != "" {
			if g := mime.TypeByExtension(ext); g != "" {
				mimeType = g
			}
		}
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	if !allowedArtifactMIME[mimeType] {
		return nil, api.CodeUnsupportedMedia, "unsupported media type", nil
	}

	tmp, err := os.CreateTemp(s.cfg.ArtifactDir, "upload-*")
	if err != nil {
		if mkErr := os.MkdirAll(s.cfg.ArtifactDir, 0o750); mkErr != nil {
			return nil, "", "", mkErr
		}
		tmp, err = os.CreateTemp(s.cfg.ArtifactDir, "upload-*")
		if err != nil {
			return nil, "", "", err
		}
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(file, s.cfg.MaxUploadBytes+1))
	if err != nil {
		return nil, "", "", err
	}
	if n > s.cfg.MaxUploadBytes {
		return nil, api.CodePayloadTooLarge, "file exceeds max upload size", nil
	}

	id := shared.NewID()
	ext := filepath.Ext(original)
	stored := id + ext
	dest := filepath.Join(s.cfg.ArtifactDir, stored)

	sum := hex.EncodeToString(h.Sum(nil))
	now := shared.FormatTime(shared.NowUTC())
	eventID := shared.NewID()
	if serviceID != nil || serviceKey != "" {
		payload, _ := json.Marshal(event.LogPayload{
			Markdown:    "上传附件 **" + original + "**（" + itoa64(n) + " bytes）",
			Severity:    "info",
			Source:      "artifact",
			ArtifactIDs: []string{id},
		})
		env := &event.Envelope{
			SchemaVersion: 1,
			EventID:       eventID,
			EventType:     event.TypeLogAppend,
			OccurredAt:    now,
			ServiceKey:    serviceKey,
			Payload:       payload,
		}
		authz := store.IngestAuth{MachineID: machineID, ServiceID: serviceID}
		res, err := s.st.IngestEvent(r.Context(), env, authz, now)
		if err != nil {
			return nil, "", "", err
		}
		if res.Status == "rejected" {
			return nil, api.CodeValidationFailed, res.Message, nil
		}
	}

	art := &store.Artifact{
		ID:            id,
		UploadEventID: eventID,
		MachineID:     machineID,
		ServiceID:     serviceID,
		StoredName:    stored,
		OriginalName:  original,
		MIMEType:      mimeType,
		SizeBytes:     n,
		SHA256:        sum,
		CreatedAt:     now,
	}
	if err := s.st.InsertArtifact(r.Context(), art); err != nil {
		return nil, "", "", err
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		_ = s.st.SoftDeleteArtifact(r.Context(), id)
		return nil, "", "", err
	}
	return art, "", "", nil
}

func (s *Server) handleArtifactContent(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	art, err := s.st.GetArtifact(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	path := filepath.Join(s.cfg.ArtifactDir, art.StoredName)
	inline := r.URL.Query().Get("inline") == "1" && isPreviewable(art.MIMEType)
	disp := "attachment"
	if inline {
		disp = "inline"
	}
	w.Header().Set("Content-Type", art.MIMEType)
	w.Header().Set("Content-Disposition", disp+`; filename="`+strings.ReplaceAll(art.OriginalName, `"`, "")+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, path)
}

func (s *Server) handleDeleteArtifact(w http.ResponseWriter, r *http.Request) {
	rid := requestID(r.Context())
	id := chi.URLParam(r, "id")
	art, err := s.st.GetArtifact(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		api.WriteError(w, http.StatusNotFound, api.CodeNotFound, "not found", rid)
		return
	}
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	if err := s.st.SoftDeleteArtifact(r.Context(), id); err != nil {
		api.WriteError(w, http.StatusInternalServerError, api.CodeInternalError, "internal error", rid)
		return
	}
	_ = os.Remove(filepath.Join(s.cfg.ArtifactDir, art.StoredName))
	api.WriteData(w, rid, map[string]any{"deleted": true}, nil)
}

func isPreviewable(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "upload.bin"
	}
	if !utf8.ValidString(name) {
		return "upload.bin"
	}
	if utf8.RuneCountInString(name) > 200 {
		name = string([]rune(name)[:200])
	}
	return name
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [32]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
