// Copyright 2016 Canonical Ltd.
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

type getDefaultsSuite struct {
	fakeModelDefaultEnvSuite
}

var _ = gc.Suite(&getDefaultsSuite{})

func (s *getDefaultsSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := model.NewGetDefaultsCommandForTest(s.fake)
	return testing.RunCommand(c, command, args...)
}

func (s *getDefaultsSuite) TestInitArgCount(c *gc.C) {
	// zero or one args is fine.
	err := testing.InitCommand(model.NewGetDefaultsCommandForTest(s.fake), nil)
	c.Check(err, jc.ErrorIsNil)
	err = testing.InitCommand(model.NewGetDefaultsCommandForTest(s.fake), []string{"one"})
	c.Check(err, jc.ErrorIsNil)
	// More than one is not allowed.
	err = testing.InitCommand(model.NewGetDefaultsCommandForTest(s.fake), []string{"one", "two"})
	c.Check(err, gc.ErrorMatches, `unrecognized args: \["two"\]`)
}

func (s *getDefaultsSuite) TestSingleValue(c *gc.C) {
	context, err := s.run(c, "attr")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	expected := "" +
		"ATTRIBUTE  DEFAULT  CONTROLLER\n" +
		"attr       foo      -"
	c.Assert(output, gc.Equals, expected)
}

func (s *getDefaultsSuite) TestSingleValueJSON(c *gc.C) {
	context, err := s.run(c, "--format=json", "attr")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	c.Assert(output, gc.Equals, `{"attr":{"default":"foo"}}`)
}

func (s *getDefaultsSuite) TestAllValuesYAML(c *gc.C) {
	context, err := s.run(c, "--format=yaml")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	expected := "" +
		"attr:\n" +
		"  default: foo\n" +
		"attr2:\n" +
		"  controller: bar\n" +
		"  regions:\n" +
		"  - name: dummy-region\n" +
		"    value: dummy-value"
	c.Assert(output, gc.Equals, expected)
}

func (s *getDefaultsSuite) TestAllValuesJSON(c *gc.C) {
	context, err := s.run(c, "--format=json")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	expected := `{"attr":{"default":"foo"},"attr2":{"controller":"bar","regions":[{"name":"dummy-region","value":"dummy-value"}]}}`
	c.Assert(output, gc.Equals, expected)
}

func (s *getDefaultsSuite) TestAllValuesTabular(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	expected := "" +
		"ATTRIBUTE       DEFAULT      CONTROLLER\n" +
		"attr            foo          -\n" +
		"attr2           -            bar\n" +
		"  dummy-region  dummy-value  -"
	c.Assert(output, gc.Equals, expected)
}
