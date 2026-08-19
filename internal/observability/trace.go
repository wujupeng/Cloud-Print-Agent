package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"go.uber.org/zap"
)

type ctxKey int

const (
	ctxKeyTraceID ctxKey = iota
	ctxKeyLogger
)

func generateUUIDv7() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		now := time.Now().UnixNano()
		for i := 0; i < 16; i++ {
			b[i] = byte(now >> (i % 8))
		}
	}

	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 8)
	b[1] = byte(ms)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 40)
	b[5] = byte(ms >> 32)

	b[6] = (b[6] & 0x0F) | 0x70
	b[8] = (b[8] & 0x3F) | 0x80

	return hex.EncodeToString(b[0:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:16])
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		traceID = generateUUIDv7()
	}
	return context.WithValue(ctx, ctxKeyTraceID, traceID)
}

func TraceIDFromCtx(ctx context.Context) string {
	if ctx == nil {
		return generateUUIDv7()
	}
	if v, ok := ctx.Value(ctxKeyTraceID).(string); ok && v != "" {
		return v
	}
	return generateUUIDv7()
}

func WithLogger(ctx context.Context, logger *zap.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	traceID := TraceIDFromCtx(ctx)
	return context.WithValue(ctx, ctxKeyLogger, logger.With(FieldTraceID(traceID)))
}

func LoggerFromCtx(ctx context.Context, base *zap.Logger) *zap.Logger {
	if ctx != nil {
		if v, ok := ctx.Value(ctxKeyLogger).(*zap.Logger); ok && v != nil {
			return v
		}
	}
	if base == nil {
		return zap.NewNop()
	}
	return base.With(FieldTraceID(TraceIDFromCtx(ctx)))
}