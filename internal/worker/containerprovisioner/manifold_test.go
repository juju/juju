// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerprovisioner_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/containerprovisioner"
)

type containerManifoldSuite struct {
	machine *MockContainerMachine
	getter  *MockContainerMachineGetter
}

func TestContainerManifoldSuite(t *stdtesting.T) { tc.Run(t, &containerManifoldSuite{}) }
func (s *containerManifoldSuite) TestConfigValidateAgentName(c *tc.C) {
	cfg := containerprovisioner.ManifoldConfig{}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "empty AgentName not valid")
}

func (s *containerManifoldSuite) TestConfigValidateAPICallerName(c *tc.C) {
	cfg := containerprovisioner.ManifoldConfig{AgentName: "testing"}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "empty APICallerName not valid")
}

func (s *containerManifoldSuite) TestConfigValidateLogger(c *tc.C) {
	cfg := containerprovisioner.ManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "nil Logger not valid")
}

func (s *containerManifoldSuite) TestConfigValidateMachineLock(c *tc.C) {
	cfg := containerprovisioner.ManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
		Logger:        loggertesting.WrapCheckLog(c),
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "missing MachineLock not valid")
}

func (s *containerManifoldSuite) TestConfigValidateContainerType(c *tc.C) {
	cfg := containerprovisioner.ManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
		Logger:        loggertesting.WrapCheckLog(c),
		MachineLock:   &fakeMachineLock{},
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, "missing Container Type not valid")
}

func (s *containerManifoldSuite) TestConfigValidateSuccess(c *tc.C) {
	cfg := containerprovisioner.ManifoldConfig{
		AgentName:     "testing",
		APICallerName: "another string",
		Logger:        loggertesting.WrapCheckLog(c),
		MachineLock:   &fakeMachineLock{},
		ContainerType: instance.LXD,
	}
	err := cfg.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *containerManifoldSuite) TestContainerProvisioningManifold(c *tc.C) {
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
	m, err := containerprovisioner.MachineSupportsContainers(c, cfg, s.getter, tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.NotNil)
}

func (s *containerManifoldSuite) TestContainerProvisioningManifoldContainersNotKnown(c *tc.C) {
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
	_, err := containerprovisioner.MachineSupportsContainers(c, cfg, s.getter, tag)
	c.Assert(err, tc.ErrorIs, errors.NotYetAvailable)
}

func (s *containerManifoldSuite) TestContainerProvisioningManifoldNoContainerSupport(c *tc.C) {
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
	_, err := containerprovisioner.MachineSupportsContainers(c, cfg, s.getter, tag)
	c.Assert(err, tc.ErrorMatches, "resource permanently unavailable")
}

func (s *containerManifoldSuite) TestContainerProvisioningManifoldMachineDead(c *tc.C) {
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
	_, err := containerprovisioner.MachineSupportsContainers(c, cfg, s.getter, tag)
	c.Assert(err, tc.ErrorMatches, "resource permanently unavailable")
}

func (s *containerManifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.machine = NewMockContainerMachine(ctrl)
	s.getter = NewMockContainerMachineGetter(ctrl)

	return ctrl
}
