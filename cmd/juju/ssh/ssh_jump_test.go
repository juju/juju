// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"text/template"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/ssh/mocks"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/pki/test"
	"github.com/juju/juju/rpc/params"
)

type sshJumpSuite struct {
	testing.IsolationSuite
	sshAPIJump *mocks.MockSSHAPIJump
}

var _ = gc.Suite(&sshJumpSuite{})

func (s *sshJumpSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.sshAPIJump = mocks.NewMockSSHAPIJump(ctrl)
	return ctrl
}

func (s *sshJumpSuite) TestResolveTarget(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.sshAPIJump.EXPECT().VirtualHostname(gomock.Any(), gomock.Any()).Return("resolved-target", nil)
	privateKey, err := test.InsecureKeyProfile()
	c.Assert(err, gc.IsNil)
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	c.Assert(err, gc.IsNil)

	s.sshAPIJump.EXPECT().PublicHostKeyForTarget(gomock.Any()).Return(params.PublicSSHHostKeyResult{
		PublicKey:           publicKey.Marshal(),
		JumpServerPublicKey: publicKey.Marshal(),
	}, nil)
	controllerAddress := "1.0.0.1"
	sshJump := sshJump{
		sshClient:            s.sshAPIJump,
		controllersAddresses: []string{"1.0.0.1", "1.0.0.2"},
		hostChecker: &fakeHostChecker{
			acceptedAddresses: set.NewStrings("1.0.0.1"),
			acceptedPort:      17022,
		},
		publicKeyRetryStrategy: baseTestingRetryStrategy,
		jumpHostPort:           17022,
	}

	resolvedTarget, err := sshJump.resolveTarget("test-target")
	c.Check(err, gc.IsNil)
	via := ResolvedTarget{
		user: jumpUser,
		host: controllerAddress,
	}
	c.Check(resolvedTarget, gc.DeepEquals, &ResolvedTarget{
		user: finalDestinationUser,
		host: "resolved-target",
		via:  &via,
	})
}

func (s *sshJumpSuite) TestShowCommandFlag(c *gc.C) {
	defer s.setupMocks(c).Finish()
	template, err := template.New("output").Parse(openSSHTemplate)
	c.Assert(err, gc.IsNil)
	sshJump := sshJump{
		sshClient:            s.sshAPIJump,
		controllersAddresses: []string{"1.0.0.1", "1.0.0.2"},
		hostChecker: &fakeHostChecker{
			acceptedAddresses: set.NewStrings("1.0.0.1"),
			acceptedPort:      17022,
		},
		publicKeyRetryStrategy: baseTestingRetryStrategy,
		jumpHostPort:           17022,
		showCommand:            true,
		outputTemplate:         template,
	}

	tests := []struct {
		name     string
		args     []string
		target   string
		expected string
		isCaaS   bool
	}{
		{
			name:     "ssh command for caas",
			target:   "caas-target",
			expected: `ssh -o "ProxyCommand=ssh -W %h:%p -p 17022 admin@1.0.0.1" ubuntu@virtual-hostname exec sh`,
			isCaaS:   true,
		},
		{
			name:     "ssh command for iaas",
			target:   "iaas-target",
			expected: `ssh -o "ProxyCommand=ssh -W %h:%p -p 17022 admin@1.0.0.1" ubuntu@virtual-hostname `,
			isCaaS:   false,
		},
		{
			name:     "ssh command with specified user",
			target:   "user-test@caas-target",
			expected: `ssh -o "ProxyCommand=ssh -W %h:%p -p 17022 user-test@1.0.0.1" ubuntu@virtual-hostname exec sh`,
			isCaaS:   true,
		},
		{
			name:     "ssh command with specified args",
			target:   "user-test@caas-target",
			args:     []string{"ls"},
			expected: `ssh -o "ProxyCommand=ssh -W %h:%p -p 17022 user-test@1.0.0.1" ubuntu@virtual-hostname ls`,
		},
	}

	s.sshAPIJump.EXPECT().VirtualHostname(gomock.Any(), gomock.Any()).Return("virtual-hostname", nil).Times(len(tests))
	privateKey, err := test.InsecureKeyProfile()
	c.Assert(err, gc.IsNil)
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	c.Assert(err, gc.IsNil)

	s.sshAPIJump.EXPECT().PublicHostKeyForTarget(gomock.Any()).Return(params.PublicSSHHostKeyResult{
		PublicKey:           publicKey.Marshal(),
		JumpServerPublicKey: publicKey.Marshal(),
	}, nil).Times(len(tests))
	for _, test := range tests {
		c.Logf("running test %q", test.name)
		ctx := cmdtesting.Context(c)
		target, err := sshJump.resolveTarget(test.target)
		c.Assert(err, gc.IsNil)
		sshJump.args = test.args
		if test.isCaaS {
			sshJump.modelType = model.CAAS
		} else {
			sshJump.modelType = model.IAAS
		}
		err = sshJump.ssh(ctx, false, target)
		c.Assert(err, gc.IsNil)
		c.Assert(cmdtesting.Stdout(ctx), gc.Equals, test.expected)
	}
}
