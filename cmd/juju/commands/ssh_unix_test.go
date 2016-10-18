// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package commands

import (
	"fmt"
	"reflect"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	coretesting "github.com/juju/juju/testing"
)

type SSHSuite struct {
	SSHCommonSuite
}

var _ = gc.Suite(&SSHSuite{})

var sshTests = []struct {
	about       string
	args        []string
	expected    argsSpec
	expectedErr string
}{
	{
		about: "connect to machine 0",
		args:  []string{"0"},
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			enablePty:       true,
			args:            "ubuntu@0.public",
		},
	},
	{
		about: "connect to machine 0 and pass extra arguments",
		args:  []string{"0", "uname", "-a"},
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			enablePty:       true,
			args:            "ubuntu@0.public uname -a",
		},
	},
	{
		about: "connect to machine 0 with no pseudo-tty",
		args:  []string{"--pty=false", "0"},
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			enablePty:       false,
			args:            "ubuntu@0.public",
		},
	},
	{
		about:       "connect to machine 1 which has no SSH host keys",
		args:        []string{"1"},
		expectedErr: `retrieving SSH host keys for "1": keys not found`,
	},
	{
		about: "connect to machine 1 which has no SSH host keys, no host key checks",
		args:  []string{"--no-host-key-checks", "1"},
		expected: argsSpec{
			hostKeyChecking: "no",
			knownHosts:      "null",
			enablePty:       true,
			args:            "ubuntu@1.public",
		},
	},
	{
		about: "connect to arbitrary (non-entity) hostname",
		args:  []string{"foo@some.host"},
		expected: argsSpec{
			// In this case, use the user's own known_hosts and own
			// StrictHostKeyChecking config.
			hostKeyChecking: "",
			knownHosts:      "",
			enablePty:       true,
			args:            "foo@some.host",
		},
	},
	{
		about: "connect to unit mysql/0",
		args:  []string{"mysql/0"},
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			enablePty:       true,
			args:            "ubuntu@0.public",
		},
	},
	{
		about: "connect to unit mysql/0 as the mongo user",
		args:  []string{"mongo@mysql/0"},
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			enablePty:       true,
			args:            "mongo@0.public",
		},
	},
	{
		about: "connect to unit mysql/0 and pass extra arguments",
		args:  []string{"mysql/0", "ls", "/"},
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			enablePty:       true,
			args:            "ubuntu@0.public ls /",
		},
	},
	{
		about: "connect to unit mysql/0 with proxy",
		args:  []string{"--proxy=true", "mysql/0"},
		expected: argsSpec{
			hostKeyChecking: "yes",
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
		if t.expectedErr != "" {
			c.Check(err, gc.ErrorMatches, t.expectedErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(coretesting.Stderr(ctx), gc.Equals, "")
			stdout := coretesting.Stdout(ctx)
			t.expected.check(c, stdout)
		}
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
		hostKeyChecking: "yes",
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

func (s *SSHSuite) TestSSHCommandHostAddressRetry(c *gc.C) {
	s.testSSHCommandHostAddressRetry(c, false)
}

func (s *SSHSuite) TestSSHCommandHostAddressRetryProxy(c *gc.C) {
	s.testSSHCommandHostAddressRetry(c, true)
}

func (s *SSHSuite) testSSHCommandHostAddressRetry(c *gc.C, proxy bool) {
	m := s.Factory.MakeMachine(c, nil)
	s.setKeys(c, m)

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
	c.Assert(err, gc.ErrorMatches, "no .+ address")
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
