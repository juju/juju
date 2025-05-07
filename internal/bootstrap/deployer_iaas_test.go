// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

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

var _ = tc.Suite(&deployerIAASSuite{})

func (s *deployerIAASSuite) TestValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	err := cfg.Validate()
	c.Assert(err, tc.IsNil)

	cfg = s.newConfig(c)
	cfg.MachineGetter = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *deployerIAASSuite) TestControllerAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machine.EXPECT().PublicAddress().Return(network.NewSpaceAddress("10.0.0.1"), nil)

	deployer := s.newDeployer(c)
	address, err := deployer.ControllerAddress(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.Equals, "10.0.0.1")
}

func (s *deployerIAASSuite) TestControllerAddressWithNoAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machine.EXPECT().PublicAddress().Return(network.NewSpaceAddress(""), network.NoAddressError("private"))

	deployer := s.newDeployer(c)
	address, err := deployer.ControllerAddress(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.Equals, "")
}

func (s *deployerIAASSuite) TestControllerAddressWithErr(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machine.EXPECT().PublicAddress().Return(network.NewSpaceAddress(""), errors.Errorf("boom"))

	deployer := s.newDeployer(c)
	_, err := deployer.ControllerAddress(context.Background())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *deployerIAASSuite) TestControllerCharmBase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machine.EXPECT().Base().Return(state.Base{
		OS:      "ubuntu",
		Channel: "22.04",
	})

	deployer := s.newDeployer(c)
	base, err := deployer.ControllerCharmBase()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(base, tc.Equals, corebase.MakeDefaultBase("ubuntu", "22.04"))
}

func (s *deployerIAASSuite) TestCompleteProcess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// There should be no expectations, as the CompleteProcess method doesn't
	// call any methods.

	deployer := s.newDeployer(c)
	err := deployer.CompleteProcess(context.Background(), coreunit.Name("controller/0"))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deployerIAASSuite) newDeployer(c *tc.C) *IAASDeployer {
	deployer, err := NewIAASDeployer(s.newConfig(c))
	c.Assert(err, tc.IsNil)
	return deployer
}

func (s *deployerIAASSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.machine = NewMockMachine(ctrl)
	s.machineGetter = NewMockMachineGetter(ctrl)

	s.machineGetter.EXPECT().Machine(agent.BootstrapControllerId).Return(s.machine, nil).AnyTimes()

	return ctrl
}

func (s *deployerIAASSuite) newConfig(c *tc.C) IAASDeployerConfig {
	return IAASDeployerConfig{
		BaseDeployerConfig: s.baseSuite.newConfig(c),
		MachineGetter:      s.machineGetter,
	}
}
