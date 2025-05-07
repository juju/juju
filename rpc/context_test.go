// Copyright 2023, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc_test

import (
	"context"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc"
)

type contextSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&contextSuite{})

func (s *contextSuite) TestWithTracing(c *tc.C) {
	ctx := rpc.WithTracing(context.Background(), "trace", "span", 1)
	traceID, spanID, flags := rpc.TracingFromContext(ctx)
	c.Assert(traceID, tc.Equals, "trace")
	c.Assert(spanID, tc.Equals, "span")
	c.Assert(flags, tc.Equals, 1)
}
