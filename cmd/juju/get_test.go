// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
)

type GetSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&GetSuite{})

var getTests = []struct {
	service  string
	expected map[string]interface{}
}{
	{
		"dummy-service",
		map[string]interface{}{
			"service": "dummy-service",
			"charm":   "dummy",
			"settings": map[string]interface{}{
				"title": map[string]interface{}{
					"description": "A descriptive title used for the service.",
					"type":        "string",
					"value":       "Nearly There",
				},
				"skill-level": map[string]interface{}{
					"description": "A number indicating skill.",
					"type":        "int",
					"default":     true,
				},
				"username": map[string]interface{}{
					"description": "The name of the initial account (given admin permissions).",
					"type":        "string",
					"value":       "admin001",
					"default":     true,
				},
				"outlook": map[string]interface{}{
					"description": "No default outlook.",
					"type":        "string",
					"default":     true,
				},
			},
		},
	},

	// TODO(dfc) add additional services (need more charms)
	// TODO(dfc) add set tests
}

func (s *GetSuite) TestGetConfig(c *gc.C) {
	sch := s.AddTestingCharm(c, "dummy")
	svc := s.AddTestingService(c, "dummy-service", sch)
	err := svc.UpdateConfigSettings(charm.Settings{"title": "Nearly There"})
	c.Assert(err, gc.IsNil)
	for _, t := range getTests {
		ctx := coretesting.Context(c)
		code := cmd.Main(&GetCommand{}, ctx, []string{t.service})
		c.Check(code, gc.Equals, 0)
		c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
		// round trip via goyaml to avoid being sucked into a quagmire of
		// map[interface{}]interface{} vs map[string]interface{}. This is
		// also required if we add json support to this command.
		buf, err := goyaml.Marshal(t.expected)
		c.Assert(err, gc.IsNil)
		expected := make(map[string]interface{})
		err = goyaml.Unmarshal(buf, &expected)
		c.Assert(err, gc.IsNil)

		actual := make(map[string]interface{})
		err = goyaml.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &actual)
		c.Assert(err, gc.IsNil)
		c.Assert(actual, gc.DeepEquals, expected)
	}
}
