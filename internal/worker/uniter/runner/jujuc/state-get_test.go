// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type stateGetSuite struct {
	stateSuite
}

var _ = gc.Suite(&stateGetSuite{})

type runStateGetCmd struct {
	description string
	args        []string
	out         string
	err         string
	code        int
	expect      func()
}

func (s *stateGetSuite) TestStateGet(c *gc.C) {
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

		toolCmd, err := jujuc.NewHookCommand(s.mockContext, "state-get")
		c.Assert(err, jc.ErrorIsNil)

		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(toolCmd), ctx, test.args)
		c.Check(code, gc.Equals, test.code)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, test.err)
		c.Assert(bufferString(ctx.Stdout), gc.Equals, test.out)
	}
}
