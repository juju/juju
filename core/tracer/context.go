// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer

import (
	"context"

	"github.com/juju/juju/worker/tracer"
)

type contextKey string

const tracerContextKey contextKey = "tracer"

// FromContext returns a tracer from the context. If no tracer is found,
// an empty tracer is returned.
func FromContext(ctx context.Context) tracer.Tracer {
	value := ctx.Value(tracerContextKey)
	if value == nil {
		return noopTracer{}
	}
	tracer, ok := ctx.Value(tracerContextKey).(tracer.Tracer)
	if !ok {
		return noopTracer{}
	}
	return tracer
}

// WithTracer returns a new context with the given tracer.
func WithTracer(ctx context.Context, tracer tracer.Tracer) context.Context {
	return context.WithValue(ctx, tracerContextKey, tracer)
}

type noopTracer struct{}

func (noopTracer) Start(ctx context.Context, name string, options ...tracer.TracerOption) (context.Context, tracer.Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) End() {}
