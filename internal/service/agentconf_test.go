// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The test cases in this file do not pertain to a specific command.

package service_test

import (
	"os"
	"path"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/agent"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/service"
	"github.com/juju/juju/internal/service/common"
	"github.com/juju/juju/internal/service/mocks"
	"github.com/juju/juju/internal/testing"
)

type agentConfSuite struct {
	testing.BaseSuite

	agentConf           agent.Config
	dataDir             string
	machineName         string
	unitNames           []string
	systemdDir          string
	systemdMultiUserDir string
	systemdDataDir      string // service.SystemdDataDir
	manager             service.SystemdServiceManager

	services []*mocks.MockService
}

func (s *agentConfSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *agentConfSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.dataDir = c.MkDir()
	s.systemdDir = path.Join(s.dataDir, "etc", "systemd", "system")
	s.systemdMultiUserDir = path.Join(s.systemdDir, "multi-user.target.wants")
	c.Assert(os.MkdirAll(s.systemdMultiUserDir, os.ModeDir|os.ModePerm), tc.ErrorIsNil)
	s.systemdDataDir = path.Join(s.dataDir, "lib", "systemd", "system")

	s.machineName = "machine-0"
	s.unitNames = []string{"unit-ubuntu-0", "unit-mysql-0"}

	s.manager = service.NewServiceManager(
		func() bool { return true },
		s.newService,
	)

	s.assertSetupAgentsForTest(c)
	s.setUpAgentConf(c)
}

func (s *agentConfSuite) TearDownTest(c *tc.C) {
	s.services = nil
	s.BaseSuite.TearDownTest(c)
}
func TestAgentConfSuite(t *stdtesting.T) { tc.Run(t, &agentConfSuite{}) }
func (s *agentConfSuite) setUpAgentConf(c *tc.C) {
	configParams := agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: s.dataDir},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: jujuversion.Current,
		APIAddresses:      []string{"localhost:17070"},
		CACert:            testing.CACert,
		Password:          "fake",
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
	}

	agentConf, err := agent.NewAgentConfig(configParams)
	c.Assert(err, tc.ErrorIsNil)

	err = agentConf.Write()
	c.Assert(err, tc.ErrorIsNil)

	s.agentConf = agentConf
}

func (s *agentConfSuite) setUpServices(ctrl *gomock.Controller) {
	s.addService(ctrl, "jujud-"+s.machineName)
}

func (s *agentConfSuite) addService(ctrl *gomock.Controller, name string) {
	svc := mocks.NewMockService(ctrl)
	svc.EXPECT().Name().Return(name).AnyTimes()
	s.services = append(s.services, svc)
}

func (s *agentConfSuite) newService(name string, _ common.Conf) (service.Service, error) {
	for _, svc := range s.services {
		if svc.Name() == name {
			return svc, nil
		}
	}
	return nil, errors.NotFoundf("service %q", name)
}

func (s *agentConfSuite) assertSetupAgentsForTest(c *tc.C) {
	agentsDir := path.Join(s.dataDir, "agents")
	err := os.MkdirAll(path.Join(agentsDir, s.machineName), os.ModeDir|os.ModePerm)
	c.Assert(err, tc.ErrorIsNil)
	for _, unit := range s.unitNames {
		err = os.Mkdir(path.Join(agentsDir, unit), os.ModeDir|os.ModePerm)
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *agentConfSuite) TestFindAgents(c *tc.C) {
	machineAgent, unitAgents, errAgents, err := s.manager.FindAgents(s.dataDir)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(machineAgent, tc.Equals, s.machineName)
	c.Assert(unitAgents, tc.SameContents, s.unitNames)
	c.Assert(errAgents, tc.HasLen, 0)
}

func (s *agentConfSuite) TestFindAgentsUnexpectedTagType(c *tc.C) {
	unexpectedAgent := names.NewApplicationTag("failme").String()
	unexpectedAgentDir := path.Join(s.dataDir, "agents", unexpectedAgent)
	err := os.MkdirAll(unexpectedAgentDir, os.ModeDir|os.ModePerm)
	c.Assert(err, tc.ErrorIsNil)

	machineAgent, unitAgents, unexpectedAgents, err := s.manager.FindAgents(s.dataDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineAgent, tc.Equals, s.machineName)
	c.Assert(unitAgents, tc.SameContents, s.unitNames)
	c.Assert(unexpectedAgents, tc.DeepEquals, []string{unexpectedAgent})
}

func (s *agentConfSuite) TestCreateAgentConfDesc(c *tc.C) {
	conf, err := s.manager.CreateAgentConf("machine-2", s.dataDir)
	c.Assert(err, tc.ErrorIsNil)
	// Spot check Conf
	c.Assert(conf.Desc, tc.Equals, "juju agent for machine-2")
}

func (s *agentConfSuite) TestCreateAgentConfLogPath(c *tc.C) {
	conf, err := s.manager.CreateAgentConf("machine-2", s.dataDir)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(conf.Logfile, tc.Equals, "/var/log/juju/machine-2.log")
}

func (s *agentConfSuite) TestCreateAgentConfFailAgentKind(c *tc.C) {
	_, err := s.manager.CreateAgentConf("application-fail", s.dataDir)
	c.Assert(err, tc.ErrorMatches, `agent "application-fail" is neither a machine nor a unit`)
}

func (s *agentConfSuite) TestWriteSystemdAgent(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.setUpServices(ctrl)
	s.services[0].EXPECT().WriteService().Return(nil)

	err := s.manager.WriteSystemdAgent(
		s.machineName, s.systemdDataDir, s.systemdMultiUserDir)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *agentConfSuite) TestWriteSystemdAgentSystemdNotRunning(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.setUpServices(ctrl)
	s.services[0].EXPECT().WriteService().Return(nil)

	s.manager = service.NewServiceManager(
		func() bool { return false },
		s.newService,
	)

	err := s.manager.WriteSystemdAgent(
		s.machineName, s.systemdDataDir, s.systemdMultiUserDir)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *agentConfSuite) TestWriteSystemdAgentWriteServiceFail(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.setUpServices(ctrl)
	s.services[0].EXPECT().WriteService().Return(errors.New("fail me"))

	err := s.manager.WriteSystemdAgent(
		s.machineName, s.systemdDataDir, s.systemdMultiUserDir)
	c.Assert(err, tc.ErrorMatches, "fail me")
}
