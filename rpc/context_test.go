// Copyright 2023, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc_test

import (
	"context"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/rpc"
)

type contextSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&contextSuite{})

func (s *contextSuite) TestWithTracing(c *gc.C) {
	ctx := rpc.WithTracing(context.Background(), "trace", "span", 1)
	traceID, spanID, flags := rpc.TracingFromContext(ctx)
	c.Assert(traceID, gc.Equals, "trace")
	c.Assert(spanID, gc.Equals, "span")
	c.Assert(flags, gc.Equals, 1)
}
