// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type ApplicationVersionSetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&ApplicationVersionSetSuite{})

func (s *ApplicationVersionSetSuite) createCommand(c *gc.C, err error) (*Context, cmd.Command) {
	hctx := s.GetHookContext(c, -1, "")
	s.Stub.SetErrors(err)

	com, err := jujuc.NewCommand(hctx, cmdString("application-version-set"))
	c.Assert(err, jc.ErrorIsNil)
	return hctx, com
}

func (s *ApplicationVersionSetSuite) TestApplicationVersionSetNoArguments(c *gc.C) {
	hctx, com := s.createCommand(c, nil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, nil)
	c.Check(code, gc.Equals, 2)
	c.Check(bufferString(ctx.Stdout), gc.Equals, "")
	c.Check(bufferString(ctx.Stderr), gc.Equals, "ERROR no version specified\n")
	c.Check(hctx.info.Version.WorkloadVersion, gc.Equals, "")
}

func (s *ApplicationVersionSetSuite) TestApplicationVersionSetWithArguments(c *gc.C) {
	hctx, com := s.createCommand(c, nil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, []string{"dia de los muertos"})
	c.Check(code, gc.Equals, 0)
	c.Check(bufferString(ctx.Stdout), gc.Equals, "")
	c.Check(bufferString(ctx.Stderr), gc.Equals, "")
	c.Check(hctx.info.Version.WorkloadVersion, gc.Equals, "dia de los muertos")
}

func (s *ApplicationVersionSetSuite) TestApplicationVersionSetError(c *gc.C) {
	hctx, com := s.createCommand(c, errors.New("uh oh spaghettio"))
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, []string{"cannae"})
	c.Check(code, gc.Equals, 1)
	c.Check(bufferString(ctx.Stdout), gc.Equals, "")
	c.Check(bufferString(ctx.Stderr), gc.Equals, "ERROR uh oh spaghettio\n")
	c.Check(hctx.info.Version.WorkloadVersion, gc.Equals, "")
}

func (s *ApplicationVersionSetSuite) TestHelp(c *gc.C) {

	var helpTemplate = `
Usage: application-version-set <new-version>

Summary:
specify which version of the application is deployed

Details:
application-version-set tells Juju which version of the application
software is running. This could be a package version number or some
other useful identifier, such as a Git hash, that indicates the
version of the deployed software. (It shouldn't be confused with the
charm revision.) The version set will be displayed in "juju status"
output for the application.
`[1:]

	_, com := s.createCommand(c, nil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Check(code, gc.Equals, 0)

	c.Check(bufferString(ctx.Stdout), gc.Equals, helpTemplate)
	c.Check(bufferString(ctx.Stderr), gc.Equals, "")
}
