// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
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
	c.Assert(err, jc.ErrorIsNil)
	for _, t := range getTests {
		ctx := coretesting.Context(c)
		code := cmd.Main(envcmd.Wrap(&GetCommand{}), ctx, []string{t.service})
		c.Check(code, gc.Equals, 0)
		c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
		// round trip via goyaml to avoid being sucked into a quagmire of
		// map[interface{}]interface{} vs map[string]interface{}. This is
		// also required if we add json support to this command.
		buf, err := goyaml.Marshal(t.expected)
		c.Assert(err, jc.ErrorIsNil)
		expected := make(map[string]interface{})
		err = goyaml.Unmarshal(buf, &expected)
		c.Assert(err, jc.ErrorIsNil)

		actual := make(map[string]interface{})
		err = goyaml.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &actual)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(actual, gc.DeepEquals, expected)
	}
}
