// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"context"
	"errors"
	"time"

	"gopkg.in/tomb.v2"
)

// SourceableError is an interface that can be implemented by errors
// that can be used as the source of a context error.
type SourceableError interface {
	Err() error
}

type sourceContext struct {
	ctx    context.Context
	source SourceableError
}

// WithSourceableError returns a new Context that carries the source.
// When the Err method is checked on the context, it will return the error
// from the source if the context was canceled or deadline exceeded.
func WithSourceableError(ctx context.Context, source SourceableError) context.Context {
	return &sourceContext{
		ctx:    ctx,
		source: source,
	}
}

// Deadline returns the time when work done on behalf of this context
// should be canceled. Deadline returns ok==false when no deadline is
// set. Successive calls to Deadline return the same results.
func (t *sourceContext) Deadline() (deadline time.Time, ok bool) {
	return t.ctx.Deadline()
}

// Done returns a channel that's closed when work done on behalf of this
// context should be canceled. Done may return nil if this context can
// never be canceled. Successive calls to Done return the same value.
// The close of the Done channel may happen asynchronously,
// after the cancel function returns.
func (t *sourceContext) Done() <-chan struct{} {
	return t.ctx.Done()
}

// Err if Done is not yet closed, Err returns nil.
// If Done is closed, Err returns a non-nil error explaining why:
// If the context was canceled or deadline exceeded, Err returns the source
// error if it exists, otherwise Canceled if the context was canceled
// or DeadlineExceeded if the context's deadline passed.
// After Err returns a non-nil error, successive calls to Err return the same
// error.
func (t *sourceContext) Err() error {
	cErr := t.ctx.Err()
	if cErr == nil {
		return nil
	}

	// If the context was canceled, check if the tomb has an error.
	// If the tomb has an error, return that error as it's more important.
	// If the tomb has no error, return the context error.
	if errors.Is(cErr, context.Canceled) || errors.Is(cErr, context.DeadlineExceeded) {
		if tErr := t.source.Err(); tErr != nil && !errors.Is(tErr, tomb.ErrStillAlive) {
			return tErr
		}
	}
	return cErr
}

// Value returns the value associated with this context for key, or nil
// if no value is associated with key. Successive calls to Value with
// the same key returns the same result.
func (t *sourceContext) Value(key any) any {
	return t.ctx.Value(key)
}
