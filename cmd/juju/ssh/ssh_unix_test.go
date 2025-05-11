// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !windows

package ssh

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/cmd/juju/ssh/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	jujussh "github.com/juju/juju/internal/network/ssh"
)

type SSHSuite struct {
	SSHMachineSuite
}

var _ = tc.Suite(&SSHSuite{})

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
		about:       "connect to machine 0 with port 222",
		args:        []string{"0", "-p", "222"},
		target:      "0",
		hostChecker: validAddressesWithPort(222, "0.private", "0.public", "0.1.2.3"), // set by setAddresses() and setLinkLayerDevicesAddresses()
		expected: argsSpec{
			hostKeyChecking: "yes",
			knownHosts:      "0",
			argsMatch:       `ubuntu@0.(public|private|1\.2\.3)( -p \d+)?`, // can be any of the 3
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
		expectedErr: `attempt count exceeded: retrieving SSH host keys for "1": keys not found`,
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

func (s *SSHSuite) TestSSHCommand(c *tc.C) {
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
		ssh, app, status := s.setupModel(ctrl, t.expected.withProxy, false, nil, nil, target)
		sshCmd := NewSSHCommandForTest(app, ssh, status, t.hostChecker, isTerminal, baseTestingRetryStrategy, baseTestingRetryStrategy)

		ctx, err := cmdtesting.RunCommand(c, modelcmd.Wrap(sshCmd), t.args...)
		if t.expectedErr != "" {
			c.Check(err, tc.ErrorMatches, t.expectedErr)
		} else {
			c.Check(err, tc.ErrorIsNil)
			c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
			stdout := cmdtesting.Stdout(ctx)
			t.expected.check(c, stdout)
		}
		ctrl.Finish()
	}
}

func (s *SSHSuite) TestSSHCommandModelConfigProxySSH(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ssh, app, status := s.setupModel(ctrl, true, false, nil, nil, "0")
	sshCmd := NewSSHCommandForTest(app, ssh, status, s.hostChecker, nil, baseTestingRetryStrategy, baseTestingRetryStrategy)

	ctx, err := cmdtesting.RunCommand(c, modelcmd.Wrap(sshCmd), "0")
	c.Check(err, tc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
	expectedArgs := argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		withProxy:       true,
		args:            "ubuntu@0.private",
	}
	expectedArgs.check(c, cmdtesting.Stdout(ctx))
}

func (s *SSHSuite) TestSSHCommandModelConfigProxySSHAddressMatch(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ssh, app, status := s.setupModel(ctrl, true, false, nil, nil, "0")
	sshCmd := NewSSHCommandForTest(app, ssh, status, s.hostChecker, nil, baseTestingRetryStrategy, baseTestingRetryStrategy)

	ctx, err := cmdtesting.RunCommand(c, modelcmd.Wrap(sshCmd), "0")
	c.Check(err, tc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")
	expectedArgs := argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		withProxy:       true,
		args:            "ubuntu@0.private",
		argsMatch:       `ubuntu@0.(public|private|1\.2\.3)`, // can be any of the 3 with api v2.
	}
	expectedArgs.check(c, cmdtesting.Stdout(ctx))

}

func (s *SSHSuite) TestSSHWillWorkInUpgrade(c *tc.C) {
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
		c.Check(apiserver.IsMethodAllowedDuringUpgrade("SSHClient", name), tc.IsTrue)
	}
}

func (s *SSHSuite) TestSSHCommandHostAddressRetry(c *tc.C) {
	s.setHostChecker(validAddresses())
	s.testSSHCommandHostAddressRetry(c, false)
}

func (s *SSHSuite) TestSSHCommandHostAddressRetryProxy(c *tc.C) {
	s.setHostChecker(validAddresses())
	s.testSSHCommandHostAddressRetry(c, true)
}

func (s *SSHSuite) testSSHCommandHostAddressRetry(c *tc.C, proxy bool) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Ensure that the ssh command waits for a public (private with proxy=true)
	// address, or the attempt strategy's Done method returns false.
	args := []string{"--proxy=" + fmt.Sprint(proxy), "0"}

	var addr []string
	ssh, app, status := s.setupModel(ctrl, proxy, false, func() []string {
		return addr
	}, nil, "0")
	sshCmd := NewSSHCommandForTest(app, ssh, status, s.hostChecker, nil, baseTestingRetryStrategy, baseTestingRetryStrategy)

	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(sshCmd), args...)
	c.Assert(err, tc.ErrorMatches, `no .+ address\(es\)`)

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

	ssh, app, status = s.setupModel(ctrl, proxy, false, func() []string {
		return addr
	}, nil, "0")
	sshCmd = NewSSHCommandForTest(app, ssh, status, s.hostChecker, nil, baseTestingRetryStrategy, baseTestingRetryStrategy)
	sshCmd.retryStrategy = retryStrategy
	_, err = cmdtesting.RunCommand(c, modelcmd.Wrap(sshCmd), args...)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *SSHSuite) TestMaybeResolveLeaderUnit(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	leaderAPI := mocks.NewMockLeaderAPI(ctrl)
	leaderAPI.EXPECT().Leader(gomock.Any(), "loop").Return("loop/1", nil)

	ldr := leaderResolver{leaderAPI: leaderAPI}
	resolvedUnit, err := ldr.maybeResolveLeaderUnit(context.Background(), "loop/leader")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resolvedUnit, tc.Equals, "loop/1", tc.Commentf("expected leader to resolve to loop/1 for principal application"))
}

func (s *SSHSuite) TestKeyFetchRetries(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	isTerminal := func(stdin interface{}) bool {
		return false
	}

	done := make(chan struct{})
	publicKeyRetry := retry.CallArgs{
		Attempts:    10,
		Delay:       10 * time.Millisecond,
		MaxDelay:    1 * time.Second,
		BackoffFunc: retry.DoubleDelay,
		Clock:       clock.WallClock,
		NotifyFunc: func(lastError error, attempt int) {
			if attempt == 1 {
				close(done)
			}
		},
	}
	keysFunc := func(ctx context.Context, target string) ([]string, error) {
		c.Check(target, tc.Equals, "1")
		select {
		case <-done:
			return []string{
				fmt.Sprintf("dsa-%s", target),
				fmt.Sprintf("rsa-%s", target),
			}, nil
		default:
			return nil, errors.NotFoundf("public keys for %s", target)
		}
	}

	ssh, app, status := s.setupModel(ctrl, false, false, nil, keysFunc, "1")
	cmd := NewSSHCommandForTest(app, ssh, status, validAddresses("1.public"), isTerminal, baseTestingRetryStrategy, publicKeyRetry)

	ctx, err := cmdtesting.RunCommand(c, modelcmd.Wrap(cmd), "1")
	c.Check(err, tc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "")

	select {
	case <-done:
	default:
		c.Fatal("command exited before keys were delay set")
	}
}
