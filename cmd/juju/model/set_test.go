// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/testing"
)

type SetSuite struct {
	fakeEnvSuite
}

var _ = gc.Suite(&SetSuite{})

func (s *SetSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := model.NewSetCommandForTest(s.fake)
	return testing.RunCommand(c, command, args...)
}

func (s *SetSuite) TestInit(c *gc.C) {
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

func (s *SetSuite) TestPassesValues(c *gc.C) {
	_, err := s.run(c, "special=extra", "unknown=foo")
	c.Assert(err, jc.ErrorIsNil)
	expected := map[string]interface{}{
		"special": "extra",
		"unknown": "foo",
	}
	c.Assert(s.fake.values, jc.DeepEquals, expected)
}

func (s *SetSuite) TestSettingKnownValue(c *gc.C) {
	_, err := s.run(c, "special=extra", "unknown=foo")
	c.Assert(err, jc.ErrorIsNil)
	// Command succeeds, but warning logged.
	expected := `key "unknown" is not defined in the current model configuration: possible misspelling`
	c.Check(c.GetTestLog(), jc.Contains, expected)
}

func (s *SetSuite) TestBlockedError(c *gc.C) {
	s.fake.err = common.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "special=extra")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	// msg is logged
	c.Check(c.GetTestLog(), jc.Contains, "TestBlockedError")
}
