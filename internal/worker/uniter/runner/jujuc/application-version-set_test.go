// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type ApplicationVersionSetSuite struct {
	ContextSuite
}

func TestApplicationVersionSetSuite(t *stdtesting.T) {
	tc.Run(t, &ApplicationVersionSetSuite{})
}
func (s *ApplicationVersionSetSuite) createCommand(c *tc.C, err error) (*Context, cmd.Command) {
	hctx := s.GetHookContext(c, -1, "")
	s.Stub.SetErrors(err)

	com, err := jujuc.NewCommand(hctx, "application-version-set")
	c.Assert(err, tc.ErrorIsNil)
	return hctx, jujuc.NewJujucCommandWrappedForTest(com)
}

func (s *ApplicationVersionSetSuite) TestApplicationVersionSetNoArguments(c *tc.C) {
	hctx, com := s.createCommand(c, nil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, nil)
	c.Check(code, tc.Equals, 2)
	c.Check(bufferString(ctx.Stdout), tc.Equals, "")
	c.Check(bufferString(ctx.Stderr), tc.Equals, "ERROR no version specified\n")
	c.Check(hctx.info.Version.WorkloadVersion, tc.Equals, "")
}

func (s *ApplicationVersionSetSuite) TestApplicationVersionSetWithArguments(c *tc.C) {
	hctx, com := s.createCommand(c, nil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, []string{"dia de los muertos"})
	c.Check(code, tc.Equals, 0)
	c.Check(bufferString(ctx.Stdout), tc.Equals, "")
	c.Check(bufferString(ctx.Stderr), tc.Equals, "")
	c.Check(hctx.info.Version.WorkloadVersion, tc.Equals, "dia de los muertos")
}

func (s *ApplicationVersionSetSuite) TestApplicationVersionSetError(c *tc.C) {
	hctx, com := s.createCommand(c, errors.New("uh oh spaghettio"))
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, []string{"cannae"})
	c.Check(code, tc.Equals, 1)
	c.Check(bufferString(ctx.Stdout), tc.Equals, "")
	c.Check(bufferString(ctx.Stderr), tc.Equals, "ERROR uh oh spaghettio\n")
	c.Check(hctx.info.Version.WorkloadVersion, tc.Equals, "")
}
