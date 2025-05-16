// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package output_test

import (
	stdtesting "testing"

	"github.com/juju/ansiterm"
	"github.com/juju/tc"

	"github.com/juju/juju/core/output"
	"github.com/juju/juju/core/status"
)

type OutputSuite struct{}

func TestOutputSuite(t *stdtesting.T) { tc.Run(t, &OutputSuite{}) }
func (s *OutputSuite) TestStatusColor(c *tc.C) {
	var ctx *ansiterm.Context

	unknown := status.Status("notKnown")
	allocating := status.Allocating

	ctx = output.StatusColor(unknown)
	c.Assert(ctx, tc.Equals, output.CurrentHighlight)

	ctx = output.StatusColor(allocating)
	c.Assert(ctx, tc.Equals, output.WarningHighlight)
}
