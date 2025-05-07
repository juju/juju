// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"context"

	"github.com/juju/tc"
	"github.com/juju/testing"

	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/internal/cmd"
)

type contextSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&contextSuite{})

func (s *contextSuite) TestBootstrapContext(c *tc.C) {
	ctx := environscmd.BootstrapContext(context.Background(), &cmd.Context{})
	c.Assert(ctx.ShouldVerifyCredentials(), tc.IsTrue)
}

func (s *contextSuite) TestBootstrapContextNoVerify(c *tc.C) {
	ctx := environscmd.BootstrapContextNoVerify(context.Background(), &cmd.Context{})
	c.Assert(ctx.ShouldVerifyCredentials(), tc.IsFalse)
}
