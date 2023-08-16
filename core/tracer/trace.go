// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer

import (
	"context"
	"runtime"
	"strings"

	"github.com/juju/juju/worker/tracer"
)

// Span represents a span in a trace.
type Span = tracer.Span

// Start returns a new context with the given tracer.
func Start(ctx context.Context, options ...tracer.TracerOption) (context.Context, Span) {
	// Get caller frame.
	var pcs [1]uintptr
	n := runtime.Callers(2, pcs[:])
	if n < 1 {
		// TODO (stickupkid): Log a warning when this happens.
		return ctx, noopSpan{}
	}

	fn := runtime.FuncForPC(pcs[0])
	name := fn.Name()
	if lastSlash := strings.LastIndexByte(name, '/'); lastSlash > 0 {
		name = name[lastSlash+1:]
	}

	// Tracer is always guaranteed to be returned here. If there is no tracer
	// available it will return a noop tracer.
	tracer := FromContext(ctx)
	return tracer.Start(ctx, name, options...)
}
