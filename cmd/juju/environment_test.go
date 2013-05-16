// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	. "launchpad.net/gocheck"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
	"strings"
)

type GetEnvironmentSuite struct {
	jujutesting.RepoSuite
}

var _ = Suite(&GetEnvironmentSuite{})

var singleValueTests = []struct {
	key    string
	output string
	err    string
}{
	{
		key:    "type",
		output: "dummy",
	}, {
		key:    "name",
		output: "dummyenv",
	}, {
		key:    "authorized-keys",
		output: "i-am-a-key",
	}, {
		key: "unknown",
		err: `Key "unknown" not found in "dummyenv" environment.`,
	},
}

func (s *GetEnvironmentSuite) TestSingleValue(c *C) {

	for _, t := range singleValueTests {
		context, err := testing.RunCommand(c, &GetEnvironmentCommand{}, []string{t.key})
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
		} else {
			output := strings.TrimSpace(testing.Stdout(context))
			c.Assert(err, IsNil)
			c.Assert(output, Equals, t.output)
		}
	}
}

func (s *GetEnvironmentSuite) TestTooManyArgs(c *C) {
	_, err := testing.RunCommand(c, &GetEnvironmentCommand{}, []string{"name", "type"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["type"\]`)
}

func (s *GetEnvironmentSuite) TestAllValues(c *C) {

	context, _ := testing.RunCommand(c, &GetEnvironmentCommand{}, []string{})
	output := strings.TrimSpace(testing.Stdout(context))

	// Make sure that all the environment keys are there.
	any := "(.|\n)*" // because . doesn't match new lines.
	for key := range s.Conn.Environ.Config().AllAttrs() {
		c.Assert(output, Matches, fmt.Sprintf("%s%s: %s", any, key, any))
	}
}

type SetEnvironmentSuite struct {
	jujutesting.RepoSuite
}

var _ = Suite(&SetEnvironmentSuite{})

var setEnvInitTests = []struct {
	args     []string
	expected attributes
	err      string
}{
	{
		args: []string{},
		err:  "No key, value pairs specified",
	}, {
		args: []string{"agent-version=1.2.3"},
		err:  `agent-version must be set via upgrade-juju`,
	}, {
		args: []string{"missing"},
		err:  `Missing "=" in arg 1: "missing"`,
	}, {
		args: []string{"key=value"},
		expected: attributes{
			"key": "value",
		},
	}, {
		args: []string{"key=value", "key=other"},
		err:  `Key "key" specified more than once`,
	}, {
		args: []string{"key=value", "other=embedded=equal"},
		expected: attributes{
			"key":   "value",
			"other": "embedded=equal",
		},
	},
}

func (s *SetEnvironmentSuite) TestInit(c *C) {

	for _, t := range setEnvInitTests {
		command := &SetEnvironmentCommand{}
		testing.TestInit(c, command, t.args, t.err)
		if t.expected != nil {
			c.Assert(command.values, DeepEquals, t.expected)
		}
	}
}

func (s *SetEnvironmentSuite) TestChangeDefaultSeries(c *C) {
	_, err := testing.RunCommand(c, &SetEnvironmentCommand{}, []string{"default-series=raring"})
	c.Assert(err, IsNil)

	stateConfig, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(stateConfig.DefaultSeries(), Equals, "raring")
}

func (s *SetEnvironmentSuite) TestChangeMultipleValues(c *C) {
	_, err := testing.RunCommand(c, &SetEnvironmentCommand{}, []string{"default-series=spartan", "broken=nope", "secret=sekrit"})
	c.Assert(err, IsNil)

	stateConfig, err := s.State.EnvironConfig()
	c.Assert(err, IsNil)
	attrs := stateConfig.AllAttrs()
	c.Assert(attrs["default-series"].(string), Equals, "spartan")
	c.Assert(attrs["broken"].(string), Equals, "nope")
	c.Assert(attrs["secret"].(string), Equals, "sekrit")
}

func (s *SetEnvironmentSuite) TestChangeAsCommandPair(c *C) {
	_, err := testing.RunCommand(c, &SetEnvironmentCommand{}, []string{"default-series=raring"})
	c.Assert(err, IsNil)

	context, err := testing.RunCommand(c, &GetEnvironmentCommand{}, []string{"default-series"})
	c.Assert(err, IsNil)
	output := strings.TrimSpace(testing.Stdout(context))

	c.Assert(output, Equals, "raring")
}

var immutableConfigTests = map[string]string{
	"name":          "foo",
	"type":          "foo",
	"firewall-mode": "global",
}

func (s *SetEnvironmentSuite) TestImmutableConfigValues(c *C) {
	for name, value := range immutableConfigTests {
		param := fmt.Sprintf("%s=%s", name, value)
		_, err := testing.RunCommand(c, &SetEnvironmentCommand{}, []string{param})
		errorPattern := fmt.Sprintf("cannot change %s from .* to %q", name, value)
		c.Assert(err, ErrorMatches, errorPattern)
	}
}
