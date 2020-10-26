// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/jujud/reboot"
	"github.com/juju/juju/cmd/jujud/reboot/mocks"
	"github.com/juju/juju/environs/instances"
)

type NewRebootSuite struct {
	agentConfig      *mocks.MockAgentConfig
	containerManager *mocks.MockManager
	instance         *mocks.MockInstance
	model            *mocks.MockModel
	rebootWaiter     *mocks.MockRebootWaiter
	service          *mocks.MockService
}

var _ = gc.Suite(&NewRebootSuite{})

func (s *NewRebootSuite) TestExecuteReboot(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectManagerIsInitialized(false, false)
	s.expectHostSeries("focal")
	s.expectListServices()
	s.expectStopDeployedUnits()
	s.expectScheduleAction()

	err := s.newRebootWaiter().ExecuteReboot(params.ShouldReboot)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *NewRebootSuite) TestExecuteRebootWaitForContainers(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectManagerIsInitialized(true, false)
	s.expectManagerIsInitialized(true, false)
	s.expectListContainers()
	s.expectHostSeries("focal")
	s.expectListServices()
	s.expectStopDeployedUnits()
	s.expectScheduleAction()

	err := s.newRebootWaiter().ExecuteReboot(params.ShouldReboot)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *NewRebootSuite) newRebootWaiter() *reboot.Reboot {
	return reboot.NewRebootForTest(s.agentConfig, s.rebootWaiter)
}

func (s *NewRebootSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.agentConfig = mocks.NewMockAgentConfig(ctrl)
	s.containerManager = mocks.NewMockManager(ctrl)
	s.instance = mocks.NewMockInstance(ctrl)
	s.model = mocks.NewMockModel(ctrl)
	s.rebootWaiter = mocks.NewMockRebootWaiter(ctrl)
	s.service = mocks.NewMockService(ctrl)

	s.agentConfig.EXPECT().Model().Return(s.model).AnyTimes()
	s.model.EXPECT().Id().Return("model-uuid").AnyTimes()

	s.rebootWaiter.EXPECT().NewContainerManager(gomock.Any(), gomock.Any()).Return(s.containerManager, nil).AnyTimes()
	return ctrl
}

func (s *NewRebootSuite) expectManagerIsInitialized(lxd, kvm bool) {
	s.containerManager.EXPECT().IsInitialized().Return(lxd)
	s.containerManager.EXPECT().IsInitialized().Return(kvm)
}

func (s *NewRebootSuite) expectHostSeries(name string) {
	s.rebootWaiter.EXPECT().HostSeries().Return(name, nil)
}

func (s *NewRebootSuite) expectListServices() {
	fakeServices := []string{
		"jujud-machine-1",
		"jujud-unit-drupal-1",
		"jujud-unit-mysql-1",
		"fake-random-service",
	}
	s.rebootWaiter.EXPECT().ListServices().Return(fakeServices, nil)
}

func (s *NewRebootSuite) expectStopDeployedUnits() {
	s.rebootWaiter.EXPECT().NewService("jujud-unit-drupal-1", gomock.Any(), gomock.Any()).Return(s.service, nil)
	s.rebootWaiter.EXPECT().NewService("jujud-unit-mysql-1", gomock.Any(), gomock.Any()).Return(s.service, nil)
	s.service.EXPECT().Stop().Times(2)
}

func (s *NewRebootSuite) expectScheduleAction() {
	s.rebootWaiter.EXPECT().ScheduleAction(gomock.Any(), gomock.Any()).Return(nil)
}

func (s *NewRebootSuite) expectListContainers() {
	inst := []instances.Instance{s.instance}
	s.containerManager.EXPECT().ListContainers().Return(inst, nil)
	s.containerManager.EXPECT().ListContainers().Return([]instances.Instance{}, nil)
}
