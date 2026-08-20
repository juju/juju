// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"bytes"
	"os"
	"strings"
	stdtesting "testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/ssh/mocks"
	pkissh "github.com/juju/juju/internal/pki/ssh"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type sshJumpSuite struct {
	SSHMachineSuite

	sshAPIJump *mocks.MockSSHAPIJump
	// hostKey is a valid wire-format public host key used for both the jump
	// server and the target in tests.
	hostKey []byte
}

func TestSSHJumpSuite(t *stdtesting.T) {
	tc.Run(t, &sshJumpSuite{})
}

func (s *sshJumpSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.sshAPIJump = mocks.NewMockSSHAPIJump(ctrl)
	key, err := pkissh.MarshalPublicKey([]byte(coretesting.SSHServerHostKey))
	c.Assert(err, tc.ErrorIsNil)
	s.hostKey = key
	return ctrl
}

// TestResolveTarget exercises the jump target selection flow: the virtual
// hostname is resolved via the API, the target/jump host keys are fetched, and
// a reachable controller jump host is selected. The final destination user is
// fixed to ubuntu.
func (s *sshJumpSuite) TestResolveTarget(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.sshAPIJump.EXPECT().VirtualHostname(gomock.Any(), gomock.Any(), gomock.Any()).Return("resolved-target", nil)
	s.sshAPIJump.EXPECT().PublicHostKeyForTarget(gomock.Any(), gomock.Any()).Return(params.PublicSSHHostKeyResult{
		PublicKey: s.hostKey,
	}, nil)

	controllerAddress := "1.0.0.1"
	jump := sshJump{
		jumpUser:             "fred",
		jumpServerHostKey:    s.hostKey,
		sshClient:            s.sshAPIJump,
		controllersAddresses: []string{"1.0.0.1", "1.0.0.2"},
		hostChecker: &fakeHostChecker{
			acceptedAddresses: set.NewStrings("1.0.0.1"),
			acceptedPort:      17022,
		},
		publicKeyRetryStrategy: baseTestingRetryStrategy,
		jumpHostPort:           17022,
	}

	resolved, err := jump.resolveTarget(c.Context(), "test-target")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resolved, tc.DeepEquals, &resolvedTarget{
		user: finalDestinationUser,
		host: "resolved-target",
		via: &resolvedTarget{
			user: "fred",
			host: controllerAddress,
		},
	})

	// A known_hosts file pinning both the jump server and target keys is
	// written.
	c.Assert(jump.knownHostsPath, tc.Not(tc.Equals), "")
	defer func() { _ = os.Remove(jump.knownHostsPath) }()
	contents, err := os.ReadFile(jump.knownHostsPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(contents), tc.Contains, "[1.0.0.1]:17022 ")
	c.Check(string(contents), tc.Contains, "resolved-target ")
}

// TestResolveTargetPassesContainerForCAAS ensures that for CAAS models the
// container name is passed through when resolving the virtual hostname.
func (s *sshJumpSuite) TestResolveTargetPassesContainerForCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	container := "redis"
	s.sshAPIJump.EXPECT().VirtualHostname(gomock.Any(), "test-target", &container).Return("resolved-target", nil)
	s.sshAPIJump.EXPECT().PublicHostKeyForTarget(gomock.Any(), gomock.Any()).Return(params.PublicSSHHostKeyResult{
		PublicKey: s.hostKey,
	}, nil)

	jump := sshJump{
		modelType:            "caas",
		container:            container,
		jumpServerHostKey:    s.hostKey,
		sshClient:            s.sshAPIJump,
		controllersAddresses: []string{"1.0.0.1"},
		hostChecker: &fakeHostChecker{
			acceptedAddresses: set.NewStrings("1.0.0.1"),
			acceptedPort:      17022,
		},
		publicKeyRetryStrategy: baseTestingRetryStrategy,
		jumpHostPort:           17022,
	}

	_, err := jump.resolveTarget(c.Context(), "test-target")
	c.Assert(err, tc.ErrorIsNil)
	if jump.knownHostsPath != "" {
		defer func() { _ = os.Remove(jump.knownHostsPath) }()
	}
}

// TestSSHUsesJumpProxyCommand verifies that the SSH invocation proxies through
// the controller jump server using ssh -W on the configured port as the current
// Juju user, and connects to the final destination as the fixed ubuntu user.
func (s *sshJumpSuite) TestSSHUsesJumpProxyCommand(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	target := &resolvedTarget{
		user: finalDestinationUser,
		host: "resolved-target",
		via: &resolvedTarget{
			// The jump connection authenticates as the Juju user.
			user: "fred",
			host: "1.0.0.1",
		},
	}
	jump := sshJump{jumpHostPort: 17022, knownHostsPath: "/tmp/known_hosts"}

	buffer := bytes.NewBuffer(nil)
	sshCtx := mocks.NewMockContext(ctrl)
	sshCtx.EXPECT().GetStdin().Return(bytes.NewBuffer(nil)).AnyTimes()
	sshCtx.EXPECT().GetStdout().Return(buffer).AnyTimes()
	sshCtx.EXPECT().GetStderr().Return(buffer).AnyTimes()

	err := jump.ssh(sshCtx, false, target)
	c.Assert(err, tc.ErrorIsNil)

	out := buffer.String()
	// The jump box is reached as the current Juju user (fred) with strict host
	// key checking against the pinned known_hosts file.
	c.Check(strings.Contains(out, "-o ProxyCommand ssh -o StrictHostKeyChecking=yes -o UserKnownHostsFile=/tmp/known_hosts -W %h:%p -p 17022 fred@1.0.0.1"), tc.IsTrue)
	// The final destination is reached as the fixed ubuntu user.
	c.Check(strings.Contains(out, "ubuntu@resolved-target"), tc.IsTrue)
}

// TestSSHEnablesPTY ensures that --pty is honoured when the connection is
// proxied through the controller jump server.
func (s *sshJumpSuite) TestSSHEnablesPTY(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	target := &resolvedTarget{
		user: finalDestinationUser,
		host: "resolved-target",
		via: &resolvedTarget{
			user: "fred",
			host: "1.0.0.1",
		},
	}
	jump := sshJump{jumpHostPort: 17022, knownHostsPath: "/tmp/known_hosts"}

	buffer := bytes.NewBuffer(nil)
	sshCtx := mocks.NewMockContext(ctrl)
	sshCtx.EXPECT().GetStdin().Return(bytes.NewBuffer(nil)).AnyTimes()
	sshCtx.EXPECT().GetStdout().Return(buffer).AnyTimes()
	sshCtx.EXPECT().GetStderr().Return(buffer).AnyTimes()

	err := jump.ssh(sshCtx, true, target)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(strings.Contains(buffer.String(), "-t -t"), tc.IsTrue)
}
