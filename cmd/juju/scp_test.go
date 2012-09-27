package main

import (
	"bytes"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"net/url"
	"os"
	"path/filepath"
)

var _ = Suite(&SCPSuite{})

type SCPSuite struct {
	testing.JujuConnSuite
	oldpath string
}

// fakescp outputs its arguments to stdout for verification
var fakescp = `#!/bin/bash

echo $@
`

func (s *SCPSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	path := c.MkDir()
	s.oldpath = os.Getenv("PATH")
	os.Setenv("PATH", path+":"+s.oldpath)
	f, err := os.OpenFile(filepath.Join(path, "scp"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	c.Assert(err, IsNil)
	_, err = f.Write([]byte(fakessh))
	c.Assert(err, IsNil)
	err = f.Close()
	c.Assert(err, IsNil)
}

func (s *SCPSuite) TearDownTest(c *C) {
	os.Setenv("PATH", s.oldpath)
	s.JujuConnSuite.TearDownTest(c)
}

var scpTests = []struct {
	args   []string
	result string
}{
	{[]string{"0:foo", "."}, "-o StrictHostKeyChecking no -o PasswordAuthentication no ubuntu@dummyenv-0.dns:foo .\n"},
	{[]string{"foo", "0:"}, "-o StrictHostKeyChecking no -o PasswordAuthentication no foo ubuntu@dummyenv-0.dns:\n"},
	{[]string{"0:foo", "mysql/0:/foo"}, "-o StrictHostKeyChecking no -o PasswordAuthentication no ubuntu@dummyenv-0.dns:foo ubuntu@dummyenv-0.dns:/foo\n"},
	{[]string{"a", "b", "mysql/0"}, "-o StrictHostKeyChecking no -o PasswordAuthentication no a b mysql/0\n"},
	{[]string{"mongodb/1:foo", "mongodb/0:"}, "-o StrictHostKeyChecking no -o PasswordAuthentication no ubuntu@dummyenv-2.dns:foo ubuntu@dummyenv-1.dns:\n"},
}

func (s *SCPSuite) TestSCPCommand(c *C) {
	m := s.makeMachines(3, c)
	ch := coretesting.Charms.Dir("series", "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:series/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.example.com/dummy-1")
	c.Assert(err, IsNil)
	dummy, err := s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, IsNil)
	srv, err := s.State.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	u, err := srv.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m[0])
	c.Assert(err, IsNil)

	srv, err = s.State.AddService("mongodb", dummy)
	c.Assert(err, IsNil)
	u, err = srv.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m[1])
	c.Assert(err, IsNil)
	u, err = srv.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m[2])
	c.Assert(err, IsNil)

	for _, t := range scpTests {
		c.Logf("testing juju scp %s", t.args)
		ctx := &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}}
		code := cmd.Main(&SCPCommand{}, ctx, t.args)
		c.Check(code, Equals, 0)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), Equals, "")
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), Equals, t.result)
	}
}

func (s *SCPSuite) makeMachines(n int, c *C) []*state.Machine {
	var machines = make([]*state.Machine, n)
	for i := 0; i < n; i++ {
		m, err := s.State.AddMachine()
		c.Assert(err, IsNil)
		// must set an instance id as the scp command uses that as a signal the machine
		// has been provisioned
		inst, err := s.Conn.Environ.StartInstance(m.Id(), nil, nil)
		c.Assert(err, IsNil)
		c.Assert(m.SetInstanceId(inst.Id()), IsNil)
		machines[i] = m
	}
	return machines
}
