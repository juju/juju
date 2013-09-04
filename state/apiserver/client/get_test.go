// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/state/api/params"
)

type getSuite struct {
	baseSuite
}

var _ = gc.Suite(&getSuite{})

func (s *getSuite) TestClientServiceGetSmoketest(c *gc.C) {
	s.setUpScenario(c)
	results, err := s.APIState.Client().ServiceGet("wordpress")
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, &params.ServiceGetResults{
		Service: "wordpress",
		Charm:   "wordpress",
		Config: map[string]interface{}{
			"blog-title": map[string]interface{}{
				"type":        "string",
				"value":       "My Title",
				"description": "A descriptive title used for the blog.",
				"default":     true,
			},
		},
	})
}

func (s *getSuite) TestServiceGetUnknownService(c *gc.C) {
	apiclient := s.APIState.Client()
	_, err := apiclient.ServiceGet("unknown")
	c.Assert(err, gc.ErrorMatches, `service "unknown" not found`)
}

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
				// TODO(jam): 2013-08-28 bug #1217742
				// we have to use float64() here, because the
				// API does not preserve int types. This used
				// to be int64() but we end up with a type
				// mismatch when comparing the content
				"value": float64(0),
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

func (s *getSuite) TestServiceGet(c *gc.C) {
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
		apiclient := s.APIState.Client()
		got, err := apiclient.ServiceGet(svc.Name())
		c.Assert(err, gc.IsNil)
		c.Assert(*got, gc.DeepEquals, expect)
	}
}

func (s *getSuite) TestServiceGetMaxResolutionInt(c *gc.C) {
	// See the bug http://pad.lv/1217742
	// ServiceGet ends up pushing a map[string]interface{} which containts
	// an int64 through a JSON Marshal & Unmarshal which ends up changing
	// the int64 into a float64. We will fix it if we find it is actually a
	// problem.
	const nonFloatInt = (int64(1) << 54) + 1
	const asFloat = float64(nonFloatInt)
	c.Assert(int64(asFloat), gc.Not(gc.Equals), nonFloatInt)
	c.Assert(int64(asFloat)+1, gc.Equals, nonFloatInt)

	ch := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("test-service", ch)
	c.Assert(err, gc.IsNil)

	err = svc.UpdateConfigSettings(map[string]interface{}{"skill-level": nonFloatInt})
	c.Assert(err, gc.IsNil)
	got, err := s.APIState.Client().ServiceGet(svc.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(got.Config["skill-level"], gc.DeepEquals, map[string]interface{}{
		"description": "A number indicating skill.",
		"type":        "int",
		"value":       asFloat,
	})
}
