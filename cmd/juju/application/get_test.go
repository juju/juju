// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"bytes"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/juju/application"
	coretesting "github.com/juju/juju/testing"
)

type GetSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	fake *fakeServiceAPI
}

var _ = gc.Suite(&GetSuite{})

var getTests = []struct {
	application string
	expected    map[string]interface{}
}{
	{
		"dummy-application",
		map[string]interface{}{
			"application": "dummy-application",
			"charm":       "dummy",
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
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fake = &fakeServiceAPI{serviceName: "dummy-application", charmName: "dummy",
		values: map[string]interface{}{
			"title":       "Nearly There",
			"skill-level": 100,
			"username":    "admin001",
			"outlook":     "true",
		}}
}

func (s *GetSuite) TestGetCommandInit(c *gc.C) {
	// missing args
	err := coretesting.InitCommand(application.NewGetCommandForTest(s.fake), []string{})
	c.Assert(err, gc.ErrorMatches, "no application name specified")
}

func (s *GetSuite) TestGetCommandInitWithApplication(c *gc.C) {
	err := coretesting.InitCommand(application.NewGetCommandForTest(s.fake), []string{"app"})
	// everything ok
	c.Assert(err, jc.ErrorIsNil)
}

func (s *GetSuite) TestGetCommandInitWithKey(c *gc.C) {
	err := coretesting.InitCommand(application.NewGetCommandForTest(s.fake), []string{"app", "key"})
	// everything ok
	c.Assert(err, jc.ErrorIsNil)
}

func (s *GetSuite) TestGetCommandInitTooManyArgs(c *gc.C) {
	err := coretesting.InitCommand(application.NewGetCommandForTest(s.fake), []string{"app", "key", "another"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["another"\]`)
}

func (s *GetSuite) TestGetConfig(c *gc.C) {
	for _, t := range getTests {
		ctx := coretesting.Context(c)
		code := cmd.Main(application.NewGetCommandForTest(s.fake), ctx, []string{t.application})
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

func (s *GetSuite) TestGetConfigKey(c *gc.C) {
	ctx := coretesting.Context(c)
	code := cmd.Main(application.NewGetCommandForTest(s.fake), ctx, []string{"dummy-application", "title"})
	c.Check(code, gc.Equals, 0)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, "Nearly There\n")
}

func (s *GetSuite) TestGetConfigKeyNotFound(c *gc.C) {
	ctx := coretesting.Context(c)
	code := cmd.Main(application.NewGetCommandForTest(s.fake), ctx, []string{"dummy-application", "invalid"})
	c.Check(code, gc.Equals, 1)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "error: key \"invalid\" not found in \"dummy-application\" application settings.\n")
	c.Assert(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, "")
}
