// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/testhelpers"
)

type contextSuite struct {
	testhelpers.IsolationSuite
}

func TestContextSuite(t *stdtesting.T) {
	tc.Run(t, &contextSuite{})
}

func (s *contextSuite) TestBootstrapContext(c *tc.C) {
	ctx := environscmd.BootstrapContext(c.Context(), &cmd.Context{})
	c.Assert(ctx.ShouldVerifyCredentials(), tc.IsTrue)
}

func (s *contextSuite) TestBootstrapContextNoVerify(c *tc.C) {
	ctx := environscmd.BootstrapContextNoVerify(c.Context(), &cmd.Context{})
	c.Assert(ctx.ShouldVerifyCredentials(), tc.IsFalse)
}
