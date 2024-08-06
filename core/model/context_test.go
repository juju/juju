// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	gc "gopkg.in/check.v1"
)

type contextSuite struct{}

var _ = gc.Suite(&contextSuite{})

func (s *contextSuite) TestContextModelUUIDIsPassed(c *gc.C) {
	ctx := WithContextModelUUID(context.Background(), UUID("model-uuid"))
	modelUUID, ok := ModelUUIDFromContext(ctx)
	c.Assert(ok, gc.Equals, true)
	c.Check(modelUUID, gc.Equals, UUID("model-uuid"))
}
