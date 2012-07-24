package main

import (
	"bytes"
	"encoding/json"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"os"
	"path/filepath"
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
		// simulate juju bootstrap by adding machine/0 to the state.
		"bootstrap/pending",
		func(st *state.State, _ *juju.Conn, c *C) {
			m, err := st.AddMachine()
			c.Assert(err, IsNil)
			c.Assert(m.Id(), Equals, 0)
		},
		map[string]interface{}{
			// note: the key of the machines map is a string
			"machines": map[int]interface{}{
				0: map[string]interface{}{
					"instance-id": "pending",
				},
			},
			"services": make(map[int]interface{}),
		},
	},
	{
		// simulate the PA starting an instance in response to the state change.
		"bootstrap/running",
		func(st *state.State, conn *juju.Conn, c *C) {
			m, err := st.Machine(0)
			c.Assert(err, IsNil)
			inst, err := conn.Environ.StartInstance(m.Id(), nil)
			c.Assert(err, IsNil)
			err = m.SetInstanceId(inst.Id())
			c.Assert(err, IsNil)
		},
		map[string]interface{}{
			// note: the key of the machines map is a string
			"machines": map[int]interface{}{
				0: map[string]interface{}{
					"dns-name":    "palermo-0.dns",
					"instance-id": "palermo-0",
				},
			},
			"services": make(map[string]interface{}),
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

		buf, err := marshal(t.output)
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
