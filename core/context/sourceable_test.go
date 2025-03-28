// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/errors"
)

type contextSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&contextSuite{})

func (s *contextSuite) TestSourceableErrorIsNilIfErrorIsNotContextError(c *gc.C) {
	var tomb tomb.Tomb
	tomb.Kill(errors.New("tomb error"))

	// We only want to propagate the sourceable error if the error is a
	// context error. Otherwise you can always check the error with the
	// source directly.

	ctx := WithSourceableError(context.Background(), &tomb)
	err := ctx.Err()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *contextSuite) TestSourceableErrorIsIgnoredIfNotInErrorState(c *gc.C) {
	var tomb tomb.Tomb

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ctx = WithSourceableError(ctx, &tomb)
	err := ctx.Err()
	c.Assert(err, jc.ErrorIs, context.Canceled)
}

func (s *contextSuite) TestSourceableErrorIsTombError(c *gc.C) {
	var tomb tomb.Tomb
	tomb.Kill(errors.New("boom"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ctx = WithSourceableError(ctx, &tomb)
	err := ctx.Err()
	c.Assert(err, gc.ErrorMatches, `boom`)
}

func (s *contextSuite) TestSourceableErrorIsTiedToTheTomb(c *gc.C) {
	var tomb tomb.Tomb

	ctx := tomb.Context(context.Background())

	tomb.Kill(errors.New("boom"))

	ctx = WithSourceableError(ctx, &tomb)
	err := ctx.Err()
	c.Assert(err, gc.ErrorMatches, `boom`)
}
