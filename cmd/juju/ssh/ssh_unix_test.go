// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows

package ssh

import (
	"fmt"
	"reflect"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/cmd/juju/ssh/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	jujussh "github.com/juju/juju/network/ssh"
)

type SSHSuite struct {
	SSHMachineSuite
}

var _ = gc.Suite(&SSHSuite{})

var sshTests = []struct {
	about       string
	args        []string
	target      string
	hostChecker jujussh.ReachableChecker
	isTerminal  bool
	expected    argsSpec
	expectedErr string
}{
	{
		about:       "connect to machine 0",
		args:        []string{"0"},
		hostChecker: validAddresses("0.private", "0.public", "0.1.2.3"), // set by setAddresses() and setLinkLayerDevicesAddresses()
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			argsMatch:       `ubuntu@0.(public|private|1\.2\.3)`, // can be any of the 3
		},
	},
	{
		about:       "connect to machine 0 and pass extra arguments",
		args:        []string{"0", "uname", "-a"},
		target:      "0",
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			args:            "ubuntu@0.public uname -a",
		},
	},
	{
		about:       "connect to machine 0 with implied pseudo-tty",
		args:        []string{"0"},
		hostChecker: validAddresses("0.public"),
		isTerminal:  true,
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			enablePty:       true, // implied by client's terminal
			args:            "ubuntu@0.public",
		},
	},
	{
		about:       "connect to machine 0 with pseudo-tty",
		args:        []string{"--pty=true", "0"},
		target:      "0",
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			enablePty:       true,
			args:            "ubuntu@0.public",
		},
	},
	{
		about:       "connect to machine 0 without pseudo-tty",
		args:        []string{"--pty=false", "0"},
		target:      "0",
		hostChecker: validAddresses("0.public"),
		isTerminal:  true,
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			enablePty:       false, // explicitly disabled
			args:            "ubuntu@0.public",
		},
	},
	{
		about:       "connect to machine 1 which has no SSH host keys",
		args:        []string{"1"},
		hostChecker: validAddresses("1.public"),
		expectedErr: `retrieving SSH host keys for "1": keys not found`,
	},
	{
		about:       "connect to machine 1 which has no SSH host keys, no host key checks",
		args:        []string{"--no-host-key-checks", "1"},
		target:      "1",
		hostChecker: validAddresses("1.public"),
		expected: argsSpec{
			hostKeyChecking: "no",
			knownHosts:      "null",
			args:            "ubuntu@1.public",
		},
	},
	{
		about:       "connect to arbitrary (non-entity) hostname",
		args:        []string{"foo@some.host"},
		hostChecker: validAddresses("some.host"),
		expected: argsSpec{
			// In this case, use the user's own known_hosts and own
			// StrictHostKeyChecking config.
			hostKeyChecking: "",
			knownHosts:      "",
			args:            "foo@some.host",
		},
	},
	{
		about:       "connect to unit mysql/0",
		args:        []string{"mysql/0"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			args:            "ubuntu@0.public",
		},
	},
	{
		about:       "connect to unit mysql/0 as the mongo user",
		args:        []string{"mongo@mysql/0"},
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			args:            "mongo@0.public",
		},
	},
	{
		about:       "connect to unit mysql/0 and pass extra arguments",
		args:        []string{"mysql/0", "ls", "/"},
		target:      "mysql/0",
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			args:            "ubuntu@0.public ls /",
		},
	},
	{
		about:       "connect to unit mysql/0 with proxy",
		args:        []string{"--proxy=true", "mysql/0"},
		target:      "mysql/0",
		hostChecker: nil, // Host checker shouldn't get used with --proxy=true
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			withProxy:       true,
			argsMatch:       `ubuntu@0.private`,
		},
	},
}

func (s *SSHSuite) TestSSHCommand(c *gc.C) {
	for i, t := range sshTests {
		c.Logf("test %d: %s -> %s", i, t.about, t.args)

		isTerminal := func(stdin interface{}) bool {
			return t.isTerminal
		}
		target := t.args[0]
		if len(t.args) > 1 {
			target = t.target
		}
		ctrl := gomock.NewController(c)
		ssh, app, status := s.setupModel(ctrl, t.expected.withProxy, nil, target)
		sshCmd := NewSSHCommandForTest(app, ssh, status, t.hostChecker, isTerminal, baseTestingRetryStrategy)

		ctx, err := cmdtesting.RunCommand(c, modelcmd.Wrap(sshCmd), t.args...)
		if t.expectedErr != "" {
			c.Check(err, gc.ErrorMatches, t.expectedErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
			stdout := cmdtesting.Stdout(ctx)
			t.expected.check(c, stdout)
		}
		ctrl.Finish()
	}
}

func (s *SSHSuite) TestSSHCommandModelConfigProxySSH(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ssh, app, status := s.setupModel(ctrl, true, nil, "0")
	sshCmd := NewSSHCommandForTest(app, ssh, status, s.hostChecker, nil, baseTestingRetryStrategy)

	ctx, err := cmdtesting.RunCommand(c, modelcmd.Wrap(sshCmd), "0")
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	expectedArgs := argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		withProxy:       true,
		args:            "ubuntu@0.private",
	}
	expectedArgs.check(c, cmdtesting.Stdout(ctx))
}

func (s *SSHSuite) TestSSHCommandModelConfigProxySSHAddressMatch(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ssh, app, status := s.setupModel(ctrl, true, nil, "0")
	sshCmd := NewSSHCommandForTest(app, ssh, status, s.hostChecker, nil, baseTestingRetryStrategy)

	ctx, err := cmdtesting.RunCommand(c, modelcmd.Wrap(sshCmd), "0")
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	expectedArgs := argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		withProxy:       true,
		args:            "ubuntu@0.private",
		argsMatch:       `ubuntu@0.(public|private|1\.2\.3)`, // can be any of the 3 with api v2.
	}
	expectedArgs.check(c, cmdtesting.Stdout(ctx))

}

func (s *SSHSuite) TestSSHWillWorkInUpgrade(c *gc.C) {
	// Check the API client interface used by "juju ssh" against what
	// the API server will allow during upgrades. Ensure that the API
	// server will allow all required API calls to support SSH.
	type concrete struct {
		coreSSHClient
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
	s.setHostChecker(validAddresses())
	s.testSSHCommandHostAddressRetry(c, false)
}

func (s *SSHSuite) TestSSHCommandHostAddressRetryProxy(c *gc.C) {
	s.setHostChecker(validAddresses())
	s.testSSHCommandHostAddressRetry(c, true)
}

func (s *SSHSuite) testSSHCommandHostAddressRetry(c *gc.C, proxy bool) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Ensure that the ssh command waits for a public (private with proxy=true)
	// address, or the attempt strategy's Done method returns false.
	args := []string{"--proxy=" + fmt.Sprint(proxy), "0"}

	var addr []string
	ssh, app, status := s.setupModel(ctrl, proxy, func() []string {
		return addr
	}, "0")
	sshCmd := NewSSHCommandForTest(app, ssh, status, s.hostChecker, nil, baseTestingRetryStrategy)

	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(sshCmd), args...)
	c.Assert(err, gc.ErrorMatches, `no .+ address\(es\)`)

	if proxy {
		s.setHostChecker(nil) // not used when proxy=true
	} else {
		s.setHostChecker(validAddresses("0.private", "0.public"))
	}

	retryStrategy := baseTestingRetryStrategy
	retryStrategy.NotifyFunc = func(lastError error, attempt int) {
		if attempt > 1 {
			addr = []string{"0.private", "0.public"}
		}
	}

	ssh, app, status = s.setupModel(ctrl, proxy, func() []string {
		return addr
	}, "0")
	sshCmd = NewSSHCommandForTest(app, ssh, status, s.hostChecker, nil, baseTestingRetryStrategy)
	sshCmd.retryStrategy = retryStrategy
	_, err = cmdtesting.RunCommand(c, modelcmd.Wrap(sshCmd), args...)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SSHSuite) TestMaybeResolveLeaderUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	leaderAPI := mocks.NewMockLeaderAPI(ctrl)
	leaderAPI.EXPECT().Leader("loop").Return("loop/1", nil)

	ldr := leaderResolver{leaderAPI: leaderAPI}
	resolvedUnit, err := ldr.maybeResolveLeaderUnit("loop/leader")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resolvedUnit, gc.Equals, "loop/1", gc.Commentf("expected leader to resolve to loop/1 for principal application"))
}
