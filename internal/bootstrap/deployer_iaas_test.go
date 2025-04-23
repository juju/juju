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
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	state "github.com/juju/juju/state"
)

type deployerIAASSuite struct {
	baseSuite

	machine       *MockMachine
	machineGetter *MockMachineGetter
}

var _ = gc.Suite(&deployerIAASSuite{})

func (s *deployerIAASSuite) TestValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	err := cfg.Validate()
	c.Assert(err, gc.IsNil)

	cfg = s.newConfig(c)
	cfg.MachineGetter = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *deployerIAASSuite) TestControllerAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machine.EXPECT().PublicAddress().Return(network.NewSpaceAddress("10.0.0.1"), nil)

	deployer := s.newDeployer(c)
	address, err := deployer.ControllerAddress(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "10.0.0.1")
}

func (s *deployerIAASSuite) TestControllerAddressWithNoAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machine.EXPECT().PublicAddress().Return(network.NewSpaceAddress(""), network.NoAddressError("private"))

	deployer := s.newDeployer(c)
	address, err := deployer.ControllerAddress(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "")
}

func (s *deployerIAASSuite) TestControllerAddressWithErr(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machine.EXPECT().PublicAddress().Return(network.NewSpaceAddress(""), errors.Errorf("boom"))

	deployer := s.newDeployer(c)
	_, err := deployer.ControllerAddress(context.Background())
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *deployerIAASSuite) TestControllerCharmBase(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.machine.EXPECT().Base().Return(state.Base{
		OS:      "ubuntu",
		Channel: "22.04",
	})

	deployer := s.newDeployer(c)
	base, err := deployer.ControllerCharmBase()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(base, gc.Equals, corebase.MakeDefaultBase("ubuntu", "22.04"))
}

func (s *deployerIAASSuite) TestCompleteProcess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// There should be no expectations, as the CompleteProcess method doesn't
	// call any methods.

	deployer := s.newDeployer(c)
	err := deployer.CompleteProcess(context.Background(), coreunit.Name("controller/0"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *deployerIAASSuite) newDeployer(c *gc.C) *IAASDeployer {
	deployer, err := NewIAASDeployer(s.newConfig(c))
	c.Assert(err, gc.IsNil)
	return deployer
}

func (s *deployerIAASSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.machine = NewMockMachine(ctrl)
	s.machineGetter = NewMockMachineGetter(ctrl)

	s.machineGetter.EXPECT().Machine(agent.BootstrapControllerId).Return(s.machine, nil).AnyTimes()

	return ctrl
}

func (s *deployerIAASSuite) newConfig(c *gc.C) IAASDeployerConfig {
	return IAASDeployerConfig{
		BaseDeployerConfig: s.baseSuite.newConfig(c),
		MachineGetter:      s.machineGetter,
	}
}
