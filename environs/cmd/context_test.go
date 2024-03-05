// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"context"

	"github.com/juju/cmd/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	environscmd "github.com/juju/juju/environs/cmd"
)

type contextSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&contextSuite{})

func (s *contextSuite) TestBootstrapContext(c *gc.C) {
	ctx := environscmd.BootstrapContext(context.Background(), &cmd.Context{})
	c.Assert(ctx.ShouldVerifyCredentials(), jc.IsTrue)
}

func (s *contextSuite) TestBootstrapContextNoVerify(c *gc.C) {
	ctx := environscmd.BootstrapContextNoVerify(context.Background(), &cmd.Context{})
	c.Assert(ctx.ShouldVerifyCredentials(), jc.IsFalse)
}
