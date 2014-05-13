// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
)

var _ = gc.Suite(&SSHSuite{})

type SSHSuite struct {
	SSHCommonSuite
}

type SSHCommonSuite struct {
	testing.JujuConnSuite
	bin string
}

// fakecommand outputs its arguments to stdout for verification
var fakecommand = `#!/bin/bash

echo $@ | tee $0.args
`

func (s *SSHCommonSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.PatchValue(&getJujuExecutable, func() (string, error) { return "juju", nil })

	s.bin = c.MkDir()
	s.PatchEnvPathPrepend(s.bin)
	for _, name := range []string{"ssh", "scp"} {
		f, err := os.OpenFile(filepath.Join(s.bin, name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
		c.Assert(err, gc.IsNil)
		_, err = f.Write([]byte(fakecommand))
		c.Assert(err, gc.IsNil)
		err = f.Close()
		c.Assert(err, gc.IsNil)
	}
}

const (
	commonArgsNoProxy = `-o StrictHostKeyChecking no -o PasswordAuthentication no `
	commonArgs        = `-o StrictHostKeyChecking no -o ProxyCommand juju ssh --proxy=false --pty=false 127.0.0.1 nc -q0 %h %p -o PasswordAuthentication no `
	sshArgs           = commonArgs + `-t -t `
	sshArgsNoProxy    = commonArgsNoProxy + `-t -t `
)

var sshTests = []struct {
	about  string
	args   []string
	result string
}{
	{
		"connect to machine 0",
		[]string{"ssh", "0"},
		sshArgs + "ubuntu@dummyenv-0.internal\n",
	},
	{
		"connect to machine 0 and pass extra arguments",
		[]string{"ssh", "0", "uname", "-a"},
		sshArgs + "ubuntu@dummyenv-0.internal uname -a\n",
	},
	{
		"connect to unit mysql/0",
		[]string{"ssh", "mysql/0"},
		sshArgs + "ubuntu@dummyenv-0.internal\n",
	},
	{
		"connect to unit mongodb/1 and pass extra arguments",
		[]string{"ssh", "mongodb/1", "ls", "/"},
		sshArgs + "ubuntu@dummyenv-2.internal ls /\n",
	},
	{
		"connect to unit mysql/0 without proxy",
		[]string{"ssh", "--proxy=false", "mysql/0"},
		sshArgsNoProxy + "ubuntu@dummyenv-0.dns\n",
	},
}

func (s *SSHSuite) TestSSHCommand(c *gc.C) {
	m := s.makeMachines(3, c, true)
	ch := coretesting.Charms.Dir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.testing.invalid/dummy-1")
	c.Assert(err, gc.IsNil)
	dummy, err := s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, gc.IsNil)
	srv := s.AddTestingService(c, "mysql", dummy)
	s.addUnit(srv, m[0], c)

	srv = s.AddTestingService(c, "mongodb", dummy)
	s.addUnit(srv, m[1], c)
	s.addUnit(srv, m[2], c)

	for i, t := range sshTests {
		c.Logf("test %d: %s -> %s\n", i, t.about, t.args)
		ctx := coretesting.Context(c)
		jujucmd := cmd.NewSuperCommand(cmd.SuperCommandParams{})
		jujucmd.Register(envcmd.Wrap(&SSHCommand{}))

		code := cmd.Main(jujucmd, ctx, t.args)
		c.Check(code, gc.Equals, 0)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, t.result)
	}
}

func (s *SSHSuite) TestSSHCommandEnvironProxySSH(c *gc.C) {
	s.makeMachines(1, c, true)
	// Setting proxy-ssh=false in the environment overrides --proxy.
	err := s.State.UpdateEnvironConfig(map[string]interface{}{"proxy-ssh": false}, nil, nil)
	c.Assert(err, gc.IsNil)
	ctx := coretesting.Context(c)
	jujucmd := cmd.NewSuperCommand(cmd.SuperCommandParams{})
	jujucmd.Register(&SSHCommand{})
	code := cmd.Main(jujucmd, ctx, []string{"ssh", "0"})
	c.Check(code, gc.Equals, 0)
	c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, sshArgsNoProxy+"ubuntu@dummyenv-0.dns\n")
}

type callbackAttemptStarter struct {
	next func() bool
}

func (s *callbackAttemptStarter) Start() attempt {
	return callbackAttempt{next: s.next}
}

type callbackAttempt struct {
	next func() bool
}

func (a callbackAttempt) Next() bool {
	return a.next()
}

func (s *SSHSuite) TestSSHCommandHostAddressRetry(c *gc.C) {
	s.testSSHCommandHostAddressRetry(c, false)
}

func (s *SSHSuite) TestSSHCommandHostAddressRetryProxy(c *gc.C) {
	s.testSSHCommandHostAddressRetry(c, true)
}

func (s *SSHSuite) testSSHCommandHostAddressRetry(c *gc.C, proxy bool) {
	m := s.makeMachines(1, c, false)
	ctx := coretesting.Context(c)

	var called int
	next := func() bool {
		called++
		return called < 2
	}
	attemptStarter := &callbackAttemptStarter{next: next}
	s.PatchValue(&sshHostFromTargetAttemptStrategy, attemptStarter)

	// Ensure that the ssh command waits for a public address, or the attempt
	// strategy's Done method returns false.
	args := []string{"--proxy=" + fmt.Sprint(proxy), "0"}
	code := cmd.Main(&SSHCommand{}, ctx, args)
	c.Check(code, gc.Equals, 1)
	c.Assert(called, gc.Equals, 2)
	called = 0
	attemptStarter.next = func() bool {
		called++
		if called > 1 {
			s.setAddresses(m[0], c)
		}
		return true
	}
	code = cmd.Main(&SSHCommand{}, ctx, args)
	c.Check(code, gc.Equals, 0)
	c.Assert(called, gc.Equals, 2)
}

func (s *SSHCommonSuite) setAddresses(m *state.Machine, c *gc.C) {
	addrPub := instance.NewAddress(fmt.Sprintf("dummyenv-%s.dns", m.Id()), instance.NetworkPublic)
	addrPriv := instance.NewAddress(fmt.Sprintf("dummyenv-%s.internal", m.Id()), instance.NetworkCloudLocal)
	err := m.SetAddresses(addrPub, addrPriv)
	c.Assert(err, gc.IsNil)
}

func (s *SSHCommonSuite) makeMachines(n int, c *gc.C, setAddresses bool) []*state.Machine {
	var machines = make([]*state.Machine, n)
	for i := 0; i < n; i++ {
		m, err := s.State.AddMachine("quantal", state.JobHostUnits)
		c.Assert(err, gc.IsNil)
		if setAddresses {
			s.setAddresses(m, c)
		}
		// must set an instance id as the ssh command uses that as a signal the
		// machine has been provisioned
		inst, md := testing.AssertStartInstance(c, s.Conn.Environ, m.Id())
		c.Assert(m.SetProvisioned(inst.Id(), "fake_nonce", md), gc.IsNil)
		machines[i] = m
	}
	return machines
}

func (s *SSHCommonSuite) addUnit(srv *state.Service, m *state.Machine, c *gc.C) {
	u, err := srv.AddUnit()
	c.Assert(err, gc.IsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, gc.IsNil)
}
