// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type LocatorAddSuite struct {
	ContextSuite
}

var _ = gc.Suite(&LocatorAddSuite{})

func (s *LocatorAddSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "locator-add")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `
Usage: locator-add [options] <locator-type> <locator-name> key=value [key=value ...]

Summary:
add service locator

Options:
-r, --relation  (= -1)
    specify a relation by id
-u, --unit (= "")
    specify a unit by id

Details:
locator-add adds the service locator, specified by type, name and params.
`[1:])
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}
