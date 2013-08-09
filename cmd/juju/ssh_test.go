// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"net/url"
	"os"
	"path/filepath"
)

var _ = Suite(&SSHSuite{})

type SSHSuite struct {
	SSHCommonSuite
}

type SSHCommonSuite struct {
	testing.JujuConnSuite
	oldpath string
}

// fakecommand outputs its arguments to stdout for verification
var fakecommand = `#!/bin/bash

echo $@
`

func (s *SSHCommonSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	path := c.MkDir()
	s.oldpath = os.Getenv("PATH")
	os.Setenv("PATH", path+":"+s.oldpath)
	for _, name := range []string{"ssh", "scp"} {
		f, err := os.OpenFile(filepath.Join(path, name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
		c.Assert(err, IsNil)
		_, err = f.Write([]byte(fakecommand))
		c.Assert(err, IsNil)
		err = f.Close()
		c.Assert(err, IsNil)
	}
}

func (s *SSHCommonSuite) TearDownTest(c *C) {
	os.Setenv("PATH", s.oldpath)
	s.JujuConnSuite.TearDownTest(c)
}

const (
	commonArgs = `-o StrictHostKeyChecking no -o PasswordAuthentication no `
	sshArgs    = `-l ubuntu -t ` + commonArgs
)

var sshTests = []struct {
	args   []string
	result string
}{
	{
		[]string{"ssh", "0"},
		sshArgs + "dummyenv-0.dns\n",
	},
	// juju ssh 0 'uname -a'
	{
		[]string{"ssh", "0", "uname -a"},
		sshArgs + "dummyenv-0.dns uname -a\n",
	},
	// juju ssh 0 -- uname -a
	{
		[]string{"ssh", "0", "--", "uname", "-a"},
		sshArgs + "dummyenv-0.dns -- uname -a\n",
	},
	// juju ssh 0 uname -a
	{
		[]string{"ssh", "0", "uname", "-a"},
		sshArgs + "dummyenv-0.dns uname -a\n",
	},
	{
		[]string{"ssh", "mysql/0"},
		sshArgs + "dummyenv-0.dns\n",
	},
	{
		[]string{"ssh", "mongodb/1"},
		sshArgs + "dummyenv-2.dns\n",
	},
}

func (s *SSHSuite) TestSSHCommand(c *C) {
	m := s.makeMachines(3, c)
	ch := coretesting.Charms.Dir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:series/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.testing.invalid/dummy-1")
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
		ctx := coretesting.Context(c)
		jujucmd := cmd.NewSuperCommand(cmd.SuperCommandParams{})
		jujucmd.Register(&SSHCommand{})

		code := cmd.Main(jujucmd, ctx, t.args)
		c.Check(code, Equals, 0)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), Equals, "")
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), Equals, t.result)
	}
}

func (s *SSHCommonSuite) makeMachines(n int, c *C) []*state.Machine {
	var machines = make([]*state.Machine, n)
	for i := 0; i < n; i++ {
		m, err := s.State.AddMachine("series", state.JobHostUnits)
		c.Assert(err, IsNil)
		// must set an instance id as the ssh command uses that as a signal the machine
		// has been provisioned
		inst, md := testing.StartInstance(c, s.Conn.Environ, m.Id())
		c.Assert(m.SetProvisioned(inst.Id(), "fake_nonce", md), IsNil)
		machines[i] = m
	}
	return machines
}

func (s *SSHCommonSuite) addUnit(srv *state.Service, m *state.Machine, c *C) {
	u, err := srv.AddUnit()
	c.Assert(err, IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, IsNil)
	// fudge unit.SetPublicAddress
	id, err := m.InstanceId()
	c.Assert(err, IsNil)
	insts, err := s.Conn.Environ.Instances([]instance.Id{id})
	c.Assert(err, IsNil)
	addr, err := insts[0].WaitDNSName()
	c.Assert(err, IsNil)
	err = u.SetPublicAddress(addr)
	c.Assert(err, IsNil)
}
