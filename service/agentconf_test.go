// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//The unit testcases in this file do not pertain to an specific command.

package service_test

import (
	"bytes"
	"os"
	"path"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	agentcmd "github.com/juju/juju/cmd/jujud/agent"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	svctesting "github.com/juju/juju/service/common/testing"
	"github.com/juju/juju/testing"
	coretest "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

type agentConfSuite struct {
	testing.BaseSuite

	acfg                agent.Config
	dataDir             string
	machineName         string
	unitNames           []string
	systemdDir          string
	systemdMultiUserDir string
	sysdIsRunning       bool
	manager             service.SystemdServiceManager

	services    []*svctesting.FakeService
	serviceData *svctesting.FakeServiceData
}

func (s *agentConfSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *agentConfSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.dataDir = c.MkDir()
	s.PatchValue(&cmdutil.DataDir, s.dataDir)

	tmpSystemdDir := path.Join(s.dataDir, "etc", "systemd", "system")
	tmpSystemdMultiUserDir := path.Join(tmpSystemdDir, "multi-user.target.wants")
	os.MkdirAll(tmpSystemdMultiUserDir, os.ModeDir|os.ModePerm)
	s.PatchValue(&s.systemdDir, tmpSystemdDir)
	s.PatchValue(&s.systemdMultiUserDir, tmpSystemdMultiUserDir)

	s.machineName = "machine-0"
	s.unitNames = []string{"unit-ubuntu-0", "unit-mysql-0"}

	s.manager = service.NewSystemdServiceManager(func() (bool, error) { return true, nil })

	s.assertSetupAgentsForTest(c)
	s.setUpAgentConf(c)
	s.setUpServices(c)
	s.services[0].ResetCalls()
	s.setupTools(c, "trusty")
}

func (s *agentConfSuite) TearDownTest(c *gc.C) {
	s.serviceData = nil
	s.services = nil
	s.BaseSuite.TearDownTest(c)
}

var _ = gc.Suite(&agentConfSuite{})

func (s *agentConfSuite) setUpAgentConf(c *gc.C) {
	// Required for CopyAgentBinaries to evaluate the version of the agent.
	configParams := agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: s.dataDir},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: jujuversion.Current,
		APIAddresses:      []string{"localhost:17070"},
		CACert:            testing.CACert,
		Password:          "fake",
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
		MongoVersion:      mongo.Mongo32wt,
	}

	acfg, err := agent.NewAgentConfig(configParams)
	c.Assert(err, jc.ErrorIsNil)
	err = acfg.Write()
	c.Assert(err, jc.ErrorIsNil)
	s.acfg = acfg
}

func (s *agentConfSuite) setUpServices(c *gc.C) {
	for _, fake := range append(s.unitNames, s.machineName) {
		s.addService("jujud-" + fake)
	}
	s.PatchValue(&service.NewService, s.newService)
	s.PatchValue(&service.ListServices, s.listServices)
}

func (s *agentConfSuite) addService(name string) {
	svc, _ := s.newService(name, common.Conf{}, "")
	svc.Install()
	svc.Start()
}

func (s *agentConfSuite) listServices() ([]string, error) {
	return s.serviceData.InstalledNames(), nil
}

func (s *agentConfSuite) newService(name string, conf common.Conf, series string) (service.Service, error) {
	for _, svc := range s.services {
		if svc.Name() == name {
			return svc, nil
		}
	}
	if s.serviceData == nil {
		s.serviceData = svctesting.NewFakeServiceData()
	}
	svc := &svctesting.FakeService{
		FakeServiceData: s.serviceData,
		Service: common.Service{
			Name: name,
			Conf: common.Conf{},
		},
		DataDir: s.dataDir,
	}
	s.services = append(s.services, svc)
	return svc, nil
}

func (s *agentConfSuite) setupTools(c *gc.C, series string) {
	files := []*testing.TarFile{
		testing.NewTarFile("jujud", 0755, "jujuc executable"),
	}
	data, checksum := testing.TarGz(files...)
	testTools := &coretest.Tools{
		URL: "http://foo/bar1",
		Version: version.Binary{
			Number: jujuversion.Current,
			Arch:   arch.HostArch(),
			Series: series,
		},
		Size:   int64(len(data)),
		SHA256: checksum,
	}
	err := agenttools.UnpackTools(s.dataDir, testTools, bytes.NewReader(data))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *agentConfSuite) assertSetupAgentsForTest(c *gc.C) {
	agentsDir := path.Join(s.dataDir, "agents")
	err := os.MkdirAll(path.Join(agentsDir, s.machineName), os.ModeDir|os.ModePerm)
	c.Assert(err, jc.ErrorIsNil)
	for _, unit := range s.unitNames {
		err = os.Mkdir(path.Join(agentsDir, unit), os.ModeDir|os.ModePerm)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *agentConfSuite) TestFindAgents(c *gc.C) {
	var err error
	var machineAgent string
	var unitAgents []string
	machineAgent, unitAgents, _, err = s.manager.FindAgents(s.dataDir)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(machineAgent, gc.Equals, s.machineName)
	c.Assert(unitAgents, jc.SameContents, s.unitNames)
}

func (s *agentConfSuite) TestFindAgentsFail(c *gc.C) {
	agentsDir := path.Join(s.dataDir, "agents")
	err := os.MkdirAll(path.Join(agentsDir, names.ApplicationTagKind+"-failme-0"), os.ModeDir|os.ModePerm)
	var machineAgent string
	var unitAgents []string
	machineAgent, unitAgents, _, err = s.manager.FindAgents(s.dataDir)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(machineAgent, gc.Equals, s.machineName)
	c.Assert(unitAgents, jc.SameContents, s.unitNames)
}

func (s *agentConfSuite) TestCreateAgentConfDesc(c *gc.C) {
	conf, err := s.manager.CreateAgentConf("machine-2", s.dataDir, "xenial")
	c.Assert(err, jc.ErrorIsNil)
	// Spot check Conf
	c.Assert(conf.Desc, gc.Equals, "juju agent for machine-2")
}

func (s *agentConfSuite) TestCreateAgentConfLogPath(c *gc.C) {
	conf, err := s.manager.CreateAgentConf("machine-2", s.dataDir, "xenial")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conf.Logfile, gc.Equals, "/var/log/juju/machine-2.log")
}

func (s *agentConfSuite) TestCreateAgentConfFailAgentKind(c *gc.C) {
	_, err := s.manager.CreateAgentConf("application-fail", s.dataDir, "xenial")
	c.Assert(err, gc.ErrorMatches, `agent "application-fail" is neither a machine nor a unit`)
}

func (s *agentConfSuite) TestStartAllAgents(c *gc.C) {
	_ = cmdtesting.Context(c)
	var err error
	var machineAgent string
	var unitAgents []string
	machineAgent, unitAgents, _, err = s.manager.FindAgents(s.dataDir)
	c.Assert(err, jc.ErrorIsNil)
	_, _, err = s.manager.StartAllAgents(machineAgent, unitAgents, s.dataDir, "xenial")
	c.Assert(err, jc.ErrorIsNil)

	s.assertServicesCalls(c, "Start", len(s.services))
}

func (s *agentConfSuite) TestStartAllAgentsFailUnit(c *gc.C) {
	s.services[0].SetErrors(
		errors.New("fail me"),
	)

	var err error
	var machineAgent string
	var unitAgents []string
	machineAgent, unitAgents, _, err = s.manager.FindAgents(s.dataDir)
	c.Assert(err, jc.ErrorIsNil)
	_, _, err = s.manager.StartAllAgents(machineAgent, unitAgents, s.dataDir, "xenial")
	c.Assert(err, gc.ErrorMatches, "failed to start .* service: fail me")

	s.assertServicesCalls(c, "Start", 1)
}

func (s *agentConfSuite) TestWriteServiceCopyStartAllAgents(c *gc.C) {
	var err error
	var machineAgent string
	var unitAgents []string
	machineAgent, unitAgents, _, err = s.manager.FindAgents(s.dataDir)
	c.Assert(err, jc.ErrorIsNil)

	_, _, _, err = s.manager.WriteSystemdAgents(machineAgent, unitAgents, s.dataDir, s.systemdDir, s.systemdMultiUserDir, "xenial")
	c.Assert(err, jc.ErrorIsNil)
	s.assertServicesCalls(c, "WriteService", len(s.services))

	jujuVersion, err := agentcmd.GetJujuVersion(machineAgent, s.dataDir)
	c.Assert(err, jc.ErrorIsNil)

	err = s.manager.CopyAgentBinary(machineAgent, unitAgents, s.dataDir, "xenial", "trusty", jujuVersion)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.manager.StartAllAgents(machineAgent, unitAgents, s.dataDir, "xenial")
	c.Assert(err, jc.ErrorIsNil)
	s.assertServicesCalls(c, "Start", len(s.services))
	s.assertToolsCopySymlink(c, "xenial")
}

func (s *agentConfSuite) TestPreRebootWriteServiceCopyStartAllAgents(c *gc.C) {
	s.manager = service.NewSystemdServiceManager(func() (bool, error) { return false, nil })
	var err error
	var machineAgent string
	var unitAgents []string
	machineAgent, unitAgents, _, err = s.manager.FindAgents(s.dataDir)
	c.Assert(err, jc.ErrorIsNil)

	_, _, _, err = s.manager.WriteSystemdAgents(machineAgent, unitAgents, s.dataDir, s.systemdDir, s.systemdMultiUserDir, "xenial")
	c.Assert(err, jc.ErrorIsNil)
	s.assertServicesCalls(c, "WriteService", len(s.services))

	jujuVersion, err := agentcmd.GetJujuVersion(machineAgent, s.dataDir)
	c.Assert(err, jc.ErrorIsNil)

	err = s.manager.CopyAgentBinary(machineAgent, unitAgents, s.dataDir, "xenial", "trusty", jujuVersion)
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.manager.StartAllAgents(machineAgent, unitAgents, s.dataDir, "xenial")
	c.Assert(err, gc.ErrorMatches, "systemd is not fully running, please reboot to start agents")
}

func (s *agentConfSuite) assertToolsCopySymlink(c *gc.C, series string) {
	// Check tools changes
	ver := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series,
	}
	jujuTools, err := agenttools.ReadTools(s.dataDir, ver)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jujuTools.Version, gc.DeepEquals, ver)

	for _, name := range append(s.unitNames, s.machineName) {
		link := path.Join(s.dataDir, "tools", name)
		linkResult, err := os.Readlink(link)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(linkResult, gc.Equals, path.Join(s.dataDir, "tools", ver.String()))
	}
}

func (s *agentConfSuite) assertServiceSymLinks(c *gc.C) {
	for _, agent := range append(s.unitNames, s.machineName) {
		svcName := "jujud-" + agent
		svcFileName := svcName + ".service"
		result, err := os.Readlink(path.Join(s.systemdDir, svcFileName))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, gc.Equals, path.Join(s.dataDir, "init", svcName, svcFileName))
	}
}

func (s *agentConfSuite) assertServicesCalls(c *gc.C, svc string, expectedCnt int) {
	// Call list shared by the services
	calls := s.services[0].Calls()
	serviceCount := 0
	for _, call := range calls {
		if call.FuncName == svc {
			serviceCount += 1
		}
	}
	c.Assert(serviceCount, gc.Equals, expectedCnt)
}
