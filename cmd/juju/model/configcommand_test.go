// Copyright 2016 Canonical Ltd.
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

type ConfigCommandSuite struct {
	fakeEnvSuite
}

var _ = gc.Suite(&ConfigCommandSuite{})

func (s *ConfigCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := model.NewConfigCommandForTest(s.fake)
	return testing.RunCommand(c, command, args...)
}

func (s *ConfigCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args       []string
		errorMatch string
		nilErr     bool
	}{
		{ // Test set
			// 0
			args:       []string{"special=extra", "special=other"},
			errorMatch: `key "special" specified more than once`,
		}, {
			// 1
			args:       []string{"agent-version=2.0.0"},
			errorMatch: `agent-version must be set via "upgrade-juju"`,
		}, {
			// Test reset
			// 2
			args:       []string{"--reset"},
			errorMatch: "no keys specified",
		}, {
			// 3
			args:   []string{"--reset", "something", "weird"},
			nilErr: true,
		}, {
			// 4
			args:       []string{"--reset", "agent-version"},
			errorMatch: "agent-version cannot be reset",
		}, {
			// Test get
			// 5
			args:   nil,
			nilErr: true,
		}, {
			// 6
			args:   []string{"one"},
			nilErr: true,
		}, {
			// 7
			args:       []string{"one", "two"},
			errorMatch: "can only retrieve a single value, or all values",
		},
	} {
		c.Logf("test %d", i)
		cmd := model.NewConfigCommandForTest(s.fake)
		err := testing.InitCommand(cmd, test.args)
		if test.nilErr {
			c.Check(err, jc.ErrorIsNil)
			continue
		}
		c.Check(err, gc.ErrorMatches, test.errorMatch)
	}
}

func (s *ConfigCommandSuite) TestSingleValue(c *gc.C) {
	context, err := s.run(c, "special")
	c.Assert(err, jc.ErrorIsNil)

	output := testing.Stdout(context)
	c.Assert(output, gc.Equals, "special value\n")
}

func (s *ConfigCommandSuite) TestSingleValueJSON(c *gc.C) {
	context, err := s.run(c, "--format=json", "special")
	c.Assert(err, jc.ErrorIsNil)

	want := "{\"special\":{\"Value\":\"special value\",\"Source\":\"model\"}}\n"
	output := testing.Stdout(context)
	c.Assert(output, gc.Equals, want)
}

func (s *ConfigCommandSuite) TestSingleValueYAML(c *gc.C) {
	context, err := s.run(c, "--format=yaml", "special")
	c.Assert(err, jc.ErrorIsNil)

	want := "" +
		"special:\n" +
		"  value: special value\n" +
		"  source: model\n"

	output := testing.Stdout(context)
	c.Assert(output, gc.Equals, want)
}

func (s *ConfigCommandSuite) TestAllValuesYAML(c *gc.C) {
	context, err := s.run(c, "--format=yaml")
	c.Assert(err, jc.ErrorIsNil)

	output := testing.Stdout(context)
	expected := "" +
		"running:\n" +
		"  value: true\n" +
		"  source: model\n" +
		"special:\n" +
		"  value: special value\n" +
		"  source: model\n"
	c.Assert(output, gc.Equals, expected)
}

func (s *ConfigCommandSuite) TestAllValuesJSON(c *gc.C) {
	context, err := s.run(c, "--format=json")
	c.Assert(err, jc.ErrorIsNil)

	output := testing.Stdout(context)
	expected := `{"running":{"Value":true,"Source":"model"},"special":{"Value":"special value","Source":"model"}}` + "\n"
	c.Assert(output, gc.Equals, expected)
}

func (s *ConfigCommandSuite) TestAllValuesTabular(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)

	output := testing.Stdout(context)
	expected := "" +
		"ATTRIBUTE  FROM   VALUE\n" +
		"running    model  true\n" +
		"special    model  special value\n" +
		"\n"
	c.Assert(output, gc.Equals, expected)
}

func (s *ConfigCommandSuite) TestPassesValues(c *gc.C) {
	_, err := s.run(c, "special=extra", "unknown=foo")
	c.Assert(err, jc.ErrorIsNil)
	expected := map[string]interface{}{
		"special": "extra",
		"unknown": "foo",
	}
	c.Assert(s.fake.values, jc.DeepEquals, expected)
}

func (s *ConfigCommandSuite) TestSettingKnownValue(c *gc.C) {
	_, err := s.run(c, "special=extra", "unknown=foo")
	c.Assert(err, jc.ErrorIsNil)
	// Command succeeds, but warning logged.
	expected := `key "unknown" is not defined in the current model configuration: possible misspelling`
	c.Check(c.GetTestLog(), jc.Contains, expected)
}

func (s *ConfigCommandSuite) TestBlockedError(c *gc.C) {
	s.fake.err = common.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "special=extra")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	// msg is logged
	c.Check(c.GetTestLog(), jc.Contains, "TestBlockedError")
}

func (s *ConfigCommandSuite) TestResetPassesValues(c *gc.C) {
	_, err := s.run(c, "--reset", "special", "running")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.keys, jc.DeepEquals, []string{"special", "running"})
}

func (s *ConfigCommandSuite) TestResettingKnownValue(c *gc.C) {
	_, err := s.run(c, "--reset", "unknown")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.keys, jc.DeepEquals, []string{"unknown"})
	// Command succeeds, but warning logged.
	expected := `key "unknown" is not defined in the current model configuration: possible misspelling`
	c.Check(c.GetTestLog(), jc.Contains, expected)
}

func (s *ConfigCommandSuite) TestResetBlockedError(c *gc.C) {
	s.fake.err = common.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "--reset", "special")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	// msg is logged
	c.Check(c.GetTestLog(), jc.Contains, "TestBlockedError")
}
