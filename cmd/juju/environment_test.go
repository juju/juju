// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
)

type GetEnvironmentSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&GetEnvironmentSuite{})

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
		output: dummy.SampleConfig()["authorized-keys"].(string),
	}, {
		key: "unknown",
		err: `Key "unknown" not found in "dummyenv" environment.`,
	},
}

func (s *GetEnvironmentSuite) TestSingleValue(c *gc.C) {
	for _, t := range singleValueTests {
		context, err := testing.RunCommand(c, &GetEnvironmentCommand{}, []string{t.key})
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			output := strings.TrimSpace(testing.Stdout(context))
			c.Assert(err, gc.IsNil)
			c.Assert(output, gc.Equals, t.output)
		}
	}
}

func (s *GetEnvironmentSuite) TestTooManyArgs(c *gc.C) {
	_, err := testing.RunCommand(c, &GetEnvironmentCommand{}, []string{"name", "type"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["type"\]`)
}

func (s *GetEnvironmentSuite) TestAllValues(c *gc.C) {
	context, _ := testing.RunCommand(c, &GetEnvironmentCommand{}, []string{})
	output := strings.TrimSpace(testing.Stdout(context))

	// Make sure that all the environment keys are there. The admin
	// secret and CA private key are never pushed into the
	// environment.
	for key := range s.Conn.Environ.Config().AllAttrs() {
		c.Logf("test for key %q", key)
		any := `(.|\n)*`
		pattern := fmt.Sprintf(`(?m)^%s:`, key)
		c.Check(output, gc.Matches, any+pattern+any)
	}
}

type SetEnvironmentSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&SetEnvironmentSuite{})

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

func (s *SetEnvironmentSuite) TestInit(c *gc.C) {
	for _, t := range setEnvInitTests {
		command := &SetEnvironmentCommand{}
		testing.TestInit(c, command, t.args, t.err)
		if t.expected != nil {
			c.Assert(command.values, gc.DeepEquals, t.expected)
		}
	}
}

func (s *SetEnvironmentSuite) TestChangeDefaultSeries(c *gc.C) {
	_, err := testing.RunCommand(c, &SetEnvironmentCommand{}, []string{"default-series=raring"})
	c.Assert(err, gc.IsNil)

	stateConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(stateConfig.DefaultSeries(), gc.Equals, "raring")
}

func (s *SetEnvironmentSuite) TestChangeBooleanAttribute(c *gc.C) {
	_, err := testing.RunCommand(c, &SetEnvironmentCommand{}, []string{"ssl-hostname-verification=false"})
	c.Assert(err, gc.IsNil)

	stateConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(stateConfig.SSLHostnameVerification(), gc.Equals, false)
}

func (s *SetEnvironmentSuite) TestChangeMultipleValues(c *gc.C) {
	_, err := testing.RunCommand(c, &SetEnvironmentCommand{}, []string{"default-series=spartan", "broken=nope", "secret=sekrit"})
	c.Assert(err, gc.IsNil)

	stateConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	attrs := stateConfig.AllAttrs()
	c.Assert(attrs["default-series"].(string), gc.Equals, "spartan")
	c.Assert(attrs["broken"].(string), gc.Equals, "nope")
	c.Assert(attrs["secret"].(string), gc.Equals, "sekrit")
}

func (s *SetEnvironmentSuite) TestChangeAsCommandPair(c *gc.C) {
	_, err := testing.RunCommand(c, &SetEnvironmentCommand{}, []string{"default-series=raring"})
	c.Assert(err, gc.IsNil)

	context, err := testing.RunCommand(c, &GetEnvironmentCommand{}, []string{"default-series"})
	c.Assert(err, gc.IsNil)
	output := strings.TrimSpace(testing.Stdout(context))

	c.Assert(output, gc.Equals, "raring")
}

var immutableConfigTests = map[string]string{
	"name":          "foo",
	"type":          "foo",
	"firewall-mode": "global",
	"state-port":    "1",
	"api-port":      "666",
	"syslog-port":   "42",
}

func (s *SetEnvironmentSuite) TestImmutableConfigValues(c *gc.C) {
	for name, value := range immutableConfigTests {
		param := fmt.Sprintf("%s=%s", name, value)
		_, err := testing.RunCommand(c, &SetEnvironmentCommand{}, []string{param})
		errorPattern := fmt.Sprintf("cannot change %s from .* to [\"]?%v[\"]?", name, value)
		c.Assert(err, gc.ErrorMatches, errorPattern)
	}
}
