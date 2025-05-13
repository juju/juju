// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type stateGetSuite struct {
	stateSuite
}

var _ = tc.Suite(&stateGetSuite{})

type runStateGetCmd struct {
	description string
	args        []string
	out         string
	err         string
	code        int
	expect      func()
}

func (s *stateGetSuite) TestStateGet(c *tc.C) {
	runStateGetCmdTests := []runStateGetCmd{
		{
			description: "get all values with no args",
			args:        nil,
			out:         "one: two\n" + "three: four\n",
			expect:      s.expectStateGetTwo,
		},
		{
			description: "get all values with -",
			args:        []string{"-"},
			out:         "one: two\n" + "three: four\n",
			expect:      s.expectStateGetTwo,
		},
		{
			description: "get value of key",
			args:        []string{"one"},
			out:         "two\n",
			expect:      s.expectStateGetValueOne,
		},
		{
			description: "key not found, give me the error",
			args:        []string{"--strict", "five"},
			err:         "ERROR \"five\" not found\n",
			out:         "",
			expect:      s.expectStateGetValueNotFound,
			code:        1,
		},
		{
			description: "key not found",
			args:        []string{"five"},
			err:         "",
			out:         "",
			expect:      s.expectStateGetValueNotFound,
		},
		{
			description: "empty result",
			args:        []string{"five"},
			err:         "",
			out:         "",
			expect:      s.expectStateGetValueEmpty,
		},
	}

	for i, test := range runStateGetCmdTests {
		c.Logf("test %d of %d: %s", i+1, len(runStateGetCmdTests), test.description)
		defer s.setupMocks(c).Finish()
		test.expect()

		toolCmd, err := jujuc.NewCommand(s.mockContext, "state-get")
		c.Assert(err, tc.ErrorIsNil)

		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(toolCmd), ctx, test.args)
		c.Check(code, tc.Equals, test.code)
		c.Assert(bufferString(ctx.Stderr), tc.Equals, test.err)
		c.Assert(bufferString(ctx.Stdout), tc.Equals, test.out)
	}
}
