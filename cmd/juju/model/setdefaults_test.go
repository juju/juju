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

type SetDefaultsSuite struct {
	fakeModelDefaultEnvSuite
}

var _ = gc.Suite(&SetDefaultsSuite{})

func (s *SetDefaultsSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := model.NewSetDefaultsCommandForTest(s.fake)
	return testing.RunCommand(c, command, args...)
}

func (s *SetDefaultsSuite) TestInitKeyArgs(c *gc.C) {
	for i, test := range []struct {
		args       []string
		errorMatch string
	}{
		{
			errorMatch: "no key, value pairs specified",
		}, {
			args:       []string{"special"},
			errorMatch: `expected "key=value", got "special"`,
		}, {
			args:       []string{"special=extra", "special=other"},
			errorMatch: `key "special" specified more than once`,
		}, {
			args:       []string{"agent-version=2.0.0"},
			errorMatch: "agent-version must be set via upgrade-juju",
		},
	} {
		c.Logf("test %d", i)
		setCmd := model.NewSetCommandForTest(s.fake)
		err := testing.InitCommand(setCmd, test.args)
		c.Check(err, gc.ErrorMatches, test.errorMatch)
	}
}

func (s *SetDefaultsSuite) TestInitUnknownValue(c *gc.C) {
	unsetCmd := model.NewUnsetDefaultsCommandForTest(s.fake)
	err := testing.InitCommand(unsetCmd, []string{"attr", "weird"})
	c.Assert(err, jc.ErrorIsNil)
	expected := `key "weird" is not defined in the known model configuration: possible misspelling`
	c.Check(c.GetTestLog(), jc.Contains, expected)
}

func (s *SetDefaultsSuite) TestSet(c *gc.C) {
	_, err := s.run(c, "special=extra", "attr=baz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.defaults, jc.DeepEquals, config.ModelDefaultAttributes{
		"attr": {Controller: "baz", Default: nil, Regions: nil},
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}}},
		"special": {Controller: "extra", Default: nil, Regions: nil},
	})
}

func (s *SetDefaultsSuite) TestBlockedError(c *gc.C) {
	s.fake.err = common.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "special=extra")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	// msg is logged
	c.Check(c.GetTestLog(), jc.Contains, "TestBlockedError")
}
