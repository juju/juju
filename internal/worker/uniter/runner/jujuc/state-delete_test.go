// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type stateDeleteSuite struct {
	stateSuite
}

var _ = gc.Suite(&stateDeleteSuite{})

func (s *stateDeleteSuite) TestHelp(c *gc.C) {
	toolCmd, err := jujuc.NewCommand(nil, "state-delete")
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(toolCmd), ctx, []string{"--help"})
	c.Check(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")

	var expectedHelp = `
Usage: state-delete <key>

Summary:
delete server-side-state key value pair

Details:
state-delete deletes the value of the server side state specified by key.

See also:
    state-get
    state-set
`[1:]
	c.Assert(bufferString(ctx.Stdout), gc.Equals, expectedHelp)
}

type runStateDeleteCmd struct {
	description string
	args        []string
	out         string
	expect      func()
}

func (s *stateDeleteSuite) TestStateDelete(c *gc.C) {
	runStateDeleteCmdTests := []runStateDeleteCmd{
		{
			description: "delete one",
			args:        []string{"five"},
			expect:      s.expectStateDeleteOne,
		},

		{
			description: "no arg",
			args:        []string{""},
			out:         "",
		},
	}
	for i, test := range runStateDeleteCmdTests {
		c.Logf("test %d of %d: %s", i+1, len(runStateDeleteCmdTests), test.description)
		defer s.setupMocks(c).Finish()
		if test.expect != nil {
			test.expect()
		}

		toolCmd, err := jujuc.NewCommand(s.mockContext, "state-delete")
		c.Assert(err, jc.ErrorIsNil)

		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(toolCmd), ctx, test.args)
		c.Check(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, test.out)
	}
}
