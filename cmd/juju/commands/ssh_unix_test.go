// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows
// +build !windows

package commands

import (
	"fmt"
	"reflect"

	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/cmd/juju/commands/mocks"
	jujussh "github.com/juju/juju/network/ssh"
	"github.com/juju/juju/rpc/params"
)

type SSHSuite struct {
	SSHMachineSuite
}

var _ = gc.Suite(&SSHSuite{})

var sshTests = []struct {
	about       string
	args        []string
	hostChecker jujussh.ReachableChecker
	isTerminal  bool
	forceAPIv1  bool
	expected    argsSpec
	expectedErr string
}{
	{
		about:       "connect to machine 0 (api v1)",
		args:        []string{"0"},
		hostChecker: validAddresses("0.private", "0.public"),
		forceAPIv1:  true,
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			args:            "ubuntu@0.public",
		},
	},
	{
		about:       "connect to machine 0 (api v2)",
		args:        []string{"0"},
		hostChecker: validAddresses("0.private", "0.public", "0.1.2.3"), // set by setAddresses() and setLinkLayerDevicesAddresses()
		forceAPIv1:  false,
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			argsMatch:       `ubuntu@0.(public|private|1\.2\.3)`, // can be any of the 3
		},
	},
	{
		about:       "connect to machine 0 and pass extra arguments",
		args:        []string{"0", "uname", "-a"},
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
		expectedErr: `attempt count exceeded: retrieving SSH host keys for "1": keys not found`,
	},
	{
		about:       "connect to machine 1 which has no SSH host keys, no host key checks",
		args:        []string{"--no-host-key-checks", "1"},
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
		hostChecker: validAddresses("0.public"),
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			args:            "ubuntu@0.public ls /",
		},
	},
	{
		about:       "connect to unit mysql/0 with proxy (api v1)",
		args:        []string{"--proxy=true", "mysql/0"},
		hostChecker: nil, // Host checker shouldn't get used with --proxy=true
		forceAPIv1:  true,
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			withProxy:       true,
			args:            "ubuntu@0.private",
		},
	},
	{
		about:       "connect to unit mysql/0 with proxy (api v2)",
		args:        []string{"--proxy=true", "mysql/0"},
		hostChecker: nil, // Host checker shouldn't get used with --proxy=true
		forceAPIv1:  false,
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			withProxy:       true,
			argsMatch:       `ubuntu@0.private`,
		},
	},
}

func (s *SSHSuite) TestSSHCommand(c *gc.C) {
	s.setupModel(c)

	for i, t := range sshTests {
		c.Logf("test %d: %s -> %s", i, t.about, t.args)

		s.setForceAPIv1(t.forceAPIv1)

		isTerminal := func(stdin interface{}) bool {
			return t.isTerminal
		}
		cmd := newSSHCommand(t.hostChecker, isTerminal, baseTestingRetryStrategy, baseTestingRetryStrategy)

		ctx, err := cmdtesting.RunCommand(c, cmd, t.args...)
		if t.expectedErr != "" {
			c.Check(err, gc.ErrorMatches, t.expectedErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
			stdout := cmdtesting.Stdout(ctx)
			t.expected.check(c, stdout)
		}
	}
}

func (s *SSHSuite) TestSSHCommandModelConfigProxySSH(c *gc.C) {
	s.setupModel(c)

	// Setting proxy-ssh=true in the environment overrides --proxy.
	err := s.Model.UpdateModelConfig(map[string]interface{}{"proxy-ssh": true}, nil)
	c.Assert(err, jc.ErrorIsNil)

	s.setForceAPIv1(true)
	ctx, err := cmdtesting.RunCommand(c, newSSHCommand(s.hostChecker, nil, baseTestingRetryStrategy, baseTestingRetryStrategy), "0")
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	expectedArgs := argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		withProxy:       true,
		args:            "ubuntu@0.private", // as set by setAddresses()
	}
	expectedArgs.check(c, cmdtesting.Stdout(ctx))

	s.setForceAPIv1(false)
	ctx, err = cmdtesting.RunCommand(c, newSSHCommand(s.hostChecker, nil, baseTestingRetryStrategy, baseTestingRetryStrategy), "0")
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "")
	expectedArgs.argsMatch = `ubuntu@0.(public|private|1\.2\.3)` // can be any of the 3 with api v2.
	expectedArgs.check(c, cmdtesting.Stdout(ctx))

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

// / XXX(jam): 2017-01-25 do we need these functions anymore? We don't really
// support ssh'ing to V1 anymore
func (s *SSHSuite) TestSSHCommandHostAddressRetryAPIv1(c *gc.C) {
	// Start with nothing valid to connect to.
	s.setHostChecker(validAddresses())
	s.setForceAPIv1(true)

	s.testSSHCommandHostAddressRetry(c, false)
}

func (s *SSHSuite) TestSSHCommandHostAddressRetryAPIv2(c *gc.C) {
	s.setHostChecker(validAddresses())
	s.setForceAPIv1(false)

	s.testSSHCommandHostAddressRetry(c, false)
}

func (s *SSHSuite) TestSSHCommandHostAddressRetryProxyAPIv1(c *gc.C) {
	s.setHostChecker(validAddresses())
	s.setForceAPIv1(true)

	s.testSSHCommandHostAddressRetry(c, true)
}

func (s *SSHSuite) TestSSHCommandHostAddressRetryProxyAPIv2(c *gc.C) {
	s.setHostChecker(validAddresses())
	s.setForceAPIv1(false)

	s.testSSHCommandHostAddressRetry(c, true)
}

func (s *SSHSuite) testSSHCommandHostAddressRetry(c *gc.C, proxy bool) {
	m := s.Factory.MakeMachine(c, nil)
	s.setKeys(c, m)

	// Ensure that the ssh command waits for a public (private with proxy=true)
	// address, or the attempt strategy's Done method returns false.
	args := []string{"--proxy=" + fmt.Sprint(proxy), "0"}
	_, err := cmdtesting.RunCommand(c, newSSHCommand(s.hostChecker, nil, baseTestingRetryStrategy, baseTestingRetryStrategy), args...)
	c.Assert(err, gc.ErrorMatches, `no .+ address\(es\)`)

	if proxy {
		s.setHostChecker(nil) // not used when proxy=true
	} else {
		s.setHostChecker(validAddresses("0.private", "0.public"))
	}

	retryStrategy := baseTestingRetryStrategy
	retryStrategy.NotifyFunc = func(lastError error, attempt int) {
		if attempt > 1 {
			s.setAddresses(c, m)
		}
	}

	_, err = cmdtesting.RunCommand(c, newSSHCommand(s.hostChecker, nil, retryStrategy, baseTestingRetryStrategy), args...)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SSHSuite) TestMaybeResolveLeaderUnitFromFullStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	statusAPI := mocks.NewMockStatusAPI(ctrl)
	statusAPI.EXPECT().Close().AnyTimes().Return(nil)
	statusAPI.EXPECT().Status(gomock.Any()).AnyTimes().Return(&params.FullStatus{
		Applications: map[string]params.ApplicationStatus{
			"loop": {
				Units: map[string]params.UnitStatus{
					"loop/0": {
						Subordinates: map[string]params.UnitStatus{
							"wormhole/0": {},
						},
					},
					"loop/1": {
						Leader: true,
						Subordinates: map[string]params.UnitStatus{
							"wormhole/1": {Leader: true},
						},
					},
				},
			},
		},
	}, nil)
	statusFunc := func() (StatusAPI, error) { return statusAPI, nil }

	leaderAPI := mocks.NewMockLeaderAPI(ctrl)
	leaderAPI.EXPECT().BestAPIVersion().Return(13).AnyTimes()
	leaderFunc := func() (LeaderAPI, bool, error) { return leaderAPI, false, nil }

	// Resolve principal application leader.
	ldr := leaderResolver{}
	resolvedUnit, err := ldr.maybeResolveLeaderUnit(leaderFunc, statusFunc, "loop/leader")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resolvedUnit, gc.Equals, "loop/1", gc.Commentf("expected leader to resolve to loop/1 for principal application"))

	// Resolve subordinate application leader.
	ldr.resolvedLeader = ""
	resolvedUnit, err = ldr.maybeResolveLeaderUnit(leaderFunc, statusFunc, "wormhole/leader")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resolvedUnit, gc.Equals, "wormhole/1", gc.Commentf("expected leader to resolve to wormhole/1 for subordinate application"))
}

func (s *SSHSuite) TestMaybeResolveLeaderUnitFromLeader(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	statusAPI := mocks.NewMockStatusAPI(ctrl)
	statusFunc := func() (StatusAPI, error) { return statusAPI, nil }

	leaderAPI := mocks.NewMockLeaderAPI(ctrl)
	leaderAPI.EXPECT().BestAPIVersion().Return(14).AnyTimes()
	leaderAPI.EXPECT().Leader("loop").Return("loop/1", nil)
	leaderFunc := func() (LeaderAPI, bool, error) { return leaderAPI, true, nil }

	ldr := leaderResolver{}
	resolvedUnit, err := ldr.maybeResolveLeaderUnit(leaderFunc, statusFunc, "loop/leader")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resolvedUnit, gc.Equals, "loop/1", gc.Commentf("expected leader to resolve to loop/1 for principal application"))
}
