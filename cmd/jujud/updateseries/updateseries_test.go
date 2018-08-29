// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package updateseries

import (
	"bytes"
	"os"
	"path"
	"runtime"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	agenttools "github.com/juju/juju/agent/tools"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	svctesting "github.com/juju/juju/service/common/testing"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/testing"
	coretest "github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

type updateSeriesCmdSuite struct {
	testing.BaseSuite

	acfg        agent.Config
	dataDir     string
	machineName string
	unitNames   []string
	manager     service.SystemdServiceManager

	services    []*svctesting.FakeService
	serviceData *svctesting.FakeServiceData
}

func (s *updateSeriesCmdSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("jujud-updateseries doesn't work on Windows")
	}
	s.BaseSuite.SetUpSuite(c)
}

func (s *updateSeriesCmdSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.dataDir = c.MkDir()
	s.PatchValue(&cmdutil.DataDir, s.dataDir)

	tmpSystemdDir := path.Join(s.dataDir, "etc", "systemd", "system")
	tmpSystemdMultiUserDir := path.Join(tmpSystemdDir, "multi-user.target.wants")
	os.MkdirAll(tmpSystemdMultiUserDir, os.ModeDir|os.ModePerm)
	s.PatchValue(&systemdDir, tmpSystemdDir)
	s.PatchValue(&systemdMultiUserDir, tmpSystemdMultiUserDir)

	s.PatchValue(&isController, func() (bool, error) { return false, nil })

	s.machineName = "machine-0"
	s.unitNames = []string{"unit-ubuntu-0", "unit-mysql-0"}

	// Equivalent to reboot after upgrade.
	s.manager = service.NewServiceManager(
		func() (bool, error) { return true, nil },
		s.newService,
	)

	s.assertSetupAgentsForTest(c)
	s.setUpAgentConf(c)
	s.setUpServices(c)
	s.services[0].ResetCalls()
	s.setupTools(c, "trusty")
}

func (s *updateSeriesCmdSuite) TearDownTest(c *gc.C) {
	s.serviceData = nil
	s.services = nil
	s.BaseSuite.TearDownTest(c)
}

var _ = gc.Suite(&updateSeriesCmdSuite{})

func (s *updateSeriesCmdSuite) setUpAgentConf(c *gc.C) {
	// Read in copyAgentBinary() to get the version of agent.
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

func (s *updateSeriesCmdSuite) setUpServices(c *gc.C) {
	for _, fake := range append(s.unitNames, s.machineName) {
		s.addService("jujud-" + fake)
	}
	s.PatchValue(&service.ListServices, s.listServices)
}

func (s *updateSeriesCmdSuite) addService(name string) {
	svc, _ := s.newService(name, common.Conf{})
	svc.Install()
	svc.Start()
}

func (s *updateSeriesCmdSuite) listServices() ([]string, error) {
	return s.serviceData.InstalledNames(), nil
}

func (s *updateSeriesCmdSuite) newService(name string, conf common.Conf) (service.Service, error) {
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

func (s *updateSeriesCmdSuite) setupTools(c *gc.C, series string) {
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

type updateSeriesArgParsing struct {
	title      string
	args       []string
	errMatch   string
	dataDir    string
	toSeries   string
	fromSeries string
	controller bool
}

func (s *updateSeriesCmdSuite) TestArgValidationInInit(c *gc.C) {
	for i, test := range []updateSeriesArgParsing{
		{
			title:      "no args",
			errMatch:   "both --to-series and --from-series must be specified",
			controller: false,
		}, {
			title:      "to-series only",
			args:       []string{"--to-series", "trusty"},
			errMatch:   "--from-series must be specified",
			controller: false,
		}, {
			title:      "from-series only",
			args:       []string{"--from-series", "trusty"},
			errMatch:   "--to-series must be specified",
			controller: false,
		}, {
			title:      "to-series == from-series",
			args:       []string{"--to-series", "trusty", "--from-series", "trusty"},
			controller: false,
			errMatch:   "--to-series and --from-series cannot be the same",
		}, {
			title:      "controller machine",
			args:       []string{"--to-series", "xenial", "--from-series", "trusty"},
			controller: true,
			errMatch:   "cannot run on a controller machine",
		}, {
			title:      "success specifying series",
			args:       []string{"--to-series", "xenial", "--from-series", "trusty"},
			toSeries:   "xenial",
			fromSeries: "trusty",
			dataDir:    s.dataDir,
			controller: false,
		}, {
			title:      "unsupported: windows",
			args:       []string{"--to-series", "win10", "--from-series", "win8"},
			toSeries:   "xenial",
			fromSeries: "trusty",
			errMatch:   "windows not supported",
		}, {
			title:      "different operating systems",
			args:       []string{"--to-series", "win10", "--from-series", "xenial"},
			toSeries:   "xenial",
			fromSeries: "trusty",
			errMatch:   "series from two different operating systems specified",
		}, {
			title:      "success specifying series & datadir",
			args:       []string{"--to-series", "xenial", "--from-series", "trusty", "--data-dir", "/tmp/testme"},
			toSeries:   "xenial",
			fromSeries: "trusty",
			dataDir:    "/tmp/testme",
			controller: false,
		},
	} {
		c.Logf("%d: %s", i, test.title)
		s.PatchValue(&isController, func() (bool, error) { return test.controller, nil })
		cmd := &UpdateSeriesCommand{}
		err := cmdtesting.InitCommand(cmd, test.args)
		if test.errMatch == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(cmd.dataDir, gc.Equals, test.dataDir)
			c.Assert(cmd.toSeries, gc.Equals, test.toSeries)
			c.Assert(cmd.fromSeries, gc.Equals, test.fromSeries)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *updateSeriesCmdSuite) TestArgValidationInRun(c *gc.C) {
	for i, test := range []updateSeriesArgParsing{
		{
			title:    "unsupported: down grade",
			args:     []string{"--to-series", "trusty", "--from-series", "xenial"},
			errMatch: "downgrade to series using upstart not supported",
		},
	} {
		c.Logf("%d: %s", i, test.title)
		_, err := s.run(c, test.args...)
		c.Assert(err, gc.ErrorMatches, test.errMatch)
	}
}

func (s *updateSeriesCmdSuite) assertSetupAgentsForTest(c *gc.C) {
	agentsDir := path.Join(s.dataDir, "agents")
	err := os.MkdirAll(path.Join(agentsDir, s.machineName), os.ModeDir|os.ModePerm)
	c.Assert(err, jc.ErrorIsNil)
	for _, unit := range s.unitNames {
		err = os.Mkdir(path.Join(agentsDir, unit), os.ModeDir|os.ModePerm)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *updateSeriesCmdSuite) TestRunPreUpstartToSystemdUpgradeReboot(c *gc.C) {
	s.manager = service.NewServiceManager(
		func() (bool, error) { return false, nil },
		s.newService,
	)

	s.assertRunTest(c)
	s.assertServiceSymLinks(c)
	// Check idempotence
	s.services[0].ResetCalls()
	ctx := s.assertRunTest(c)
	c.Assert(cmdtesting.Stderr(ctx), gc.Matches, ""+
		"wrote jujud-unit-.*-0 agent, enabled and linked by symlink\n"+
		"wrote jujud-unit-.*-0 agent, enabled and linked by symlink\n"+
		"wrote jujud-machine-0 agent, enabled and linked by symlink\n"+
		"successfully copied and relinked agent binaries\n")
	s.assertServiceSymLinks(c)
}

func (s *updateSeriesCmdSuite) TestRunPostUpstartToSystemdUpgradeReboot(c *gc.C) {
	s.assertRunTest(c)
	// Check idempotence
	s.services[0].ResetCalls()
	ctx := s.assertRunTest(c)
	c.Assert(cmdtesting.Stderr(ctx), gc.Matches, ""+
		"wrote jujud-unit-.*-0 agent, enabled and linked by systemd\n"+
		"wrote jujud-unit-.*-0 agent, enabled and linked by systemd\n"+
		"wrote jujud-machine-0 agent, enabled and linked by systemd\n"+
		"successfully copied and relinked agent binaries\n")
}

func (s *updateSeriesCmdSuite) TestRunPostUpstartToSystemdUpgradeStartAllAgents(c *gc.C) {
	args := []string{"--to-series", "xenial", "--from-series", "trusty", "--start-agents"}
	ctx, err := s.run(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	s.assertVerifyCmdResults(c)
	c.Assert(cmdtesting.Stderr(ctx), gc.Matches, ""+
		"wrote jujud-unit-.*-0 agent, enabled and linked by systemd\n"+
		"wrote jujud-unit-.*-0 agent, enabled and linked by systemd\n"+
		"wrote jujud-machine-0 agent, enabled and linked by systemd\n"+
		"successfully copied and relinked agent binaries\n"+
		"started jujud-unit-.*-0 service\n"+
		"started jujud-unit-.*-0 service\n"+
		"started jujud-machine-0 service\n"+
		"all agents successfully restarted\n")
}

func (s *updateSeriesCmdSuite) TestSystemdToSystemdUpgrade(c *gc.C) {
	s.setupTools(c, "xenial")
	args := []string{"--to-series", "yakkety", "--from-series", "xenial"}
	ctx, err := s.run(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	s.assertToolsCopySymlink(c, "yakkety")
	c.Assert(cmdtesting.Stderr(ctx), gc.Matches, ""+
		"successfully copied and relinked agent binaries\n")
}

func (s *updateSeriesCmdSuite) TestSystemdToSystemdUpgradeStartAllAgents(c *gc.C) {
	s.setupTools(c, "xenial")
	args := []string{"--to-series", "yakkety", "--from-series", "xenial", "--start-agents"}
	ctx, err := s.run(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	s.assertToolsCopySymlink(c, "yakkety")
	c.Assert(cmdtesting.Stderr(ctx), gc.Matches, ""+
		"successfully copied and relinked agent binaries\n"+
		"started jujud-unit-.*-0 service\n"+
		"started jujud-unit-.*-0 service\n"+
		"started jujud-machine-0 service\n"+
		"all agents successfully restarted\n")
}

func (s *updateSeriesCmdSuite) TestRunTwiceFailFirstSystemdWriteService(c *gc.C) {
	s.manager = service.NewServiceManager(
		func() (bool, error) { return false, nil },
		s.newService,
	)

	s.services[0].SetErrors(
		errors.New("fail me"),
	)

	args := []string{"--to-series", "xenial", "--from-series", "trusty"}
	ctx, err := s.run(c, args...)
	c.Assert(err, jc.ErrorIsNil)

	s.assertServicesCalls(c, "WriteService", len(s.services))
	c.Assert(cmdtesting.Stderr(ctx), gc.Matches, ""+
		"wrote jujud-unit-.*-0 agent, enabled and linked by symlink\n"+
		"wrote jujud-machine-0 agent, enabled and linked by symlink\n"+
		"successfully copied and relinked agent binaries\n")

	// Check idempotence
	s.services[0].ResetCalls()
	ctx = s.assertRunTest(c)
	c.Assert(cmdtesting.Stderr(ctx), gc.Matches, ""+
		"wrote jujud-unit-.*-0 agent, enabled and linked by symlink\n"+
		"wrote jujud-unit-.*-0 agent, enabled and linked by symlink\n"+
		"wrote jujud-machine-0 agent, enabled and linked by symlink\n"+
		"successfully copied and relinked agent binaries\n")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updateSeriesCmdSuite) TestRunTwiceFailFirstWriteService(c *gc.C) {
	s.services[0].SetErrors(
		errors.New("fail me"),
	)

	args := []string{"--to-series", "xenial", "--from-series", "trusty"}
	ctx, err := s.run(c, args...)
	c.Assert(err, jc.ErrorIsNil)

	s.assertServicesCalls(c, "WriteService", len(s.services))
	c.Assert(cmdtesting.Stderr(ctx), gc.Matches, ""+
		"wrote jujud-unit-.*-0 agent, enabled and linked by systemd\n"+
		"wrote jujud-machine-0 agent, enabled and linked by systemd\n"+
		"successfully copied and relinked agent binaries\n")

	// Check idempotence
	s.services[0].ResetCalls()
	ctx = s.assertRunTest(c)
	c.Assert(cmdtesting.Stderr(ctx), gc.Matches, ""+
		"wrote jujud-unit-.*-0 agent, enabled and linked by systemd\n"+
		"wrote jujud-unit-.*-0 agent, enabled and linked by systemd\n"+
		"wrote jujud-machine-0 agent, enabled and linked by systemd\n"+
		"successfully copied and relinked agent binaries\n")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updateSeriesCmdSuite) TestRunTwiceRewriteToolsLink(c *gc.C) {
	s.manager = service.NewServiceManager(
		func() (bool, error) { return false, nil },
		s.newService,
	)

	s.assertRunTest(c)
	s.assertServiceSymLinks(c)

	ver := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: "xenial",
	}
	os.RemoveAll(path.Join(s.dataDir, "tools", ver.String()))
	name := s.services[0].Service.Name
	os.RemoveAll(path.Join(s.dataDir, "init", name, name+".service"))

	s.services[0].ResetCalls()
	s.assertRunTest(c)
	s.assertServiceSymLinks(c)
}

func (s *updateSeriesCmdSuite) assertRunTest(c *gc.C) *cmd.Context {
	args := []string{"--to-series", "xenial", "--from-series", "trusty"}
	ctx, err := s.run(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	s.assertVerifyCmdResults(c)
	return ctx
}

func (s *updateSeriesCmdSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, &UpdateSeriesCommand{manager: s.manager}, args...)
}

func (s *updateSeriesCmdSuite) assertVerifyCmdResults(c *gc.C) {
	s.assertServicesCalls(c, "WriteService", len(s.services))
	s.assertToolsCopySymlink(c, "xenial")
}

func (s *updateSeriesCmdSuite) assertToolsCopySymlink(c *gc.C, series string) {
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

func (s *updateSeriesCmdSuite) assertServiceSymLinks(c *gc.C) {
	for _, name := range append(s.unitNames, s.machineName) {
		svcName := "jujud-" + name
		svcFileName := svcName + ".service"
		result, err := os.Readlink(path.Join(systemdDir, svcFileName))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, gc.Equals, path.Join(systemd.DataDir, svcName, svcFileName))
	}
}

func (s *updateSeriesCmdSuite) assertServicesCalls(c *gc.C, svc string, expectedCnt int) {
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
