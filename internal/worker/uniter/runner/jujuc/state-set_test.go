// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"bytes"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type stateSetSuite struct {
	stateSuite
}

var _ = gc.Suite(&stateSetSuite{})

type runStateSetCmd struct {
	description string
	args        []string
	content     string
	code        int
	err         string
	expect      func()
}

func (s *stateSetSuite) TestStateSet(c *gc.C) {
	runStateSetCmdTests := []runStateSetCmd{
		{
			description: "no input",
			args:        nil,
		},
		{
			description: "set 1 values",
			args:        []string{"one=two"},
			expect:      s.expectStateSetOne,
		},
		{
			description: "set 2 values",
			args:        []string{"one=two", "three=four"},
			expect:      s.expectStateSetTwo,
		},
		{
			description: "key value pairs from file, yaml",
			args:        []string{"--file", "-"},
			content:     "{one: two, three: four}",
			expect:      s.expectStateSetTwo,
		},
		{
			description: "key value pairs from file, not yaml",
			args:        []string{"--file", "-"},
			content:     "one = two",
			code:        1,
			err:         "ERROR yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `one = two` into map[string]string\n",
		},
		{
			description: "single work, not equal sign",
			args:        []string{"five"},
			code:        2,
			err:         "ERROR expected \"key=value\", got \"five\"\n",
		},
		{
			description: "set key with empty value",
			args:        []string{"one="},
			expect:      s.expectStateSetOneEmpty,
		},
	}
	for i, test := range runStateSetCmdTests {
		c.Logf("test %d of %d: %s", i+1, len(runStateSetCmdTests), test.description)
		defer s.setupMocks(c).Finish()
		if test.expect != nil {
			test.expect()
		}

		toolCmd, err := jujuc.NewCommand(s.mockContext, "state-set")
		c.Assert(err, jc.ErrorIsNil)

		ctx := cmdtesting.Context(c)
		if test.content != "" {
			ctx.Stdin = bytes.NewBufferString(test.content)
		}
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(toolCmd), ctx, test.args)
		c.Check(code, gc.Equals, test.code)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, test.err)
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	}
}

func (s *stateSetSuite) TestStateSetExistingEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateSetOne()
	s.expectStateSetOneEmpty()

	toolCmd, err := jujuc.NewCommand(s.mockContext, "state-set")
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)

	for _, arg := range []string{"one=two", "one="} {
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(toolCmd), ctx, []string{arg})
		c.Check(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	}
}
