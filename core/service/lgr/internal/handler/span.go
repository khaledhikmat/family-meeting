package handler

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type Span struct {
	slog.Handler
}

func (h *Span) Handle(ctx context.Context, record slog.Record) error {
	// Get the SpanContext from the golang Context.
	if s := trace.SpanContextFromContext(ctx); s.IsValid() {
		// Add trace context attributes following Cloud Logging structured log format described
		// in https://cloud.google.com/logging/docs/structured-logging#special-payload-fields
		record.AddAttrs(
			slog.Any("logging.googleapis.com/trace", s.TraceID()),
		)
		record.AddAttrs(
			slog.Any("logging.googleapis.com/spanId", s.SpanID()),
		)
		record.AddAttrs(
			slog.Bool("logging.googleapis.com/trace_sampled", s.TraceFlags().IsSampled()),
		)
	}

	return h.Handler.Handle(ctx, record)
}

func NewSpan(handler slog.Handler) *Span {
	h := &Span{
		Handler: handler,
	}

	return h
}
