// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package output_test

import (
	"github.com/juju/ansiterm"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/status"
)

type OutputSuite struct{}

var _ = gc.Suite(&OutputSuite{})

func (s *OutputSuite) TestStatusColor(c *gc.C) {
	var ctx *ansiterm.Context

	unknown := status.Status("notKnown")
	allocating := status.Allocating

	ctx = output.StatusColor(unknown)
	c.Assert(ctx, gc.Equals, output.CurrentHighlight)

	ctx = output.StatusColor(allocating)
	c.Assert(ctx, gc.Equals, output.WarningHighlight)
}
