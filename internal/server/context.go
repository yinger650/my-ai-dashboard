package server

import (
	"context"

	"agentboard/internal/store"
)

type ctxKey int

const (
	ctxRequestID ctxKey = iota
	ctxClientIP
	ctxToken
	ctxSession
	ctxAccess
)

// accessInfo accumulates data for the access log entry for a request.
type accessInfo struct {
	actorType  string
	actorID    *string
	result     string
	reason     string
	isAbnormal bool
	bytesIn    int64
}

func requestID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxRequestID).(string); ok {
		return v
	}
	return ""
}

func clientIP(ctx context.Context) string {
	if v, ok := ctx.Value(ctxClientIP).(string); ok {
		return v
	}
	return ""
}

func tokenFrom(ctx context.Context) *store.Token {
	if v, ok := ctx.Value(ctxToken).(*store.Token); ok {
		return v
	}
	return nil
}

func sessionFrom(ctx context.Context) *store.Session {
	if v, ok := ctx.Value(ctxSession).(*store.Session); ok {
		return v
	}
	return nil
}

func accessFrom(ctx context.Context) *accessInfo {
	if v, ok := ctx.Value(ctxAccess).(*accessInfo); ok {
		return v
	}
	return nil
}
