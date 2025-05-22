// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type stateDeleteSuite struct {
	stateSuite
}

func TestStateDeleteSuite(t *stdtesting.T) {
	tc.Run(t, &stateDeleteSuite{})
}

type runStateDeleteCmd struct {
	description string
	args        []string
	out         string
	expect      func()
}

func (s *stateDeleteSuite) TestStateDelete(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)

		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(toolCmd), ctx, test.args)
		c.Check(code, tc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), tc.Equals, test.out)
	}
}
