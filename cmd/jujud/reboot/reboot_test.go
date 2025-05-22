// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	"os"
	"path/filepath"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/jujud/reboot"
	"github.com/juju/juju/cmd/jujud/reboot/mocks"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testhelpers/filetesting"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type NewRebootSuite struct {
	agentConfig      *mocks.MockAgentConfig
	containerManager *mocks.MockManager
	instance         *mocks.MockInstance
	model            *mocks.MockModel
	rebootWaiter     *mocks.MockRebootWaiter
	service          *mocks.MockService
	clock            *mocks.MockClock
}

func TestNewRebootSuite(t *stdtesting.T) {
	tc.Run(t, &NewRebootSuite{})
}

func (s *NewRebootSuite) TestExecuteReboot(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectManagerIsInitialized(false, 1)
	s.expectListServices()
	s.expectStopDeployedUnits()
	s.expectScheduleAction()

	err := s.newRebootWaiter().ExecuteReboot(params.ShouldReboot)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *NewRebootSuite) TestExecuteRebootWaitForContainers(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectManagerIsInitialized(true, 2)
	s.expectListContainers()
	s.expectListServices()
	s.expectStopDeployedUnits()
	s.expectScheduleAction()

	err := s.newRebootWaiter().ExecuteReboot(params.ShouldReboot)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *NewRebootSuite) newRebootWaiter() *reboot.Reboot {
	return reboot.NewRebootForTest(s.agentConfig, s.rebootWaiter, s.clock)
}

func (s *NewRebootSuite) setupMocks(c *tc.C) *gomock.Controller {
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

	s.clock = mocks.NewMockClock(ctrl)
	s.clock.EXPECT().After(time.Minute * 10).DoAndReturn(func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time)
		return ch
	})
	s.clock.EXPECT().After(time.Second).DoAndReturn(func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time)
		close(ch)
		return ch
	}).AnyTimes()
	return ctrl
}

func (s *NewRebootSuite) expectManagerIsInitialized(lxd bool, times int) {
	s.containerManager.EXPECT().IsInitialized().Return(lxd).Times(times)
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
	s.rebootWaiter.EXPECT().NewServiceReference("jujud-unit-drupal-1").Return(s.service, nil)
	s.rebootWaiter.EXPECT().NewServiceReference("jujud-unit-mysql-1").Return(s.service, nil)
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

func TestNixRebootSuite(t *stdtesting.T) {
	tc.Run(t, &NixRebootSuite{})
}

func (s *NixRebootSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	testhelpers.PatchExecutableAsEchoArgs(c, s, rebootBin)
	s.tmpDir = c.MkDir()
	s.rebootScriptName = "juju-reboot-script"
	s.PatchValue(reboot.TmpFile, func() (*os.File, error) {
		script := s.rebootScript()
		return os.Create(script)
	})
}

func (s *NixRebootSuite) TestReboot(c *tc.C) {
	expectedParams := s.commandParams()
	err := reboot.ScheduleAction(params.ShouldReboot, 15)
	c.Assert(err, tc.ErrorIsNil)
	testhelpers.AssertEchoArgs(c, rebootBin, expectedParams...)
	filetesting.File{Path: s.rebootScriptName, Data: expectedRebootScript, Perm: 0755}.Check(c, s.tmpDir)
}

func (s *NixRebootSuite) TestShutdownNoContainers(c *tc.C) {
	expectedParams := s.commandParams()

	err := reboot.ScheduleAction(params.ShouldShutdown, 15)
	c.Assert(err, tc.ErrorIsNil)
	testhelpers.AssertEchoArgs(c, rebootBin, expectedParams...)
	filetesting.File{Path: s.rebootScriptName, Data: expectedShutdownScript, Perm: 0755}.Check(c, s.tmpDir)
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
