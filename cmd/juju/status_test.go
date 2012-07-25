package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
)

type StatusSuite struct {
	envSuite
	repoPath, seriesPath string
	conn                 *juju.Conn
	st                   *state.State
}

var _ = Suite(&StatusSuite{})

func (s *StatusSuite) SetUpTest(c *C) {
	s.envSuite.SetUpTest(c, zkConfig)
	repoPath := c.MkDir()
	s.repoPath = os.Getenv("JUJU_REPOSITORY")
	os.Setenv("JUJU_REPOSITORY", repoPath)
	s.seriesPath = filepath.Join(repoPath, "precise")
	err := os.Mkdir(s.seriesPath, 0777)
	c.Assert(err, IsNil)
	s.conn, err = juju.NewConn("")
	c.Assert(err, IsNil)
	err = s.conn.Bootstrap(false)
	c.Assert(err, IsNil)
	s.st, err = s.conn.State()
	c.Assert(err, IsNil)
}

func (s *StatusSuite) TearDownTest(c *C) {
	s.conn.Close()
	dummy.Reset()
	//	s.StateSuite.TearDownTest(c)
	s.envSuite.TearDownTest(c)
	os.Setenv("JUJU_REPOSITORY", s.repoPath)
}

var statusTests = []struct {
	title   string
	prepare func(*state.State, *juju.Conn, *C)
	output  map[string]interface{}
}{
	{
		// unlikely, as you can't run juju status in real life without 
		// machine/0 bootstrapped.
		"empty state",
		func(*state.State, *juju.Conn, *C) {},
		map[string]interface{}{
			"machines": make(map[int]interface{}),
			"services": make(map[string]interface{}),
		},
	},
	{
		"simulate juju bootstrap by adding machine/0 to the state",
		func(st *state.State, _ *juju.Conn, c *C) {
			m, err := st.AddMachine()
			c.Assert(err, IsNil)
			c.Assert(m.Id(), Equals, 0)
		},
		map[string]interface{}{
			"machines": map[int]interface{}{
				0: map[string]interface{}{
					"instance-id": "pending",
				},
			},
			"services": make(map[string]interface{}),
		},
	},
	{
		"simulate the PA starting an instance in response to the state change",
		func(st *state.State, conn *juju.Conn, c *C) {
			m, err := st.Machine(0)
			c.Assert(err, IsNil)
			inst, err := conn.Environ.StartInstance(m.Id(), nil)
			c.Assert(err, IsNil)
			err = m.SetInstanceId(inst.Id())
			c.Assert(err, IsNil)
		},
		map[string]interface{}{
			"machines": map[int]interface{}{
				0: map[string]interface{}{
					"dns-name":    "palermo-0.dns",
					"instance-id": "palermo-0",
				},
			},
			"services": make(map[string]interface{}),
		},
	},
	{
		"add two services and expose one",
		func(st *state.State, conn *juju.Conn, c *C) {
			ch := coretesting.Charms.Dir("dummy")
			curl := charm.MustParseURL(
				fmt.Sprintf("local:series/%s-%d", ch.Meta().Name, ch.Revision()),
			)
			bundleURL, err := url.Parse("http://bundles.example.com/dummy-1")
			c.Assert(err, IsNil)
			dummy, err := st.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
			c.Assert(err, IsNil)
			_, err = st.AddService("dummy-service", dummy)
			c.Assert(err, IsNil)
			s, err := st.AddService("exposed-service", dummy)
			c.Assert(err, IsNil)
			err = s.SetExposed()
			c.Assert(err, IsNil)
		},
		map[string]interface{}{
			"machines": map[int]interface{}{
				0: map[string]interface{}{
					"dns-name":    "palermo-0.dns",
					"instance-id": "palermo-0",
				},
			},
			"services": map[string]interface{}{
				"dummy-service": map[string]interface{}{
					"charm":   "dummy",
					"exposed": false,
				},
				"exposed-service": map[string]interface{}{
					"charm":   "dummy",
					"exposed": true,
				},
			},
		},
	},
}

func (s *StatusSuite) testStatus(format string, marshal func(v interface{}) ([]byte, error), unmarshal func(data []byte, v interface{}) error, c *C) {
	for _, t := range statusTests {
		c.Logf("testing %s: %s", format, t.title)
		t.prepare(s.st, s.conn, c)
		ctx := &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}}
		code := cmd.Main(&StatusCommand{}, ctx, []string{"--format", format})
		c.Check(code, Equals, 0)
		c.Assert(ctx.Stderr.(*bytes.Buffer).String(), Equals, "")

		var buf []byte
		var err error
		if format == "json" {
			buf, err = marshal(Jsonify(t.output))
		} else {
			buf, err = marshal(t.output)
		}
		c.Assert(err, IsNil)
		expected := make(map[string]interface{})
		err = unmarshal(buf, &expected)
		c.Assert(err, IsNil)

		actual := make(map[string]interface{})
		err = unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &actual)
		c.Assert(err, IsNil)

		c.Assert(actual, DeepEquals, expected)
	}
}

func (s *StatusSuite) TestYamlStatus(c *C) {
	s.testStatus("yaml", goyaml.Marshal, goyaml.Unmarshal, c)
}

func (s *StatusSuite) TestJsonStatus(c *C) {
	s.testStatus("json", json.Marshal, json.Unmarshal, c)
}
