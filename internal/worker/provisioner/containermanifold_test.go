// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/provisioner"
	"github.com/juju/juju/internal/worker/provisioner/mocks"
)

type containerManifoldSuite struct {
	machine *mocks.MockContainerMachine
	getter  *mocks.MockContainerMachineGetter
}

var _ = gc.Suite(&containerManifoldSuite{})

func (s *containerManifoldSuite) TestConfigValidateAgentName(c *gc.C) {
	cfg := provisioner.ContainerManifoldConfig{}
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, "empty AgentName not valid")
}

func (s *containerManifoldSuite) TestConfigValidateAPICallerName(c *gc.C) {
	cfg := provisioner.ContainerManifoldConfig{AgentName: "testing"}
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, "empty APICallerName not valid")
}

func (s *containerManifoldSuite) TestConfigValidateLogger(c *gc.C) {
	cfg := provisioner.ContainerManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
	}
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, "nil Logger not valid")
}

func (s *containerManifoldSuite) TestConfigValidateMachineLock(c *gc.C) {
	cfg := provisioner.ContainerManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
		Logger:        &noOpLogger{},
	}
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, "missing MachineLock not valid")
}

func (s *containerManifoldSuite) TestConfigValidateCredentialValidatorFacade(c *gc.C) {
	cfg := provisioner.ContainerManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
		Logger:        &noOpLogger{},
		MachineLock:   &fakeMachineLock{},
	}
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, "missing NewCredentialValidatorFacade not valid")
}

func (s *containerManifoldSuite) TestConfigValidateContainerType(c *gc.C) {
	cfg := provisioner.ContainerManifoldConfig{
		AgentName:                    "testing",
		APICallerName:                "another string",
		Logger:                       &noOpLogger{},
		MachineLock:                  &fakeMachineLock{},
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) { return nil, nil },
	}
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, "missing Container Type not valid")
}

func (s *containerManifoldSuite) TestConfigValidateSuccess(c *gc.C) {
	cfg := provisioner.ContainerManifoldConfig{
		AgentName:                    "testing",
		APICallerName:                "another string",
		Logger:                       &noOpLogger{},
		MachineLock:                  &fakeMachineLock{},
		NewCredentialValidatorFacade: func(base.APICaller) (common.CredentialAPI, error) { return nil, nil },
		ContainerType:                instance.LXD,
	}
	err := cfg.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *containerManifoldSuite) TestContainerProvisioningManifold(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	retval := []provisioner.ContainerMachineResult{
		{Machine: s.machine},
	}
	s.getter.EXPECT().Machines([]names.MachineTag{tag}).Return(retval, nil)
	s.machine.EXPECT().SupportedContainers().Return([]instance.ContainerType{instance.LXD}, true, nil)
	s.machine.EXPECT().Life().Return(life.Alive)
	cfg := provisioner.ContainerManifoldConfig{
		Logger:        &noOpLogger{},
		ContainerType: instance.LXD,
	}
	m, err := provisioner.MachineSupportsContainers(cfg, s.getter, tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.NotNil)
}

func (s *containerManifoldSuite) TestContainerProvisioningManifoldContainersNotKnown(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	retval := []provisioner.ContainerMachineResult{
		{Machine: s.machine},
	}
	s.getter.EXPECT().Machines([]names.MachineTag{tag}).Return(retval, nil)
	s.machine.EXPECT().SupportedContainers().Return(nil, false, nil)
	s.machine.EXPECT().Life().Return(life.Alive)
	cfg := provisioner.ContainerManifoldConfig{
		Logger:        &noOpLogger{},
		ContainerType: instance.LXD,
	}
	_, err := provisioner.MachineSupportsContainers(cfg, s.getter, tag)
	c.Assert(err, jc.ErrorIs, errors.NotYetAvailable)
}

func (s *containerManifoldSuite) TestContainerProvisioningManifoldNoContainerSupport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	retval := []provisioner.ContainerMachineResult{
		{Machine: s.machine},
	}
	s.getter.EXPECT().Machines([]names.MachineTag{tag}).Return(retval, nil)
	s.machine.EXPECT().SupportedContainers().Return(nil, true, nil)
	s.machine.EXPECT().Life().Return(life.Alive)
	cfg := provisioner.ContainerManifoldConfig{
		Logger:        &noOpLogger{},
		ContainerType: instance.LXD,
	}
	_, err := provisioner.MachineSupportsContainers(cfg, s.getter, tag)
	c.Assert(err, gc.ErrorMatches, "resource permanently unavailable")
}

func (s *containerManifoldSuite) TestContainerProvisioningManifoldMachineDead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	retval := []provisioner.ContainerMachineResult{
		{Machine: s.machine},
	}
	s.getter.EXPECT().Machines([]names.MachineTag{tag}).Return(retval, nil)
	s.machine.EXPECT().Life().Return(life.Dead)
	cfg := provisioner.ContainerManifoldConfig{
		Logger:        &noOpLogger{},
		ContainerType: instance.LXD,
	}
	_, err := provisioner.MachineSupportsContainers(cfg, s.getter, tag)
	c.Assert(err, gc.ErrorMatches, "resource permanently unavailable")
}

func (s *containerManifoldSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.machine = mocks.NewMockContainerMachine(ctrl)
	s.getter = mocks.NewMockContainerMachineGetter(ctrl)

	return ctrl
}
