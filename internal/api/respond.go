// Package api provides shared HTTP response helpers and error formatting.
package api

import (
	"encoding/json"
	"net/http"
)

// Meta is the metadata block on successful responses.
type Meta struct {
	RequestID  string  `json:"request_id"`
	NextCursor *string `json:"next_cursor"`
}

// Detail is one field-level validation error.
type Detail struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

type errorBody struct {
	Code      string   `json:"code"`
	Message   string   `json:"message"`
	Details   []Detail `json:"details,omitempty"`
	RequestID string   `json:"request_id"`
}

// Error codes (spec 12.10).
const (
	CodeInvalidJSON       = "invalid_json"
	CodeUnauthorized      = "unauthorized"
	CodeForbidden         = "forbidden"
	CodeNotFound          = "not_found"
	CodeEventConflict     = "event_conflict"
	CodeInvalidTransition = "invalid_transition"
	CodePayloadTooLarge   = "payload_too_large"
	CodeUnsupportedMedia  = "unsupported_media_type"
	CodeValidationFailed  = "validation_failed"
	CodeRateLimited       = "rate_limited"
	CodeQuotaExceeded     = "quota_exceeded"
	CodeInternalError     = "internal_error"
	CodeNotReady          = "not_ready"
	CodeUnsupportedEvent  = "unsupported_event_type"
	CodeTOTPRequired      = "totp_required"
)

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteData writes a success envelope with data and an optional next cursor.
func WriteData(w http.ResponseWriter, requestID string, data any, nextCursor *string) {
	WriteJSON(w, http.StatusOK, map[string]any{
		"data": data,
		"meta": Meta{RequestID: requestID, NextCursor: nextCursor},
	})
}

// WriteCreated writes a 201 success envelope.
func WriteCreated(w http.ResponseWriter, requestID string, data any) {
	WriteJSON(w, http.StatusCreated, map[string]any{
		"data": data,
		"meta": Meta{RequestID: requestID},
	})
}

// WriteError writes a structured error envelope.
func WriteError(w http.ResponseWriter, status int, code, message, requestID string, details ...Detail) {
	WriteJSON(w, status, map[string]any{
		"error": errorBody{Code: code, Message: message, Details: details, RequestID: requestID},
	})
}
