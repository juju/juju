// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type WorkloadVersionSetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&WorkloadVersionSetSuite{})

func (s *WorkloadVersionSetSuite) createCommand(c *gc.C, err error) (*Context, cmd.Command) {
	hctx := s.GetHookContext(c, -1, "")
	s.Stub.SetErrors(err)

	com, err := jujuc.NewCommand(hctx, cmdString("workload-version-set"))
	c.Assert(err, jc.ErrorIsNil)
	return hctx, com
}

func (s *WorkloadVersionSetSuite) TestWorkloadVersionSetNoArguments(c *gc.C) {
	hctx, com := s.createCommand(c, nil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, nil)
	c.Check(code, gc.Equals, 2)
	c.Check(bufferString(ctx.Stdout), gc.Equals, "")
	c.Check(bufferString(ctx.Stderr), gc.Equals, "error: no version specified\n")
	c.Check(hctx.info.Version.WorkloadVersion, gc.Equals, "")
}

func (s *WorkloadVersionSetSuite) TestWorkloadVersionSetWithArguments(c *gc.C) {
	hctx, com := s.createCommand(c, nil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"dia de los muertos"})
	c.Check(code, gc.Equals, 0)
	c.Check(bufferString(ctx.Stdout), gc.Equals, "")
	c.Check(bufferString(ctx.Stderr), gc.Equals, "")
	c.Check(hctx.info.Version.WorkloadVersion, gc.Equals, "dia de los muertos")
}

func (s *WorkloadVersionSetSuite) TestWorkloadVersionSetError(c *gc.C) {
	hctx, com := s.createCommand(c, errors.New("uh oh spaghettio"))
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"cannae"})
	c.Check(code, gc.Equals, 1)
	c.Check(bufferString(ctx.Stdout), gc.Equals, "")
	c.Check(bufferString(ctx.Stderr), gc.Equals, "error: uh oh spaghettio\n")
	c.Check(hctx.info.Version.WorkloadVersion, gc.Equals, "")
}

func (s *WorkloadVersionSetSuite) TestHelp(c *gc.C) {

	var helpTemplate = `
Usage: workload-version-set <new-version>

Summary:
set workload version

Details:
workload-version-set updates the workload version for the current unit
to the value passed to the command.
`[1:]

	_, com := s.createCommand(c, nil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Check(code, gc.Equals, 0)

	c.Check(bufferString(ctx.Stdout), gc.Equals, helpTemplate)
	c.Check(bufferString(ctx.Stderr), gc.Equals, "")
}
