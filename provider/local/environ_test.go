// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	coreCloudinit "github.com/juju/juju/cloudinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxc"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/jujutest"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/provider/local"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/initsystems"
	"github.com/juju/juju/state/multiwatcher"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

const echoCommandScript = "#!/bin/sh\necho $0 \"$@\" >> $0.args"

type environSuite struct {
	baseProviderSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.baseProviderSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
}

func (s *environSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.baseProviderSuite.TearDownTest(c)
}

func (*environSuite) TestOpenFailsWithProtectedDirectories(c *gc.C) {
	testConfig := minimalConfig(c)
	testConfig, err := testConfig.Apply(map[string]interface{}{
		"root-dir": "/usr/lib/juju",
	})
	c.Assert(err, jc.ErrorIsNil)

	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.ErrorMatches, "failure setting config: mkdir .* permission denied")
	c.Assert(environ, gc.IsNil)
}

func (s *environSuite) TestName(c *gc.C) {
	testConfig := minimalConfig(c)
	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(environ.Config().Name(), gc.Equals, "test")
}

func (s *environSuite) TestGetToolsMetadataSources(c *gc.C) {
	testConfig := minimalConfig(c)
	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, jc.ErrorIsNil)
	sources, err := tools.GetMetadataSources(environ)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 0)
}

func (*environSuite) TestSupportedArchitectures(c *gc.C) {
	testConfig := minimalConfig(c)
	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, jc.ErrorIsNil)
	arches, err := environ.SupportedArchitectures()
	c.Assert(err, jc.ErrorIsNil)
	for _, a := range arches {
		c.Assert(arch.IsSupportedArch(a), jc.IsTrue)
	}
}

func (*environSuite) TestSupportsNetworking(c *gc.C) {
	testConfig := minimalConfig(c)
	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, jc.ErrorIsNil)
	_, ok := environs.SupportsNetworking(environ)
	c.Assert(ok, jc.IsFalse)
}

type localJujuTestSuite struct {
	baseProviderSuite
	jujutest.Tests
	oldUpstartLocation string
	testPath           string
	fakesudo           string
	services           *service.Services
}

func (s *localJujuTestSuite) SetUpTest(c *gc.C) {
	s.baseProviderSuite.SetUpTest(c)
	// Construct the directories first.
	err := local.CreateDirs(c, minimalConfig(c))
	c.Assert(err, jc.ErrorIsNil)
	s.testPath = c.MkDir()
	s.fakesudo = filepath.Join(s.testPath, "sudo")
	s.PatchEnvPathPrepend(s.testPath)
	s.PatchValue(&lxc.TemplateLockDir, c.MkDir())
	s.PatchValue(&lxc.TemplateStopTimeout, 500*time.Millisecond)

	// Write a fake "sudo" which records its args to sudo.args.
	err = ioutil.WriteFile(s.fakesudo, []byte(echoCommandScript), 0755)
	c.Assert(err, jc.ErrorIsNil)

	// Add in an admin secret
	s.Tests.TestConfig["admin-secret"] = "sekrit"
	s.PatchValue(local.CheckIfRoot, func() bool { return false })
	s.Tests.SetUpTest(c)

	s.PatchValue(local.ExecuteCloudConfig, func(environs.BootstrapContext, *cloudinit.MachineConfig, *coreCloudinit.Config) error {
		return nil
	})

	// Patch out the init system.
	initSystem := service.NewMockInitSystem("<mock-provider-local", service.InitSystemUpstart)
	s.services = service.NewServices(c.MkDir(), initSystem)
	s.PatchValue(local.NewServices, func(string) (*service.Services, error) {
		return s.services, nil
	})
}

func (s *localJujuTestSuite) TearDownTest(c *gc.C) {
	s.Tests.TearDownTest(c)
	s.baseProviderSuite.TearDownTest(c)
}

func (s *localJujuTestSuite) MakeTool(c *gc.C, name, script string) {
	path := filepath.Join(s.testPath, name)
	script = "#!/bin/bash\n" + script
	err := ioutil.WriteFile(path, []byte(script), 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localJujuTestSuite) StoppedStatus(c *gc.C) {
	s.MakeTool(c, "status", `echo "some-service stop/waiting"`)
}

func (s *localJujuTestSuite) RunningStatus(c *gc.C) {
	s.MakeTool(c, "status", `echo "some-service start/running, process 123"`)
}

var _ = gc.Suite(&localJujuTestSuite{
	Tests: jujutest.Tests{
		TestConfig: minimalConfigValues(),
	},
})

func (s *localJujuTestSuite) TestStartStop(c *gc.C) {
	c.Skip("StartInstance not implemented yet.")
}

func (s *localJujuTestSuite) testBootstrap(c *gc.C, cfg *config.Config) environs.Environ {
	ctx := envtesting.BootstrapContext(c)
	environ, err := local.Provider.PrepareForBootstrap(ctx, cfg)
	c.Assert(err, jc.ErrorIsNil)
	availableTools := coretools.List{&coretools.Tools{
		Version: version.Current,
		URL:     "http://testing.invalid/tools.tar.gz",
	}}
	_, _, finalizer, err := environ.Bootstrap(ctx, environs.BootstrapParams{
		AvailableTools: availableTools,
	})
	c.Assert(err, jc.ErrorIsNil)
	mcfg, err := environs.NewBootstrapMachineConfig(constraints.Value{}, "quantal")
	c.Assert(err, jc.ErrorIsNil)
	mcfg.Tools = availableTools[0]
	err = finalizer(ctx, mcfg)
	c.Assert(err, jc.ErrorIsNil)
	return environ
}

func (s *localJujuTestSuite) TestBootstrap(c *gc.C) {

	minCfg := minimalConfig(c)

	mockFinish := func(ctx environs.BootstrapContext, mcfg *cloudinit.MachineConfig, cloudcfg *coreCloudinit.Config) error {

		envCfgAttrs := minCfg.AllAttrs()
		if val, ok := envCfgAttrs["enable-os-refresh-update"]; !ok {
			c.Check(cloudcfg.AptUpdate(), jc.IsFalse)
		} else {
			c.Check(cloudcfg.AptUpdate(), gc.Equals, val)
		}

		if val, ok := envCfgAttrs["enable-os-upgrade"]; !ok {
			c.Check(cloudcfg.AptUpgrade(), jc.IsFalse)
		} else {
			c.Check(cloudcfg.AptUpgrade(), gc.Equals, val)
		}

		if !mcfg.EnableOSRefreshUpdate {
			c.Assert(cloudcfg.Packages(), gc.HasLen, 0)
		}
		c.Assert(mcfg.AgentEnvironment, gc.Not(gc.IsNil))
		c.Assert(mcfg.AgentEnvironment[agent.LxcBridge], gc.Not(gc.Equals), "")
		// local does not allow machine-0 to host units
		c.Assert(mcfg.Jobs, gc.DeepEquals, []multiwatcher.MachineJob{multiwatcher.JobManageEnviron})
		return nil
	}
	s.PatchValue(local.ExecuteCloudConfig, mockFinish)

	// Test that defaults are correct.
	s.testBootstrap(c, minCfg)

	// Test that overrides work.
	minCfg, err := minCfg.Apply(map[string]interface{}{
		"enable-os-refresh-update": true,
		"enable-os-upgrade":        true,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.testBootstrap(c, minCfg)
}

func (s *localJujuTestSuite) TestDestroy(c *gc.C) {
	env := s.testBootstrap(c, minimalConfig(c))
	err := env.Destroy()
	// Succeeds because there's no "agents" directory,
	// so destroy will just return without attempting
	// sudo or anything.
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fakesudo+".args", jc.DoesNotExist)
}

func (s *localJujuTestSuite) makeAgentsDir(c *gc.C, env environs.Environ) {
	rootDir := env.Config().AllAttrs()["root-dir"].(string)
	agentsDir := filepath.Join(rootDir, "agents")
	err := os.Mkdir(agentsDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *localJujuTestSuite) TestDestroyCallSudo(c *gc.C) {
	env := s.testBootstrap(c, minimalConfig(c))
	s.makeAgentsDir(c, env)
	err := env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	data, err := ioutil.ReadFile(s.fakesudo + ".args")
	c.Assert(err, jc.ErrorIsNil)
	expected := []string{
		s.fakesudo,
		"env",
		"JUJU_HOME=" + osenv.JujuHome(),
		os.Args[0],
		"destroy-environment",
		"-y",
		"--force",
		env.Config().Name(),
	}
	c.Assert(string(data), gc.Equals, strings.Join(expected, " ")+"\n")
}

func (s *localJujuTestSuite) makeFakeInitScripts(c *gc.C, env environs.Environ) (mongoService *service.Service, machineAgent *service.Service) {
	s.MakeTool(c, "start", `echo "some-service start/running, process 123"`)

	// First start mongo.
	namespace := env.Config().AllAttrs()["namespace"].(string)
	mongoConf := service.Conf{Conf: initsystems.Conf{
		Desc: "fake mongo",
		Cmd:  "echo FAKE",
	}}
	mongoService = s.services.NewService(mongo.ServiceName(namespace), mongoConf)
	err := mongoService.Install()
	c.Assert(err, jc.ErrorIsNil)
	running, err := mongoService.IsRunning()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(running, jc.IsTrue)

	// Then start jujud.
	agentConf := service.Conf{Conf: initsystems.Conf{
		Desc: "fake agent",
		Cmd:  "echo FAKE",
	}}
	machineAgent = s.services.NewService(fmt.Sprintf("juju-agent-%s", namespace), agentConf)
	err = machineAgent.Install()
	c.Assert(err, jc.ErrorIsNil)
	running, err = machineAgent.IsRunning()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(running, jc.IsTrue)

	return mongoService, machineAgent
}

func (s *localJujuTestSuite) TestDestroyRemovesUpstartServices(c *gc.C) {
	env := s.testBootstrap(c, minimalConfig(c))
	s.makeAgentsDir(c, env)
	mongo, machineAgent := s.makeFakeInitScripts(c, env)
	s.PatchValue(local.CheckIfRoot, func() bool { return true })

	err := env.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	enabled, err := mongo.IsEnabled()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enabled, jc.IsFalse)

	enabled, err = machineAgent.IsEnabled()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enabled, jc.IsFalse)
}

func (s *localJujuTestSuite) TestDestroyRemovesContainers(c *gc.C) {
	env := s.testBootstrap(c, minimalConfig(c))
	s.makeAgentsDir(c, env)
	s.PatchValue(local.CheckIfRoot, func() bool { return true })

	namespace := env.Config().AllAttrs()["namespace"].(string)
	manager, err := lxc.NewContainerManager(container.ManagerConfig{
		container.ConfigName:   namespace,
		container.ConfigLogDir: "logdir",
		"use-clone":            "false",
	}, nil)
	c.Assert(err, jc.ErrorIsNil)

	machine1 := containertesting.CreateContainer(c, manager, "1")

	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	container := s.ContainerFactory.New(string(machine1.Id()))
	c.Assert(container.IsConstructed(), jc.IsFalse)
}

func (s *localJujuTestSuite) TestBootstrapRemoveLeftovers(c *gc.C) {
	cfg := minimalConfig(c)
	rootDir := cfg.AllAttrs()["root-dir"].(string)

	// Create a dir inside local/log that should be removed by Bootstrap.
	logThings := filepath.Join(rootDir, "log", "things")
	err := os.MkdirAll(logThings, 0755)
	c.Assert(err, jc.ErrorIsNil)

	// Create a cloud-init-output.log in root-dir that should be
	// removed/truncated by Bootstrap.
	cloudInitOutputLog := filepath.Join(rootDir, "cloud-init-output.log")
	err = ioutil.WriteFile(cloudInitOutputLog, []byte("ohai"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	s.testBootstrap(c, cfg)
	c.Assert(logThings, jc.DoesNotExist)
	c.Assert(cloudInitOutputLog, jc.DoesNotExist)
	c.Assert(filepath.Join(rootDir, "log"), jc.IsSymlink)
}

func (s *localJujuTestSuite) TestConstraintsValidator(c *gc.C) {
	ctx := envtesting.BootstrapContext(c)
	env, err := local.Provider.PrepareForBootstrap(ctx, minimalConfig(c))
	c.Assert(err, jc.ErrorIsNil)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	hostArch := arch.HostArch()
	cons := constraints.MustParse(fmt.Sprintf("arch=%s instance-type=foo tags=bar cpu-power=10 cpu-cores=2", hostArch))
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-cores", "cpu-power", "instance-type", "tags"})
}

func (s *localJujuTestSuite) TestConstraintsValidatorVocab(c *gc.C) {
	env := s.Prepare(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)

	hostArch := arch.HostArch()
	var invalidArch string
	for _, a := range arch.AllSupportedArches {
		if a != hostArch {
			invalidArch = a
			break
		}
	}
	cons := constraints.MustParse(fmt.Sprintf("arch=%s", invalidArch))
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: arch="+invalidArch+"\nvalid values are:.*")
}

func (s *localJujuTestSuite) TestStateServerInstances(c *gc.C) {
	env := s.testBootstrap(c, minimalConfig(c))

	instances, err := env.StateServerInstances()
	c.Assert(err, gc.Equals, environs.ErrNotBootstrapped)
	c.Assert(instances, gc.HasLen, 0)

	s.makeAgentsDir(c, env)
	instances, err = env.StateServerInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.DeepEquals, []instance.Id{"localhost"})
}
