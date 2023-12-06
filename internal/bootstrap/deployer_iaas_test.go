// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
)

type deployerIAASSuite struct {
	baseSuite

	machineGetter *MockMachineGetter
}

var _ = gc.Suite(&deployerIAASSuite{})

func (s *deployerIAASSuite) TestValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig()
	err := cfg.Validate()
	c.Assert(err, gc.IsNil)

	cfg = s.newConfig()
	cfg.MachineGetter = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *deployerIAASSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.machineGetter = NewMockMachineGetter(ctrl)

	return ctrl
}

func (s *deployerIAASSuite) newConfig() IAASDeployerConfig {
	return IAASDeployerConfig{
		BaseDeployerConfig: s.baseSuite.newConfig(),
		MachineGetter:      s.machineGetter,
	}
}

func (s *deployerIAASSuite) TestControllerAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machineGetter.EXPECT().Machine(agent.BootstrapControllerId).Return(s.machine, nil)

	deployer := s.newDeployer(c)
	address, err := deployer.ControllerAddress(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "")
}

func (s *deployerIAASSuite) newDeployer(c *gc.C) *IAASDeployer {
	deployer, err := NewIAASDeployer(s.newConfig())
	c.Assert(err, gc.IsNil)
	return deployer
}
