// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/juju/tc"
)

type contextSuite struct{}

var _ = tc.Suite(&contextSuite{})

func (s *contextSuite) TestContextModelUUIDIsPassed(c *tc.C) {
	ctx := WithContextModelUUID(context.Background(), UUID("model-uuid"))
	modelUUID, ok := ModelUUIDFromContext(ctx)
	c.Assert(ok, tc.Equals, true)
	c.Check(modelUUID, tc.Equals, UUID("model-uuid"))
}
