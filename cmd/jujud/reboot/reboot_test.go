// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	"os"
	"path/filepath"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	ft "github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/reboot"
	"github.com/juju/juju/cmd/jujud/reboot/mocks"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/rpc/params"
	jujutesting "github.com/juju/juju/testing"
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

// on linux we use the "nohup" command to run a reboot
var rebootBin = "nohup"

var expectedRebootScript = `#!/bin/bash
sleep 15
shutdown -r now`

var expectedShutdownScript = `#!/bin/bash
sleep 15
shutdown -h now`

type NixRebootSuite struct {
	jujutesting.BaseSuite
	tmpDir           string
	rebootScriptName string
}

var _ = gc.Suite(&NixRebootSuite{})

func (s *NixRebootSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	testing.PatchExecutableAsEchoArgs(c, s, rebootBin)
	s.tmpDir = c.MkDir()
	s.rebootScriptName = "juju-reboot-script"
	s.PatchValue(reboot.TmpFile, func() (*os.File, error) {
		script := s.rebootScript()
		return os.Create(script)
	})
}

func (s *NixRebootSuite) TestReboot(c *gc.C) {
	expectedParams := s.commandParams()
	err := reboot.ScheduleAction(params.ShouldReboot, 15)
	c.Assert(err, jc.ErrorIsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedParams...)
	ft.File{s.rebootScriptName, expectedRebootScript, 0755}.Check(c, s.tmpDir)
}

func (s *NixRebootSuite) TestShutdownNoContainers(c *gc.C) {
	expectedParams := s.commandParams()

	err := reboot.ScheduleAction(params.ShouldShutdown, 15)
	c.Assert(err, jc.ErrorIsNil)
	testing.AssertEchoArgs(c, rebootBin, expectedParams...)
	ft.File{s.rebootScriptName, expectedShutdownScript, 0755}.Check(c, s.tmpDir)
}

func (s *NixRebootSuite) rebootScript() string {
	return filepath.Join(s.tmpDir, s.rebootScriptName)
}

func (s *NixRebootSuite) commandParams() []string {
	return []string{
		"sh",
		s.rebootScript(),
		"&",
	}
}
