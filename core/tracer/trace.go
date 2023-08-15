// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer

import (
	"context"

	"github.com/juju/juju/worker/tracer"
)

// Span represents a span in a trace.
type Span = tracer.Span

// Start returns a new context with the given tracer.
func Start(ctx context.Context, name string, options ...tracer.TracerOption) (context.Context, Span) {
	tracer := FromContext(ctx)
	return tracer.Start(ctx, name, options...)
}
