// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statecmd_test

import (
	"fmt"
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
	coretesting "launchpad.net/juju-core/testing"
	stdtesting "testing"
)

type ConfigSuite struct {
	testing.JujuConnSuite
}

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

var _ = gc.Suite(&ConfigSuite{})

var getTests = []struct {
	about       string
	charm       string
	constraints string
	config      map[string]string
	expect      params.ServiceGetResults
}{{
	about:       "deployed service",
	charm:       "dummy",
	constraints: "mem=2G cpu-power=400",
	config: map[string]string{
		// Different from default.
		"title": "Look To Windward",
		// Same as default.
		"username": "admin001",
		// Use default (but there's no charm default)
		"skill-level": "",
		// Outlook is left unset.
	},
	expect: params.ServiceGetResults{
		Config: map[string]interface{}{
			"title": map[string]interface{}{
				"description": "A descriptive title used for the service.",
				"type":        "string",
				"value":       "Look To Windward",
			},
			"outlook": map[string]interface{}{
				"description": "No default outlook.",
				"type":        "string",
				"default":     true,
				"value":       nil,
			},
			"username": map[string]interface{}{
				"description": "The name of the initial account (given admin permissions).",
				"type":        "string",
				"value":       "admin001",
			},
			"skill-level": map[string]interface{}{
				"description": "A number indicating skill.",
				"type":        "int",
				"default":     true,
				"value":       nil,
			},
		},
	},
}, {
	about: "deployed service  #2",
	charm: "dummy",
	config: map[string]string{
		// Empty string gives default
		"title": "",
		// Value when there's a default
		"username": "foobie",
		// Numeric value
		"skill-level": "0",
		// String value
		"outlook": "phlegmatic",
	},
	expect: params.ServiceGetResults{
		Config: map[string]interface{}{
			"title": map[string]interface{}{
				"description": "A descriptive title used for the service.",
				"type":        "string",
				"value":       "My Title",
				"default":     true,
			},
			"outlook": map[string]interface{}{
				"description": "No default outlook.",
				"type":        "string",
				"value":       "phlegmatic",
			},
			"username": map[string]interface{}{
				"description": "The name of the initial account (given admin permissions).",
				"type":        "string",
				"value":       "foobie",
			},
			"skill-level": map[string]interface{}{
				"description": "A number indicating skill.",
				"type":        "int",
				"value":       int64(0),
			},
		},
	},
}, {
	about: "subordinate service",
	charm: "logging",
	expect: params.ServiceGetResults{
		Config: map[string]interface{}{},
	},
}}

func (s *ConfigSuite) TestServiceGetUnknownService(c *gc.C) {
	_, err := statecmd.ServiceGet(s.State, params.ServiceGet{ServiceName: "unknown"})
	c.Assert(err, gc.ErrorMatches, `service "unknown" not found`)
}

func (s *ConfigSuite) TestServiceGet(c *gc.C) {
	for i, t := range getTests {
		c.Logf("test %d. %s", i, t.about)
		ch := s.AddTestingCharm(c, t.charm)
		svc, err := s.State.AddService(fmt.Sprintf("test%d", i), ch)
		c.Assert(err, gc.IsNil)

		var constraintsv constraints.Value
		if t.constraints != "" {
			constraintsv = constraints.MustParse(t.constraints)
			err = svc.SetConstraints(constraintsv)
			c.Assert(err, gc.IsNil)
		}
		if t.config != nil {
			settings, err := ch.Config().ParseSettingsStrings(t.config)
			c.Assert(err, gc.IsNil)
			err = svc.UpdateConfigSettings(settings)
			c.Assert(err, gc.IsNil)
		}
		expect := t.expect
		expect.Constraints = constraintsv
		expect.Service = svc.Name()
		expect.Charm = ch.Meta().Name
		got, err := statecmd.ServiceGet(s.State, params.ServiceGet{svc.Name()})
		c.Assert(err, gc.IsNil)
		c.Assert(got, gc.DeepEquals, expect)
	}
}
