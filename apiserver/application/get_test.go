// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	apiapplication "github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/application"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/constraints"
	jujutesting "github.com/juju/juju/juju/testing"
)

type getSuite struct {
	jujutesting.JujuConnSuite

	serviceAPI *application.API
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&getSuite{})

func (s *getSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.serviceAPI, err = application.NewAPI(s.State, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *getSuite) TestClientServiceGetSmoketest(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	results, err := s.serviceAPI.Get(params.ApplicationGet{"wordpress"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ApplicationGetResults{
		Application: "wordpress",
		Charm:       "wordpress",
		Config: map[string]interface{}{
			"blog-title": map[string]interface{}{
				"type":        "string",
				"value":       "My Title",
				"description": "A descriptive title used for the blog.",
				"default":     true,
			},
		},
		Series: "quantal",
	})
}

func (s *getSuite) TestServiceGetUnknownService(c *gc.C) {
	_, err := s.serviceAPI.Get(params.ApplicationGet{"unknown"})
	c.Assert(err, gc.ErrorMatches, `application "unknown" not found`)
}

var getTests = []struct {
	about       string
	charm       string
	constraints string
	config      charm.Settings
	expect      params.ApplicationGetResults
}{{
	about:       "deployed service",
	charm:       "dummy",
	constraints: "mem=2G cpu-power=400",
	config: charm.Settings{
		// Different from default.
		"title": "Look To Windward",
		// Same as default.
		"username": "admin001",
		// Use default (but there's no charm default)
		"skill-level": nil,
		// Outlook is left unset.
	},
	expect: params.ApplicationGetResults{
		Config: map[string]interface{}{
			"title": map[string]interface{}{
				"description": "A descriptive title used for the application.",
				"type":        "string",
				"value":       "Look To Windward",
			},
			"outlook": map[string]interface{}{
				"description": "No default outlook.",
				"type":        "string",
				"default":     true,
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
			},
		},
		Series: "quantal",
	},
}, {
	about: "deployed service  #2",
	charm: "dummy",
	config: charm.Settings{
		// Set title to default.
		"title": nil,
		// Value when there's a default.
		"username": "foobie",
		// Numeric value.
		"skill-level": 0,
		// String value.
		"outlook": "phlegmatic",
	},
	expect: params.ApplicationGetResults{
		Config: map[string]interface{}{
			"title": map[string]interface{}{
				"description": "A descriptive title used for the application.",
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
		Series: "quantal",
	},
}, {
	about: "subordinate service",
	charm: "logging",
	expect: params.ApplicationGetResults{
		Config: map[string]interface{}{},
		Series: "quantal",
	},
}}

func (s *getSuite) TestServiceGet(c *gc.C) {
	for i, t := range getTests {
		c.Logf("test %d. %s", i, t.about)
		ch := s.AddTestingCharm(c, t.charm)
		svc := s.AddTestingService(c, fmt.Sprintf("test%d", i), ch)

		var constraintsv constraints.Value
		if t.constraints != "" {
			constraintsv = constraints.MustParse(t.constraints)
			err := svc.SetConstraints(constraintsv)
			c.Assert(err, jc.ErrorIsNil)
		}
		if t.config != nil {
			err := svc.UpdateConfigSettings(t.config)
			c.Assert(err, jc.ErrorIsNil)
		}
		expect := t.expect
		expect.Constraints = constraintsv
		expect.Application = svc.Name()
		expect.Charm = ch.Meta().Name
		client := apiapplication.NewClient(s.APIState)
		got, err := client.Get(svc.Name())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(*got, gc.DeepEquals, expect)
	}
}

func (s *getSuite) TestGetMaxResolutionInt(c *gc.C) {
	// See the bug http://pad.lv/1217742
	// Get ends up pushing a map[string]interface{} which containts
	// an int64 through a JSON Marshal & Unmarshal which ends up changing
	// the int64 into a float64. We will fix it if we find it is actually a
	// problem.
	const nonFloatInt = (int64(1) << 54) + 1
	const asFloat = float64(nonFloatInt)
	c.Assert(int64(asFloat), gc.Not(gc.Equals), nonFloatInt)
	c.Assert(int64(asFloat)+1, gc.Equals, nonFloatInt)

	ch := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "test-service", ch)

	err := svc.UpdateConfigSettings(map[string]interface{}{"skill-level": nonFloatInt})
	c.Assert(err, jc.ErrorIsNil)
	client := apiapplication.NewClient(s.APIState)
	got, err := client.Get(svc.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.Config["skill-level"], jc.DeepEquals, map[string]interface{}{
		"description": "A number indicating skill.",
		"type":        "int",
		"value":       asFloat,
	})
}
