package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"

	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

type StatusSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&StatusSuite{})

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
			m, err := st.AddMachine(state.MachinerWorker)
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
			inst, err := conn.Environ.StartInstance(m.Id(), testing.InvalidStateInfo(m.Id()), nil)
			c.Assert(err, IsNil)
			err = m.SetInstanceId(inst.Id())
			c.Assert(err, IsNil)
		},
		map[string]interface{}{
			"machines": map[int]interface{}{
				0: map[string]interface{}{
					"dns-name":    "dummyenv-0.dns",
					"instance-id": "dummyenv-0",
				},
			},
			"services": make(map[string]interface{}),
		},
	},
	{
		"simulate the MA setting the version",
		func(st *state.State, conn *juju.Conn, c *C) {
			m, err := st.Machine(0)
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
		},
		map[string]interface{}{
			"machines": map[int]interface{}{
				0: map[string]interface{}{
					"dns-name":      "dummyenv-0.dns",
					"instance-id":   "dummyenv-0",
					"agent-version": "1.2.3",
				},
			},
			"services": make(map[string]interface{}),
		},
	},
	{
		"add two services and expose one",
		func(st *state.State, conn *juju.Conn, c *C) {
			ch := coretesting.Charms.Dir("series", "dummy")
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
					"dns-name":      "dummyenv-0.dns",
					"instance-id":   "dummyenv-0",
					"agent-version": "1.2.3",
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
	{
		"add two more machines for units",
		func(st *state.State, conn *juju.Conn, c *C) {
			for i := 1; i < 3; i++ {
				m, err := st.AddMachine(state.MachinerWorker)
				c.Assert(err, IsNil)
				c.Assert(m.Id(), Equals, i)
				inst, err := conn.Environ.StartInstance(m.Id(), testing.InvalidStateInfo(m.Id()), nil)
				c.Assert(err, IsNil)
				err = m.SetInstanceId(inst.Id())
				c.Assert(err, IsNil)
			}
		},
		map[string]interface{}{
			"machines": map[int]interface{}{
				0: map[string]interface{}{
					"dns-name":      "dummyenv-0.dns",
					"instance-id":   "dummyenv-0",
					"agent-version": "1.2.3",
				},
				1: map[string]interface{}{
					"dns-name":    "dummyenv-1.dns",
					"instance-id": "dummyenv-1",
				},
				2: map[string]interface{}{
					"dns-name":    "dummyenv-2.dns",
					"instance-id": "dummyenv-2",
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
	{
		"add units for services",
		func(st *state.State, conn *juju.Conn, c *C) {
			for i, n := range []string{"dummy-service", "exposed-service"} {
				s, err := st.Service(n)
				c.Assert(err, IsNil)
				u, err := s.AddUnit()
				c.Assert(err, IsNil)
				m, err := st.Machine(i + 1)
				c.Assert(err, IsNil)
				err = u.AssignToMachine(m)
				c.Assert(err, IsNil)

				if n == "exposed-service" {
					err := u.SetStatus("error", "You Require More Vespene Gas")
					c.Assert(err, IsNil)
				}
			}
		},
		map[string]interface{}{
			"machines": map[int]interface{}{
				0: map[string]interface{}{
					"dns-name":      "dummyenv-0.dns",
					"instance-id":   "dummyenv-0",
					"agent-version": "1.2.3",
				},
				1: map[string]interface{}{
					"dns-name":    "dummyenv-1.dns",
					"instance-id": "dummyenv-1",
				},
				2: map[string]interface{}{
					"dns-name":    "dummyenv-2.dns",
					"instance-id": "dummyenv-2",
				},
			},
			"services": map[string]interface{}{
				"exposed-service": map[string]interface{}{
					"exposed": true,
					"units": map[string]interface{}{
						"exposed-service/0": map[string]interface{}{
							"machine":     2,
							"status":      "error",
							"status-info": "You Require More Vespene Gas",
						},
					},
					"charm": "dummy",
				},
				"dummy-service": map[string]interface{}{
					"charm":   "dummy",
					"exposed": false,
					"units": map[string]interface{}{
						"dummy-service/0": map[string]interface{}{
							"machine": 1,
							"status":  "pending",
						},
					},
				},
			},
		},
	},

	// TODO(dfc) test failing components by destructively mutating the state under the hood
}

func (s *StatusSuite) testStatus(format string, marshal func(v interface{}) ([]byte, error), unmarshal func(data []byte, v interface{}) error, c *C) {
	for _, t := range statusTests {
		c.Logf("testing %s: %s", format, t.title)
		t.prepare(s.State, s.Conn, c)
		ctx := &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}}
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
