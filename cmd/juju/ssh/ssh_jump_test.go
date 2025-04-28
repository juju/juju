// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"github.com/juju/collections/set"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/ssh/mocks"
	"github.com/juju/juju/rpc/params"
)

type sshJumpSuite struct {
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
	s.sshAPIJump.EXPECT().PublicHostKeyForTarget(gomock.Any()).Return(params.PublicSSHHostKeyResult{
		PublicKey: []byte("host-key"),
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
