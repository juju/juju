// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/environment"
	"github.com/juju/juju/testing"
)

type GetSuite struct {
	fakeEnvSuite
}

var _ = gc.Suite(&GetSuite{})

func (s *GetSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := environment.NewGetCommand(s.fake)
	return testing.RunCommand(c, envcmd.Wrap(command), args...)
}

func (s *GetSuite) TestInit(c *gc.C) {
	// zero or one args is fine.
	err := testing.InitCommand(&environment.GetCommand{}, nil)
	c.Check(err, jc.ErrorIsNil)
	err = testing.InitCommand(&environment.GetCommand{}, []string{"one"})
	c.Check(err, jc.ErrorIsNil)
	// More than one is not allowed.
	err = testing.InitCommand(&environment.GetCommand{}, []string{"one", "two"})
	c.Check(err, gc.ErrorMatches, `unrecognized args: \["two"\]`)
}

func (s *GetSuite) TestSingleValue(c *gc.C) {
	context, err := s.run(c, "name")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	c.Assert(output, gc.Equals, "test-env")
}

func (s *GetSuite) TestSingleValueJSON(c *gc.C) {
	context, err := s.run(c, "--format=json", "name")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	c.Assert(output, gc.Equals, `"test-env"`)
}

func (s *GetSuite) TestAllValues(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	expected := "" +
		"name: test-env\n" +
		"running: true\n" +
		"special: special value"
	c.Assert(output, gc.Equals, expected)
}

func (s *GetSuite) TestAllValuesJSON(c *gc.C) {
	context, err := s.run(c, "--format=json")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	expected := `{"name":"test-env","running":true,"special":"special value"}`
	c.Assert(output, gc.Equals, expected)
}
