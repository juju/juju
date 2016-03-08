// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/testing"
)

type GetSuite struct {
	fakeEnvSuite
}

var _ = gc.Suite(&GetSuite{})

func (s *GetSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := model.NewGetCommandForTest(s.fake)
	return testing.RunCommand(c, command, args...)
}

func (s *GetSuite) TestInit(c *gc.C) {
	// zero or one args is fine.
	err := testing.InitCommand(model.NewGetCommandForTest(s.fake), nil)
	c.Check(err, jc.ErrorIsNil)
	err = testing.InitCommand(model.NewGetCommandForTest(s.fake), []string{"one"})
	c.Check(err, jc.ErrorIsNil)
	// More than one is not allowed.
	err = testing.InitCommand(model.NewGetCommandForTest(s.fake), []string{"one", "two"})
	c.Check(err, gc.ErrorMatches, `unrecognized args: \["two"\]`)
}

func (s *GetSuite) TestSingleValue(c *gc.C) {
	context, err := s.run(c, "name")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	c.Assert(output, gc.Equals, "test-model")
}

func (s *GetSuite) TestSingleValueJSON(c *gc.C) {
	context, err := s.run(c, "--format=json", "name")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	c.Assert(output, gc.Equals, `"test-model"`)
}

func (s *GetSuite) TestAllValues(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	expected := "" +
		"name: test-model\n" +
		"running: true\n" +
		"special: special value"
	c.Assert(output, gc.Equals, expected)
}

func (s *GetSuite) TestAllValuesJSON(c *gc.C) {
	context, err := s.run(c, "--format=json")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	expected := `{"name":"test-model","running":true,"special":"special value"}`
	c.Assert(output, gc.Equals, expected)
}
