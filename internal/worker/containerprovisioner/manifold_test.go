// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerprovisioner_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/containerprovisioner"
)

type containerManifoldSuite struct {
	machine *MockContainerMachine
	getter  *MockContainerMachineGetter
}

var _ = gc.Suite(&containerManifoldSuite{})

func (s *containerManifoldSuite) TestConfigValidateAgentName(c *gc.C) {
	cfg := containerprovisioner.ManifoldConfig{}
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, "empty AgentName not valid")
}

func (s *containerManifoldSuite) TestConfigValidateAPICallerName(c *gc.C) {
	cfg := containerprovisioner.ManifoldConfig{AgentName: "testing"}
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, "empty APICallerName not valid")
}

func (s *containerManifoldSuite) TestConfigValidateLogger(c *gc.C) {
	cfg := containerprovisioner.ManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
	}
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, "nil Logger not valid")
}

func (s *containerManifoldSuite) TestConfigValidateMachineLock(c *gc.C) {
	cfg := containerprovisioner.ManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
		Logger:        loggertesting.WrapCheckLog(c),
	}
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, "missing MachineLock not valid")
}

func (s *containerManifoldSuite) TestConfigValidateContainerType(c *gc.C) {
	cfg := containerprovisioner.ManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
		Logger:        loggertesting.WrapCheckLog(c),
		MachineLock:   &fakeMachineLock{},
	}
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, "missing Container Type not valid")
}

func (s *containerManifoldSuite) TestConfigValidateSuccess(c *gc.C) {
	cfg := containerprovisioner.ManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
		Logger:        loggertesting.WrapCheckLog(c),
		MachineLock:   &fakeMachineLock{},
		ContainerType: instance.LXD,
	}
	err := cfg.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *containerManifoldSuite) TestContainerProvisioningManifold(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	retval := []containerprovisioner.ContainerMachineResult{
		{Machine: s.machine},
	}
	s.getter.EXPECT().Machines(gomock.Any(), []names.MachineTag{tag}).Return(retval, nil)
	s.machine.EXPECT().SupportedContainers(gomock.Any()).Return([]instance.ContainerType{instance.LXD}, true, nil)
	s.machine.EXPECT().Life().Return(life.Alive)
	cfg := containerprovisioner.ManifoldConfig{
		Logger:        loggertesting.WrapCheckLog(c),
		ContainerType: instance.LXD,
	}
	m, err := containerprovisioner.MachineSupportsContainers(cfg, s.getter, tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.NotNil)
}

func (s *containerManifoldSuite) TestContainerProvisioningManifoldContainersNotKnown(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	retval := []containerprovisioner.ContainerMachineResult{
		{Machine: s.machine},
	}
	s.getter.EXPECT().Machines(gomock.Any(), []names.MachineTag{tag}).Return(retval, nil)
	s.machine.EXPECT().SupportedContainers(gomock.Any()).Return(nil, false, nil)
	s.machine.EXPECT().Life().Return(life.Alive)
	cfg := containerprovisioner.ManifoldConfig{
		Logger:        loggertesting.WrapCheckLog(c),
		ContainerType: instance.LXD,
	}
	_, err := containerprovisioner.MachineSupportsContainers(cfg, s.getter, tag)
	c.Assert(err, jc.ErrorIs, errors.NotYetAvailable)
}

func (s *containerManifoldSuite) TestContainerProvisioningManifoldNoContainerSupport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	retval := []containerprovisioner.ContainerMachineResult{
		{Machine: s.machine},
	}
	s.getter.EXPECT().Machines(gomock.Any(), []names.MachineTag{tag}).Return(retval, nil)
	s.machine.EXPECT().SupportedContainers(gomock.Any()).Return(nil, true, nil)
	s.machine.EXPECT().Life().Return(life.Alive)
	cfg := containerprovisioner.ManifoldConfig{
		Logger:        loggertesting.WrapCheckLog(c),
		ContainerType: instance.LXD,
	}
	_, err := containerprovisioner.MachineSupportsContainers(cfg, s.getter, tag)
	c.Assert(err, gc.ErrorMatches, "resource permanently unavailable")
}

func (s *containerManifoldSuite) TestContainerProvisioningManifoldMachineDead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewMachineTag("42")
	retval := []containerprovisioner.ContainerMachineResult{
		{Machine: s.machine},
	}
	s.getter.EXPECT().Machines(gomock.Any(), []names.MachineTag{tag}).Return(retval, nil)
	s.machine.EXPECT().Life().Return(life.Dead)
	cfg := containerprovisioner.ManifoldConfig{
		Logger:        loggertesting.WrapCheckLog(c),
		ContainerType: instance.LXD,
	}
	_, err := containerprovisioner.MachineSupportsContainers(cfg, s.getter, tag)
	c.Assert(err, gc.ErrorMatches, "resource permanently unavailable")
}

func (s *containerManifoldSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.machine = NewMockContainerMachine(ctrl)
	s.getter = NewMockContainerMachineGetter(ctrl)

	return ctrl
}
