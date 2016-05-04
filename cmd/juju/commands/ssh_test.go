// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"reflect"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&SSHSuite{})

type SSHSuite struct {
	SSHCommonSuite
}

var sshTests = []struct {
	about    string
	args     []string
	expected argsSpec
}{
	{
		"connect to machine 0",
		[]string{"0"},
		argsSpec{
			hostKeyChecking: true,
			knownHosts:      "0",
			enablePty:       true,
			args:            "ubuntu@0.public",
		},
	},
	{
		"connect to machine 0 and pass extra arguments",
		[]string{"0", "uname", "-a"},
		argsSpec{
			hostKeyChecking: true,
			knownHosts:      "0",
			enablePty:       true,
			args:            "ubuntu@0.public uname -a",
		},
	},
	{
		"connect to machine 0 with no pseudo-tty",
		[]string{"--pty=false", "0"},
		argsSpec{
			hostKeyChecking: true,
			knownHosts:      "0",
			enablePty:       false,
			args:            "ubuntu@0.public",
		},
	},
	{
		"connect to machine 1 which has no SSH host keys",
		[]string{"1"},
		argsSpec{
			hostKeyChecking: false,
			knownHosts:      "null",
			enablePty:       true,
			args:            "ubuntu@1.public",
		},
	},
	{
		"connect to unit mysql/0",
		[]string{"mysql/0"},
		argsSpec{
			hostKeyChecking: true,
			knownHosts:      "0",
			enablePty:       true,
			args:            "ubuntu@0.public",
		},
	},
	{
		"connect to unit mysql/0 as the mongo user",
		[]string{"mongo@mysql/0"},
		argsSpec{
			hostKeyChecking: true,
			knownHosts:      "0",
			enablePty:       true,
			args:            "mongo@0.public",
		},
	},
	{
		"connect to unit mysql/0 and pass extra arguments",
		[]string{"mysql/0", "ls", "/"},
		argsSpec{
			hostKeyChecking: true,
			knownHosts:      "0",
			enablePty:       true,
			args:            "ubuntu@0.public ls /",
		},
	},
	{
		"connect to unit mysql/0 with proxy",
		[]string{"--proxy=true", "mysql/0"},
		argsSpec{
			hostKeyChecking: true,
			knownHosts:      "0",
			enablePty:       true,
			withProxy:       true,
			args:            "ubuntu@0.private",
		},
	},
}

func (s *SSHSuite) TestSSHCommand(c *gc.C) {
	s.setupModel(c)

	for i, t := range sshTests {
		c.Logf("test %d: %s -> %s", i, t.about, t.args)

		ctx, err := coretesting.RunCommand(c, newSSHCommand(), t.args...)
		c.Check(err, jc.ErrorIsNil)
		c.Check(coretesting.Stderr(ctx), gc.Equals, "")
		stdout := coretesting.Stdout(ctx)
		t.expected.check(c, stdout)
	}
}

func (s *SSHSuite) TestSSHCommandModelConfigProxySSH(c *gc.C) {
	s.setupModel(c)

	// Setting proxy-ssh=true in the environment overrides --proxy.
	err := s.State.UpdateModelConfig(map[string]interface{}{"proxy-ssh": true}, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	ctx, err := coretesting.RunCommand(c, newSSHCommand(), "0")
	c.Check(err, jc.ErrorIsNil)
	c.Check(coretesting.Stderr(ctx), gc.Equals, "")
	expectedArgs := argsSpec{
		hostKeyChecking: true,
		knownHosts:      "0",
		enablePty:       true,
		withProxy:       true,
		args:            "ubuntu@0.private",
	}
	expectedArgs.check(c, coretesting.Stdout(ctx))
}

func (s *SSHSuite) TestSSHWillWorkInUpgrade(c *gc.C) {
	// Check the API client interface used by "juju ssh" against what
	// the API server will allow during upgrades. Ensure that the API
	// server will allow all required API calls to support SSH.
	type concrete struct {
		sshAPIClient
	}
	t := reflect.TypeOf(concrete{})
	for i := 0; i < t.NumMethod(); i++ {
		name := t.Method(i).Name

		// Close isn't an API method.
		if name == "Close" {
			continue
		}
		c.Logf("checking %q", name)
		c.Check(apiserver.IsMethodAllowedDuringUpgrade("SSHClient", name), jc.IsTrue)
	}
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
	m := s.Factory.MakeMachine(c, nil)

	called := 0
	attemptStarter := &callbackAttemptStarter{next: func() bool {
		called++
		return called < 2
	}}
	s.PatchValue(&sshHostFromTargetAttemptStrategy, attemptStarter)

	// Ensure that the ssh command waits for a public address, or the attempt
	// strategy's Done method returns false.
	args := []string{"--proxy=" + fmt.Sprint(proxy), "0"}
	_, err := coretesting.RunCommand(c, newSSHCommand(), args...)
	c.Assert(err, gc.ErrorMatches, ".+ no address")
	c.Assert(called, gc.Equals, 2)

	called = 0
	attemptStarter.next = func() bool {
		called++
		if called > 1 {
			s.setAddresses(c, m)
		}
		return true
	}

	_, err = coretesting.RunCommand(c, newSSHCommand(), args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, gc.Equals, 2)
}
