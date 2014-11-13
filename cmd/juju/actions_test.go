// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"

	"github.com/juju/cmd"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	yaml "gopkg.in/yaml.v2"
)

type ActionsSuite struct {
	testing.JujuConnSuite
	svc *state.Service
}

var _ = gc.Suite(&ActionsSuite{})

func (s *ActionsSuite) TestRun(c *gc.C) {
	tests := []struct {
		args            []string
		expectedResults map[string]interface{}
		expectedErr     string
		expectedCode    int
	}{{
		args:            []string{},
		expectedErr:     "error: no service name specified\n",
		expectedResults: map[string]interface{}{},
		expectedCode:    2,
	}, {
		args: []string{"dummy"},
		expectedResults: map[string]interface{}{
			"snapshot": map[string]interface{}{
				"description": "Take a snapshot of the database.",
				"params": map[string]interface{}{
					"outfile": map[string]interface{}{
						"default":     "foo.bz2",
						"type":        "string",
						"description": "The file to write out to.",
					},
				},
			},
		},
	}, {
		args:            []string{"dne"},
		expectedErr:     "error: service \"dne\" not found\n",
		expectedResults: map[string]interface{}{},
		expectedCode:    1,
	}, {
		args:            []string{"two", "things"},
		expectedErr:     "error: unrecognized args: [\"things\"]\n",
		expectedResults: map[string]interface{}{},
		expectedCode:    2,
	}, {
		args:            []string{"dummy", "things"},
		expectedErr:     "error: unrecognized args: [\"things\"]\n",
		expectedResults: map[string]interface{}{},
		expectedCode:    2,
	}}

	ch := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "dummy", ch)
	s.svc = svc

	for i, t := range tests {
		c.Logf("test %d: %#v", i, t.args)
		ctx := coretesting.Context(c)
		code := cmd.Main(envcmd.Wrap(&ActionsCommand{}), ctx, t.args)
		c.Check(code, gc.Equals, t.expectedCode)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, t.expectedErr)
		buf, err := yaml.Marshal(t.expectedResults)
		c.Assert(err, gc.IsNil)
		expected := make(map[string]interface{})
		err = yaml.Unmarshal(buf, &expected)
		c.Assert(err, gc.IsNil)
		actual := make(map[string]interface{})
		err = yaml.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &actual)
		c.Assert(err, gc.IsNil)
		c.Check(actual, jc.DeepEquals, expected)
	}
}
