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

var _ = Suite(&SSHSuite{})

type SSHSuite struct {
	testing.JujuConnSuite
	oldpath string
}

// fakessh outputs its arguments to stdout for verification
var fakessh = `#!/bin/bash

echo $@
`

func (s *SSHSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	path := c.MkDir()
	s.oldpath = os.Getenv("PATH")
	os.Setenv("PATH", path+":"+s.oldpath)
	f, err := os.OpenFile(filepath.Join(path, "ssh"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	c.Assert(err, IsNil)
	_, err = f.Write([]byte(fakessh))
	c.Assert(err, IsNil)
	err = f.Close()
	c.Assert(err, IsNil)
}

func (s *SSHSuite) TearDownTest(c *C) {
	os.Setenv("PATH", s.oldpath)
	s.JujuConnSuite.TearDownTest(c)
}

const commonArgs = "-l ubuntu -t -o StrictHostKeyChecking no -o PasswordAuthentication no "

var sshTests = []struct {
	args   []string
	result string
}{
	{
		[]string{"0"},
		commonArgs + "dummyenv-0.dns\n",
	},
	// juju ssh 0 'uname -a'
	{
		[]string{"0", "uname -a"},
		commonArgs + "dummyenv-0.dns uname -a\n",
	},
	// juju ssh 0 -- uname -a
	{
		[]string{"0", "--", "uname", "-a"},
		commonArgs + "dummyenv-0.dns uname -a\n",
	},
	{
		[]string{"mysql/0"},
		commonArgs + "dummyenv-0.dns\n",
	},
	{
		[]string{"mongodb/1"},
		commonArgs + "dummyenv-2.dns\n",
	},
}

func (s *SSHSuite) TestSSHCommand(c *C) {
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
	s.addUnit(srv, m[0], c)

	srv, err = s.State.AddService("mongodb", dummy)
	c.Assert(err, IsNil)
	s.addUnit(srv, m[1], c)
	s.addUnit(srv, m[2], c)

	for _, t := range sshTests {
		c.Logf("testing juju ssh %s", t.args)
		ctx := &cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}}
		code := cmd.Main(&SSHCommand{}, ctx, t.args)
		c.Check(code, Equals, 0)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), Equals, "")
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), Equals, t.result)
	}
}

func (s *SSHSuite) makeMachines(n int, c *C) []*state.Machine {
	var machines = make([]*state.Machine, n)
	for i := 0; i < n; i++ {
		m, err := s.State.AddMachine()
		c.Assert(err, IsNil)
		// must set an instance id as the ssh command uses that as a signal the machine
		// has been provisioned
		inst, err := s.Conn.Environ.StartInstance(m.Id(), nil, nil)
		c.Assert(err, IsNil)
		c.Assert(m.SetInstanceId(inst.Id()), IsNil)
		machines[i] = m
	}
	return machines
}

func (s *SSHSuite) addUnit(srv *state.Service, m *state.Machine, c *C) {
	u, err := srv.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, IsNil)
	// fudge unit.SetPublicAddress
	id, err := m.InstanceId()
	c.Assert(err, IsNil)
	insts, err := s.Conn.Environ.Instances([]string{id})
	c.Assert(err, IsNil)
	addr, err := insts[0].WaitDNSName()
	c.Assert(err, IsNil)
	err = u.SetPublicAddress(addr)
	c.Assert(err, IsNil)
}
