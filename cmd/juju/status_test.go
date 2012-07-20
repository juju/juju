package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/cmd"
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
	err = s.conn.Environ.Bootstrap(false)
	c.Assert(err, IsNil)
	s.st, err = s.conn.State()
	c.Assert(err, IsNil)
}

func (s *StatusSuite) TearDownTest(c *C) {
	s.envSuite.TearDownTest(c)
	os.Setenv("JUJU_REPOSITORY", s.repoPath)
	s.conn.Close()
}

var statusTests = []struct {
	title   string
	prepare func(*state.State, *C)
	yaml    string
}{
	{
		// unlikely, as you can't run juju status in real life without 
		// machine/0 bootstrapped.
		"empty state",
		func(*state.State, *C) {},
		"machines: {}\nservices: {}\n\n",
	},
	{
		"bootstrapped",
		func(st *state.State, c *C) {
			_, err := st.AddMachine()
			c.Assert(err, IsNil)
		},
		"machines: {}\nservices: {}\n\n",
	},
}

func (s *StatusSuite) TestYamlStatus(c *C) {
	for _, t := range statusTests {
		c.Logf("testing yaml: %s", t.title)
		t.prepare(s.st, c)
		ctx := &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}}
		code := cmd.Main(&StatusCommand{}, ctx, []string{"--format", "yaml"})
		c.Check(code, Equals, 0)
		c.Assert(ctx.Stderr.(*bytes.Buffer).String(), Equals, "")
		expected := make(map[string]interface{})
		err := goyaml.Unmarshal([]byte(t.yaml), &expected)
		c.Assert(err, IsNil)
		actual := make(map[string]interface{})
		err = goyaml.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &actual)
		c.Assert(err, IsNil)
		c.Assert(actual, DeepEquals, expected)
	}
}

// TODO(dfc) test json output
