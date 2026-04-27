package trace

import (
	"context"
	"fmt"
	"strings"
)

type contextKey string

const requestIDKey contextKey = "request_id"

func WithRequestID(ctx context.Context, requestID string) context.Context {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, ok := ctx.Value(requestIDKey).(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

func Prefix(ctx context.Context) string {
	if requestID := RequestID(ctx); requestID != "" {
		return fmt.Sprintf("[req=%s] ", requestID)
	}
	return ""
}
