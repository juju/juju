package main

import (
	"bytes"
	"os"

	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/testing"
)

// juju get and set tests (because one needs the other)

type ConfigSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&ConfigSuite{})

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
				"skill-level": map[string]interface{}{
					"description": "A number indicating skill.",
					"type":        "int",
					"value":       nil,
				},
				"title": map[string]interface{}{
					"description": "A descriptive title used for the service.",
					"type":        "string",
					"value":       nil,
				},
				"username": map[string]interface{}{
					"description": "The name of the initial account (given admin permissions).",
					"type":        "string",
					"value":       nil,
				},
				"outlook": map[string]interface{}{
					"description": "No default outlook.",
					"type":        "string",
					"value":       nil,
				},
			},
		},
	},

	// TODO(dfc) add additional services (need more charms)
	// TODO(dfc) add set tests
}

func (s *ConfigSuite) TestGetConfig(c *C) {
	sch := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddService("dummy-service", sch)
	c.Assert(err, IsNil)
	for _, t := range getTests {
		ctx := &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}}
		code := cmd.Main(&GetCommand{}, ctx, []string{t.service})
		c.Check(code, Equals, 0)
		c.Assert(ctx.Stderr.(*bytes.Buffer).String(), Equals, "")
		// round trip via goyaml to avoid being sucked into a quagmire of
		// map[interface{}]interface{} vs map[string]interface{}. This is
		// also required if we add json support to this command.
		buf, err := goyaml.Marshal(t.expected)
		c.Assert(err, IsNil)
		expected := make(map[string]interface{})
		err = goyaml.Unmarshal(buf, &expected)
		c.Assert(err, IsNil)

		actual := make(map[string]interface{})
		err = goyaml.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &actual)
		c.Assert(err, IsNil)
		c.Assert(actual, DeepEquals, expected)
	}
}

var setTests = []struct {
	args     []string               // command to be executed
	expected map[string]interface{} // resulting configuration of the dummy service.
	err      string                 // error regex
}{
	{
		// unnown option
		[]string{"foo=bar"},
		nil,
		"error: Unknown configuration option: \"foo\"\n",
	}, {
		// invalid option
		[]string{"foo", "bar"},
		nil,
		"error: invalid option: \"foo\"\n",
	}, {
		// whack option
		[]string{"=bar"},
		nil,
		"error: invalid option: \"=bar\"\n",
	}, {
		// set outlook
		[]string{"outlook=positive"},
		map[string]interface{}{
			"outlook": "positive",
		},
		"",
	}, {
		// unset outlook and set title
		[]string{"outlook=", "title=sir"},
		map[string]interface{}{
			"title": "sir",
		},
		"",
	}, {
		// set a default value
		[]string{"username=admin001"},
		map[string]interface{}{
			"username": "admin001",
			"title":    "sir",
		},
		"",
	}, {
		// unset a default value, set a different default
		[]string{"username=", "title=My Title"},
		map[string]interface{}{
			"title": "My Title",
		},
		"",
	}, {
		// --config missing
		[]string{"--config", "missing.yaml"},
		nil,
		"error.*no such file or directory\n",
	}, {
		// --config $FILE test
		[]string{"--config", "testconfig.yaml"},
		map[string]interface{}{
			"title":       "My Title",
			"username":    "admin001",
			"skill-level": int64(9000), // yaml int types are int64
		},
		"",
	},
}

func (s *ConfigSuite) TestSetConfig(c *C) {
	sch := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("dummy-service", sch)
	c.Assert(err, IsNil)
	dir := c.MkDir()
	setupConfigfile(c, dir)
	for _, t := range setTests {
		args := append([]string{"dummy-service"}, t.args...)
		c.Logf("%s", args)
		ctx := &cmd.Context{dir, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}}
		code := cmd.Main(&SetCommand{}, ctx, args)
		if code != 0 {
			c.Assert(ctx.Stderr.(*bytes.Buffer).String(), Matches, t.err)
		} else {
			cfg, err := svc.Config()
			c.Assert(err, IsNil)
			c.Assert(cfg.Map(), DeepEquals, t.expected)
		}
	}
}

func setupConfigfile(c *C, dir string) {
	ctx := &cmd.Context{dir, nil, nil, nil}
	path := ctx.AbsPath("testconfig.yaml")
	file, err := os.Create(path)
	c.Assert(err, IsNil)
	_, err = file.Write([]byte("skill-level: 9000\nusername: admin001\n\n"))
	c.Assert(err, IsNil)
	file.Close()
}
