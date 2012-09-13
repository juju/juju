package main

import (
	"bytes"
	"fmt"
	"net/url"

	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

// juju get and set tests (because one needs the other)

type ConfigSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(m.Id(), Equals, 0)
	inst, err := s.Conn.Environ.StartInstance(m.Id(), nil, nil)
	c.Assert(err, IsNil)
	err = m.SetInstanceId(inst.Id())
	c.Assert(err, IsNil)
	t := &state.Tools{
		Binary: version.Binary{
			Number: version.MustParse("1.2.3"),
			Series: "gutsy",
			Arch:   "ppc",
		},
		URL: "http://canonical.com/",
	}
	err = m.SetAgentTools(t)
	c.Assert(err, IsNil)
	ch := coretesting.Charms.Dir("series", "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:series/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.example.com/dummy-1")
	c.Assert(err, IsNil)
	dummy, err := s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, IsNil)
	_, err = s.State.AddService("dummy-service", dummy)
	c.Assert(err, IsNil)
}

func (s *ConfigSuite) TearDownTest(c *C) {
	s.JujuConnSuite.TearDownTest(c)
}

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
					"value":       "-Not Set-",
					"type":        "int",
				},
				"title": map[string]interface{}{
					"description": "A descriptive title used for the service.",
					"value":       "-Not Set-",
					"type":        "string",
				},
				"username": map[string]interface{}{
					"description": "The name of the initial account (given admin permissions).",
					"type":        "string",
					"value":       "-Not Set-",
				},
				"outlook": map[string]interface{}{
					"description": "No default outlook.",
					"value":       "-Not Set-",
					"type":        "string",
				},
			},
		},
	},

	// TODO(dfc) add additional services (need more charms)
	// TODO(dfc) add set tests
}

func (s *ConfigSuite) TestGetConfig(c *C) {
	for _, t := range getTests {
		ctx := &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}}
		code := cmd.Main(&GetCommand{}, ctx, []string{t.service})
		c.Check(code, Equals, 0)
		c.Assert(ctx.Stderr.(*bytes.Buffer).String(), Equals, "")
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
