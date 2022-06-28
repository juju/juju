// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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

func (s *LocatorAddSuite) TestLocatorAdd(c *gc.C) {
	testCases := []struct {
		about  string
		cmd    []string
		result int
		stdout string
		stderr string
		expect []jujuc.ServiceLocator
	}{
		{
			"add single service locator",
			[]string{"locator-add", "test-type", "test-name", "k=v"},
			0,
			"",
			"",
			[]jujuc.ServiceLocator{{Type: "test-type", Name: "test-name"}},
		}, {
			"no parameters error",
			[]string{"locator-add"},
			2,
			"",
			"ERROR no arguments specified\n",
			nil,
		}}
	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		hctx := s.GetHookContext(c, -1, "")
		com, err := jujuc.NewCommand(hctx, t.cmd[0])
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		ret := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.cmd[1:])
		c.Check(ret, gc.Equals, t.result)
		c.Check(bufferString(ctx.Stdout), gc.Equals, t.stdout)
		c.Check(bufferString(ctx.Stderr), gc.Equals, t.stderr)
	}
}
