// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"bytes"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/service"
	coretesting "github.com/juju/juju/testing"
)

type GetSuite struct {
	coretesting.FakeJujuHomeSuite
	fake *fakeServiceAPI
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
					"description": "Specifies title",
					"type":        "string",
					"value":       "Nearly There",
				},
				"skill-level": map[string]interface{}{
					"description": "Specifies skill-level",
					"value":       100,
					"type":        "int",
				},
				"username": map[string]interface{}{
					"description": "Specifies username",
					"type":        "string",
					"value":       "admin001",
				},
				"outlook": map[string]interface{}{
					"description": "Specifies outlook",
					"type":        "string",
					"value":       "true",
				},
			},
		},
	},

	// TODO(dfc) add additional services (need more charms)
	// TODO(dfc) add set tests
}

func (s *GetSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.fake = &fakeServiceAPI{servName: "dummy-service", charmName: "dummy",
		values: map[string]interface{}{
			"title":       "Nearly There",
			"skill-level": 100,
			"username":    "admin001",
			"outlook":     "true",
		}}
}

func (s *GetSuite) TestGetCommandInit(c *gc.C) {
	// missing args
	err := coretesting.InitCommand(&service.GetCommand{}, []string{})
	c.Assert(err, gc.ErrorMatches, "no service name specified")
}

func (s *GetSuite) TestGetConfig(c *gc.C) {
	for _, t := range getTests {
		ctx := coretesting.Context(c)
		code := cmd.Main(envcmd.Wrap(service.NewGetCommand(s.fake)), ctx, []string{t.service})
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
