// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package model_test

import (
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type DefaultsCommandSuite struct {
	fakeModelDefaultEnvSuite
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&DefaultsCommandSuite{})

func (s *DefaultsCommandSuite) SetUpTest(c *gc.C) {
	s.fakeModelDefaultEnvSuite.SetUpTest(c)
	s.store = jujuclienttesting.NewMemStore()
	s.store.CurrentControllerName = "controller"
	s.store.Controllers["controller"] = jujuclient.ControllerDetails{}
}

func (s *DefaultsCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := model.NewDefaultsCommandForTest(s.fake, s.store)
	return testing.RunCommand(c, command, args...)
}

func (s *DefaultsCommandSuite) TestDefaultsInit(c *gc.C) {
	for i, test := range []struct {
		args       []string
		errorMatch string
		nilErr     bool
	}{
		{
			// Test set
			// 0
			args:       []string{"special=extra", "special=other"},
			errorMatch: `key "special" specified more than once`,
		}, {
			// 1
			args:       []string{"agent-version=2.0.0"},
			errorMatch: `"agent-version" must be set via "upgrade-juju"`,
		}, {
			// 2
			args:   []string{"foo=bar", "baz=eggs"},
			nilErr: true,
		}, {
			// Test reset
			// 3
			args:       []string{"--reset"},
			errorMatch: "no keys specified",
		}, {
			// 4
			args:   []string{"--reset", "something", "weird"},
			nilErr: true,
		}, {
			// 5
			args:       []string{"--reset", "agent-version"},
			errorMatch: `"agent-version" cannot be reset`,
		}, {
			// Test get
			// 6
			args:   nil,
			nilErr: true,
		}, {
			// 7
			args:   []string{"one"},
			nilErr: true,
		}, {
			// 8
			args:       []string{"one", "two"},
			errorMatch: "can only retrieve a single value, or all values",
		},
	} {
		c.Logf("test %d", i)
		cmd := model.NewDefaultsCommandForTest(s.fake, s.store)
		err := testing.InitCommand(cmd, test.args)
		if test.nilErr {
			c.Check(err, jc.ErrorIsNil)
			continue
		}
		c.Check(err, gc.ErrorMatches, test.errorMatch)
	}
}

func (s *DefaultsCommandSuite) TestResetUnknownValueLogs(c *gc.C) {
	_, err := s.run(c, "--reset", "attr", "weird")
	c.Assert(err, jc.ErrorIsNil)
	expected := `key "weird" is not defined in the known model configuration: possible misspelling`
	c.Check(c.GetTestLog(), jc.Contains, expected)
}

func (s *DefaultsCommandSuite) TestResetAttr(c *gc.C) {
	_, err := s.run(c, "--reset", "attr", "unknown")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.defaults, jc.DeepEquals, config.ModelDefaultAttributes{
		"attr2": {Controller: "bar", Default: nil, Regions: []config.RegionDefaultValue{{
			Name:  "dummy-region",
			Value: "dummy-value",
		}}},
	})
}

func (s *DefaultsCommandSuite) TestResetBlockedError(c *gc.C) {
	s.fake.err = common.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "--reset", "attr")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	// msg is logged
	c.Check(c.GetTestLog(), jc.Contains, "TestBlockedError")
}

func (s *DefaultsCommandSuite) TestSetUnknownValueLogs(c *gc.C) {
	_, err := s.run(c, "weird=foo")
	c.Assert(err, jc.ErrorIsNil)
	expected := `key "weird" is not defined in the known model configuration: possible misspelling`
	c.Check(c.GetTestLog(), jc.Contains, expected)
}

func (s *DefaultsCommandSuite) TestSet(c *gc.C) {
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

func (s *DefaultsCommandSuite) TestBlockedErrorOnSet(c *gc.C) {
	s.fake.err = common.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "special=extra")
	c.Assert(err, gc.Equals, cmd.ErrSilent)
	// msg is logged
	c.Check(c.GetTestLog(), jc.Contains, "TestBlockedError")
}

func (s *DefaultsCommandSuite) TestGetSingleValue(c *gc.C) {
	context, err := s.run(c, "attr2")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	expected := "" +
		"ATTRIBUTE       DEFAULT      CONTROLLER\n" +
		"attr2           -            bar\n" +
		"  dummy-region  dummy-value  -"
	c.Assert(output, gc.Equals, expected)
}

func (s *DefaultsCommandSuite) TestGetSingleValueJSON(c *gc.C) {
	context, err := s.run(c, "--format=json", "attr2")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	c.Assert(output, gc.Equals,
		`{"attr2":{"controller":"bar","regions":[{"name":"dummy-region","value":"dummy-value"}]}}`)
}

func (s *DefaultsCommandSuite) TestGetAllValuesYAML(c *gc.C) {
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

func (s *DefaultsCommandSuite) TestGetAllValuesJSON(c *gc.C) {
	context, err := s.run(c, "--format=json")
	c.Assert(err, jc.ErrorIsNil)

	output := strings.TrimSpace(testing.Stdout(context))
	expected := `{"attr":{"default":"foo"},"attr2":{"controller":"bar","regions":[{"name":"dummy-region","value":"dummy-value"}]}}`
	c.Assert(output, gc.Equals, expected)
}

func (s *DefaultsCommandSuite) TestGetAllValuesTabular(c *gc.C) {
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
