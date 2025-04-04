// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type stateDeleteSuite struct {
	stateSuite
}

var _ = gc.Suite(&stateDeleteSuite{})

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

		toolCmd, err := jujuc.NewHookCommand(s.mockContext, "state-delete")
		c.Assert(err, jc.ErrorIsNil)

		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(toolCmd), ctx, test.args)
		c.Check(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, test.out)
	}
}
