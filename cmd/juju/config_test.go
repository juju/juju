// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"io/ioutil"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
)

// juju get and set tests (because one needs the other)

type ConfigSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&ConfigSuite{})

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
					"value":       nil,
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
					"value":       nil,
				},
			},
		},
	},

	// TODO(dfc) add additional services (need more charms)
	// TODO(dfc) add set tests
}

func (s *ConfigSuite) TestGetConfig(c *gc.C) {
	sch := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("dummy-service", sch)
	c.Assert(err, gc.IsNil)
	err = svc.UpdateConfigSettings(charm.Settings{"title": "Nearly There"})
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

var setUnsetTests = []struct {
	about  string
	unset  bool
	args   []string       // command to be executed
	expect charm.Settings // resulting configuration of the dummy service.
	err    string         // error regex
}{{
	about: "invalid option",
	args:  []string{"foo", "bar"},
	err:   "error: invalid option: \"foo\"\n",
}, {
	about: "whack option",
	args:  []string{"=bar"},
	err:   "error: invalid option: \"=bar\"\n",
}, {
	about: "--config missing",
	args:  []string{"--config", "missing.yaml"},
	err:   "error.*no such file or directory\n",
}, {
	about: "set with options",
	args:  []string{"username=hello", "outlook=hello@world.tld"},
	expect: charm.Settings{
		"username": "hello",
		"outlook":  "hello@world.tld",
	},
}, {
	about: "set with option values containing =",
	args:  []string{"username=hello=foo"},
	expect: charm.Settings{
		"username": "hello=foo",
		"outlook":  "hello@world.tld",
	},
}, {
	about: "set to default value",
	unset: true,
	args:  []string{"username"},
	expect: charm.Settings{
		"username": "admin001",
		"outlook":  "hello@world.tld",
	},
}, {
	about: "set to a nil default value (aka remove a setting)",
	unset: true,
	args:  []string{"outlook"},
	expect: charm.Settings{
		"username": "admin001",
	},
}, {
	about: "set illegal option to default",
	unset: true,
	args:  []string{"lookout"},
	err:   "error: invalid option: \"lookout\"\n",
}, {
	about: "mixing unset and set expression",
	unset: true,
	args:  []string{"username=admin002"},
	err:   "error: invalid setting during unset: \"username=admin002\"\n",
}, {
	about: "setting valid and invalid options to default",
	unset: true,
	args:  []string{"username", "outlook", "invalidstuff"},
	err:   "error: invalid option: \"invalidstuff\"\n",
}, {
	about: "another set with options for multi-default in next test",
	args:  []string{"username=foobar", "outlook=foobar@barfoo.tld"},
	expect: charm.Settings{
		"username": "foobar",
		"outlook":  "foobar@barfoo.tld",
	},
}, {
	about: "set multiple options to their default values",
	unset: true,
	args:  []string{"username", "outlook"},
	expect: charm.Settings{
		"username": "admin001",
	},
}, {
	about: "--config $FILE test",
	args:  []string{"--config", "testconfig.yaml"},
	expect: charm.Settings{
		"username":    "admin001",
		"skill-level": int64(9000), // charm int types are int64
	},
},
}

func (s *ConfigSuite) TestSetUnsetConfig(c *gc.C) {
	sch := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("dummy-service", sch)
	c.Assert(err, gc.IsNil)
	dir := c.MkDir()
	setupConfigfile(c, dir)
	for i, t := range setUnsetTests {
		var command cmd.Command = &SetCommand{}
		if t.unset {
			command = &UnsetCommand{}
		}
		args := append([]string{"dummy-service"}, t.args...)
		c.Logf("test %d. %s", i, t.about)
		ctx := coretesting.ContextForDir(c, dir)
		code := cmd.Main(command, ctx, args)
		if t.err != "" {
			c.Check(code, gc.Not(gc.Equals), 0)
			c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Matches, t.err)
		} else {
			c.Check(code, gc.Equals, 0)
			settings, err := svc.ConfigSettings()
			c.Assert(err, gc.IsNil)
			c.Assert(settings, gc.DeepEquals, t.expect)
		}
	}
}

func setupConfigfile(c *gc.C, dir string) string {
	ctx := coretesting.ContextForDir(c, dir)
	path := ctx.AbsPath("testconfig.yaml")
	content := []byte("dummy-service:\n  skill-level: 9000\n  username: admin001\n\n")
	err := ioutil.WriteFile(path, content, 0666)
	c.Assert(err, gc.IsNil)
	return path
}
