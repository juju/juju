// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/local"
	"github.com/juju/juju/testing"
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
		err: `key "unknown" not found in "dummyenv" environment.`,
	},
}

func (s *GetEnvironmentSuite) TestSingleValue(c *gc.C) {
	for _, t := range singleValueTests {
		context, err := testing.RunCommand(c, envcmd.Wrap(&GetEnvironmentCommand{}), t.key)
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
	_, err := testing.RunCommand(c, envcmd.Wrap(&GetEnvironmentCommand{}), "name", "type")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["type"\]`)
}

func (s *GetEnvironmentSuite) TestAllValues(c *gc.C) {
	context, _ := testing.RunCommand(c, envcmd.Wrap(&GetEnvironmentCommand{}))
	output := strings.TrimSpace(testing.Stdout(context))

	// Make sure that all the environment keys are there. The admin
	// secret and CA private key are never pushed into the
	// environment.
	for key := range s.Environ.Config().AllAttrs() {
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
		err:  "no key, value pairs specified",
	}, {
		args: []string{"agent-version=1.2.3"},
		err:  `agent-version must be set via upgrade-juju`,
	}, {
		args: []string{"missing"},
		err:  `expected "key=value", got "missing"`,
	}, {
		args: []string{"key=value"},
		expected: attributes{
			"key": "value",
		},
	}, {
		args: []string{"key=value", "key=other"},
		err:  `key "key" specified more than once`,
	}, {
		args: []string{"key=value", "other=embedded=equal"},
		expected: attributes{
			"key":   "value",
			"other": "embedded=equal",
		},
	},
}

var setUnknownKeyTests = []struct {
	args   []string
	output string
}{
	{
		args:   []string{"authoXized-keys=abc"},
		output: `WARNING juju.cmd.juju key "authoXized-keys" is not defined in the current environemnt configuration: possible misspelling`,
	},
	{
		args:   []string{"states-port=123"},
		output: `WARNING juju.cmd.juju key "states-port" is not defined in the current environemnt configuration: possible misspelling`,
	},
	{
		args:   []string{"loggging-config=<root>=INFO;juju.provider=DEBUG"},
		output: `WARNING juju.cmd.juju key "loggging-config" is not defined in the current environemnt configuration: possible misspelling`,
	},
}

func (s *SetEnvironmentSuite) TestInit(c *gc.C) {
	for _, t := range setEnvInitTests {
		command := &SetEnvironmentCommand{}
		testing.TestInit(c, envcmd.Wrap(command), t.args, t.err)
		if t.expected != nil {
			c.Assert(command.values, gc.DeepEquals, t.expected)
		}
	}
}

func (s *SetEnvironmentSuite) TestSetUnknownKey(c *gc.C) {
	for _, t := range setUnknownKeyTests {
		ctx := testing.ContextForDir(c, s.DataDir())
		code := cmd.Main(envcmd.Wrap(&SetEnvironmentCommand{}), ctx, t.args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(c.GetTestLog(), jc.Contains, t.output)
	}
}

func (s *SetEnvironmentSuite) TestChangeDefaultSeries(c *gc.C) {
	// default-series not set
	stateConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	series, ok := stateConfig.DefaultSeries()
	c.Assert(ok, gc.Equals, true)
	c.Assert(series, gc.Equals, config.LatestLtsSeries()) // default-series set in RepoSuite.SetUpTest

	_, err = testing.RunCommand(c, envcmd.Wrap(&SetEnvironmentCommand{}), "default-series=raring")
	c.Assert(err, gc.IsNil)

	stateConfig, err = s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	series, ok = stateConfig.DefaultSeries()
	c.Assert(ok, gc.Equals, true)
	c.Assert(series, gc.Equals, "raring")
	c.Assert(config.PreferredSeries(stateConfig), gc.Equals, "raring")
}

func (s *SetEnvironmentSuite) TestChangeBooleanAttribute(c *gc.C) {
	_, err := testing.RunCommand(c, envcmd.Wrap(&SetEnvironmentCommand{}), "ssl-hostname-verification=false")
	c.Assert(err, gc.IsNil)

	stateConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(stateConfig.SSLHostnameVerification(), gc.Equals, false)
}

func (s *SetEnvironmentSuite) TestChangeMultipleValues(c *gc.C) {
	_, err := testing.RunCommand(c, envcmd.Wrap(&SetEnvironmentCommand{}), "default-series=spartan", "broken=nope", "secret=sekrit")
	c.Assert(err, gc.IsNil)

	stateConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	attrs := stateConfig.AllAttrs()
	c.Assert(attrs["default-series"].(string), gc.Equals, "spartan")
	c.Assert(attrs["broken"].(string), gc.Equals, "nope")
	c.Assert(attrs["secret"].(string), gc.Equals, "sekrit")
}

func (s *SetEnvironmentSuite) TestChangeAsCommandPair(c *gc.C) {
	_, err := testing.RunCommand(c, envcmd.Wrap(&SetEnvironmentCommand{}), "default-series=raring")
	c.Assert(err, gc.IsNil)

	context, err := testing.RunCommand(c, envcmd.Wrap(&GetEnvironmentCommand{}), "default-series")
	c.Assert(err, gc.IsNil)
	output := strings.TrimSpace(testing.Stdout(context))

	c.Assert(output, gc.Equals, "raring")
}

var immutableConfigTests = map[string]string{
	"name":          "foo",
	"type":          "local",
	"firewall-mode": config.FwGlobal,
	"state-port":    "1",
	"api-port":      "666",
}

func (s *SetEnvironmentSuite) TestImmutableConfigValues(c *gc.C) {
	for name, value := range immutableConfigTests {
		param := fmt.Sprintf("%s=%s", name, value)
		_, err := testing.RunCommand(c, envcmd.Wrap(&SetEnvironmentCommand{}), param)
		errorPattern := fmt.Sprintf("cannot change %s from .* to [\"]?%v[\"]?", name, value)
		c.Assert(err, gc.ErrorMatches, errorPattern)
	}
}

type UnsetEnvironmentSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&UnsetEnvironmentSuite{})

var unsetEnvTests = []struct {
	args       []string
	err        string
	expected   attributes
	unexpected []string
}{
	{
		args: []string{},
		err:  "no keys specified",
	}, {
		args:       []string{"xyz", "xyz"},
		unexpected: []string{"xyz"},
	}, {
		args: []string{"type", "xyz"},
		err:  "type: expected string, got nothing",
		expected: attributes{
			"type": "dummy",
			"xyz":  123,
		},
	}, {
		args:       []string{"xyz2", "xyz"},
		unexpected: []string{"xyz"},
	},
}

var unsetUnknownKeyTests = []struct {
	args   []string
	output string
}{
	{
		args:   []string{"authorixed-keys"},
		output: `WARNING juju.cmd.juju key "authorixed-keys" is not defined in the current environemnt configuration: possible misspelling`,
	},
	{
		args:   []string{"statez-port"},
		output: `WARNING juju.cmd.juju key "statez-port" is not defined in the current environemnt configuration: possible misspelling`,
	},
	{
		args:   []string{"loggin-config"},
		output: `WARNING juju.cmd.juju key "loggin-config" is not defined in the current environemnt configuration: possible misspelling`,
	},
}

func (s *UnsetEnvironmentSuite) TestUnsetUnknownKey(c *gc.C) {
	for _, t := range unsetUnknownKeyTests {
		ctx := testing.ContextForDir(c, s.DataDir())
		code := cmd.Main(envcmd.Wrap(&UnsetEnvironmentCommand{}), ctx, t.args)
		c.Assert(code, gc.Equals, 0)
		c.Assert(c.GetTestLog(), jc.Contains, t.output)
	}
}

func (s *UnsetEnvironmentSuite) initConfig(c *gc.C) {
	err := s.State.UpdateEnvironConfig(map[string]interface{}{
		"xyz": 123,
	}, nil, nil)
	c.Assert(err, gc.IsNil)
}

func (s *UnsetEnvironmentSuite) TestUnsetEnvironment(c *gc.C) {
	for _, t := range unsetEnvTests {
		c.Logf("testing unset-env %v", t.args)
		s.initConfig(c)
		_, err := testing.RunCommand(c, envcmd.Wrap(&UnsetEnvironmentCommand{}), t.args...)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, gc.IsNil)
		}
		if len(t.expected)+len(t.unexpected) != 0 {
			stateConfig, err := s.State.EnvironConfig()
			c.Assert(err, gc.IsNil)
			for k, v := range t.expected {
				vstate, ok := stateConfig.AllAttrs()[k]
				c.Assert(ok, jc.IsTrue)
				c.Assert(vstate, gc.Equals, v)
			}
			for _, k := range t.unexpected {
				_, ok := stateConfig.AllAttrs()[k]
				c.Assert(ok, jc.IsFalse)
			}
		}
	}
}
