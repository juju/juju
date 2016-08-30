// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type UnsetDefaultsSuite struct {
	fakeModelDefaultEnvSuite
}

var _ = gc.Suite(&UnsetDefaultsSuite{})

func (s *UnsetDefaultsSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := model.NewUnsetDefaultsCommandForTest(s.fake)
	return testing.RunCommand(c, command, args...)
}

func (s *UnsetDefaultsSuite) TestInitArgCount(c *gc.C) {
	unsetCmd := model.NewUnsetDefaultsCommandForTest(s.fake)
	// Only empty is a problem.
	err := testing.InitCommand(unsetCmd, []string{})
	c.Assert(err, gc.ErrorMatches, "no keys specified")
	// Everything else is fine.
	err = testing.InitCommand(unsetCmd, []string{"something", "weird"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnsetDefaultsSuite) TestInitUnknownValue(c *gc.C) {
	unsetCmd := model.NewUnsetDefaultsCommandForTest(s.fake)
	err := testing.InitCommand(unsetCmd, []string{"attr", "weird"})
	c.Assert(err, jc.ErrorIsNil)
	expected := `key "weird" is not defined in the known model configuration: possible misspelling`
	c.Check(c.GetTestLog(), jc.Contains, expected)
}

func (s *UnsetDefaultsSuite) TestUnset(c *gc.C) {
	_, err := s.run(c, "attr", "unknown")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.defaults, jc.DeepEquals, config.ModelDefaultAttributes{
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}}},
	})
}

func (s *UnsetDefaultsSuite) TestBlockedError(c *gc.C) {
	s.fake.err = common.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "attr")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	// msg is logged
	c.Check(c.GetTestLog(), jc.Contains, "TestBlockedError")
}
