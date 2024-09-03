// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain"
)

type testingAtomicContext struct {
	ctx context.Context
}

// NewAtomicContext returns a stub implementation of domain.AtomicContext.
// Used for testing.
func NewAtomicContext(ctx context.Context) *testingAtomicContext {
	return &testingAtomicContext{
		ctx: ctx,
	}
}

func (t *testingAtomicContext) Context() context.Context {
	return t.ctx
}

// IsAtomicContextChecker is a gomock.Matcher that checks if the argument is an
// AtomicContext.
var IsAtomicContextChecker = gomock.AssignableToTypeOf(domain.AtomicContext(&testingAtomicContext{}))
