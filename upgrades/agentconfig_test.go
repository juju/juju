// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/version"
)

type migrateLocalProviderAgentConfigSuite struct {
	jujutesting.JujuConnSuite

	config agent.ConfigSetterWriter
	ctx    upgrades.Context
}

var _ = gc.Suite(&migrateLocalProviderAgentConfigSuite{})

func (s *migrateLocalProviderAgentConfigSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("No need to test local provider on windows")
	}
	s.JujuConnSuite.SetUpTest(c)
	// Make sure we fallback to SUDO_USER if USER is root.
	s.PatchEnvironment("USER", "root")
	s.PatchEnvironment("SUDO_USER", "user")
	s.PatchValue(upgrades.RootLogDir, c.MkDir())
	s.PatchValue(upgrades.RootSpoolDir, c.MkDir())
	s.PatchValue(&agent.DefaultDataDir, c.MkDir())
	s.PatchValue(upgrades.ChownPath, func(_, _ string) error { return nil })
	s.PatchValue(upgrades.IsLocalEnviron, func(_ *config.Config) bool { return true })
}

func (s *migrateLocalProviderAgentConfigSuite) primeConfig(c *gc.C, st *state.State, job state.MachineJob, tag names.Tag) {
	rootDir := c.MkDir()
	sharedStorageDir := filepath.Join(rootDir, "shared-storage")
	c.Assert(os.MkdirAll(sharedStorageDir, 0755), gc.IsNil)
	localLogDir := filepath.Join(rootDir, "log")
	c.Assert(os.MkdirAll(localLogDir, 0755), gc.IsNil)

	initialConfig, err := agent.NewAgentConfig(agent.AgentConfigParams{
		Tag:               tag,
		Password:          "blah",
		CACert:            testing.CACert,
		StateAddresses:    []string{"localhost:1111"},
		DataDir:           agent.DefaultDataDir,
		LogDir:            agent.DefaultLogDir,
		UpgradedToVersion: version.MustParse("1.16.0"),
		Environment:       s.State.EnvironTag(),
		Values: map[string]string{
			"SHARED_STORAGE_ADDR": "blah",
			"SHARED_STORAGE_DIR":  sharedStorageDir,
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.config = initialConfig
	c.Assert(s.config.Write(), gc.IsNil)

	apiState, _ := s.OpenAPIAsNewMachine(c, job)
	s.ctx = &mockContext{
		realAgentConfig: initialConfig,
		apiState:        apiState,
		state:           st,
	}

	newCfg := (map[string]interface{}{
		"root-dir": rootDir,
	})
	err = s.State.UpdateEnvironConfig(newCfg, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *migrateLocalProviderAgentConfigSuite) assertConfigProcessed(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	allAttrs := envConfig.AllAttrs()

	namespace, _ := allAttrs["namespace"].(string)
	c.Assert(namespace, gc.Equals, "user-dummyenv")
	container, _ := allAttrs["container"].(string)
	c.Assert(container, gc.Equals, "lxc")

	expectedDataDir, _ := allAttrs["root-dir"].(string)
	expectedSharedStorageDir := filepath.Join(expectedDataDir, "shared-storage")
	_, err = os.Lstat(expectedSharedStorageDir)
	c.Assert(err, gc.NotNil)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	expectedLogDir := filepath.Join(*upgrades.RootLogDir, "juju-"+namespace)
	expectedJobs := []multiwatcher.MachineJob{multiwatcher.JobManageEnviron}
	tag := s.ctx.AgentConfig().Tag()

	// We need to read the actual migrated agent config.
	configFilePath := agent.ConfigPath(expectedDataDir, tag)
	agentConfig, err := agent.ReadConfig(configFilePath)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(agentConfig.DataDir(), gc.Equals, expectedDataDir)
	c.Assert(agentConfig.LogDir(), gc.Equals, expectedLogDir)
	c.Assert(agentConfig.Jobs(), gc.DeepEquals, expectedJobs)
	c.Assert(agentConfig.Value("SHARED_STORAGE_ADDR"), gc.Equals, "")
	c.Assert(agentConfig.Value("SHARED_STORAGE_DIR"), gc.Equals, "")
	c.Assert(agentConfig.Value(agent.Namespace), gc.Equals, namespace)
	agentService := "juju-agent-user-dummyenv"
	c.Assert(agentConfig.Value(agent.AgentServiceName), gc.Equals, agentService)
	c.Assert(agentConfig.Value(agent.ContainerType), gc.Equals, "")
}

func (s *migrateLocalProviderAgentConfigSuite) assertConfigNotProcessed(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	allAttrs := envConfig.AllAttrs()

	namespace, _ := allAttrs["namespace"].(string)
	c.Assert(namespace, gc.Equals, "")
	container, _ := allAttrs["container"].(string)
	c.Assert(container, gc.Equals, "")

	rootDir, _ := allAttrs["root-dir"].(string)
	expectedSharedStorageDir := filepath.Join(rootDir, "shared-storage")
	_, err = os.Lstat(expectedSharedStorageDir)
	c.Assert(err, jc.ErrorIsNil)
	tag := s.ctx.AgentConfig().Tag()

	// We need to read the actual migrated agent config.
	configFilePath := agent.ConfigPath(agent.DefaultDataDir, tag)
	agentConfig, err := agent.ReadConfig(configFilePath)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(agentConfig.DataDir(), gc.Equals, agent.DefaultDataDir)
	c.Assert(agentConfig.LogDir(), gc.Equals, agent.DefaultLogDir)
	c.Assert(agentConfig.Jobs(), gc.HasLen, 0)
	c.Assert(agentConfig.Value("SHARED_STORAGE_ADDR"), gc.Equals, "blah")
	c.Assert(agentConfig.Value("SHARED_STORAGE_DIR"), gc.Equals, expectedSharedStorageDir)
	c.Assert(agentConfig.Value(agent.Namespace), gc.Equals, "")
	c.Assert(agentConfig.Value(agent.AgentServiceName), gc.Equals, "")
	c.Assert(agentConfig.Value(agent.ContainerType), gc.Equals, "")
}
func (s *migrateLocalProviderAgentConfigSuite) TestMigrateStateServer(c *gc.C) {
	s.primeConfig(c, s.State, state.JobManageEnviron, names.NewMachineTag("0"))
	err := upgrades.MigrateLocalProviderAgentConfig(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	err = s.config.Write()
	c.Assert(err, jc.ErrorIsNil)
	s.assertConfigProcessed(c)
}

func (s *migrateLocalProviderAgentConfigSuite) TestMigrateNonLocalEnvNotDone(c *gc.C) {
	s.PatchValue(upgrades.IsLocalEnviron, func(_ *config.Config) bool { return false })
	s.primeConfig(c, s.State, state.JobManageEnviron, names.NewMachineTag("0"))
	err := upgrades.MigrateLocalProviderAgentConfig(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	err = s.config.Write()
	c.Assert(err, jc.ErrorIsNil)
	s.assertConfigNotProcessed(c)
}

func (s *migrateLocalProviderAgentConfigSuite) TestMigrateWithoutStateConnectionNotDone(c *gc.C) {
	s.primeConfig(c, nil, state.JobManageEnviron, names.NewMachineTag("0"))
	err := upgrades.MigrateLocalProviderAgentConfig(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	err = s.config.Write()
	c.Assert(err, jc.ErrorIsNil)
	s.assertConfigNotProcessed(c)
}

func (s *migrateLocalProviderAgentConfigSuite) TestIdempotent(c *gc.C) {
	s.primeConfig(c, s.State, state.JobManageEnviron, names.NewMachineTag("0"))
	err := upgrades.MigrateLocalProviderAgentConfig(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	err = s.config.Write()
	c.Assert(err, jc.ErrorIsNil)
	s.assertConfigProcessed(c)

	err = upgrades.MigrateLocalProviderAgentConfig(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	err = s.config.Write()
	c.Assert(err, jc.ErrorIsNil)
	s.assertConfigProcessed(c)
}

type migrateAgentEnvUUIDSuite struct {
	jujutesting.JujuConnSuite
	machine  *state.Machine
	password string
	ctx      *mockContext
}

var _ = gc.Suite(&migrateAgentEnvUUIDSuite{})

func (s *migrateAgentEnvUUIDSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.PatchValue(&agent.DefaultLogDir, c.MkDir())
	s.PatchValue(&agent.DefaultDataDir, c.MkDir())
	s.primeConfig(c)
}

func (s *migrateAgentEnvUUIDSuite) primeConfig(c *gc.C) {
	s.machine, s.password = s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Nonce: "a nonce",
	})
	initialConfig, err := agent.NewAgentConfig(agent.AgentConfigParams{
		Tag:               s.machine.Tag(),
		Password:          s.password,
		CACert:            testing.CACert,
		StateAddresses:    []string{"localhost:1111"},
		DataDir:           agent.DefaultDataDir,
		LogDir:            agent.DefaultLogDir,
		UpgradedToVersion: version.MustParse("1.22.0"),
		Environment:       s.State.EnvironTag(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(initialConfig.Write(), gc.IsNil)

	apiState, _ := s.OpenAPIAsNewMachine(c)
	s.ctx = &mockContext{
		realAgentConfig: initialConfig,
		apiState:        apiState,
		state:           s.State,
	}
}

func (s *migrateAgentEnvUUIDSuite) removeEnvUUIDFromAgentConfig(c *gc.C) {
	// Read the file in as simple map[string]interface{} and delete
	// the element, and write it back out again.

	// First step, read the file contents.
	filename := agent.ConfigPath(agent.DefaultDataDir, s.machine.Tag())
	data, err := ioutil.ReadFile(filename)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("Data in:\n\n%s\n", data)

	// Parse it into the map.
	var content map[string]interface{}
	err = goyaml.Unmarshal(data, &content)
	c.Assert(err, jc.ErrorIsNil)

	// Remove the environment value, and marshal back into bytes.
	delete(content, "environment")
	data, err = goyaml.Marshal(content)
	c.Assert(err, jc.ErrorIsNil)

	// Write the yaml back out remembering to add the format prefix.
	data = append([]byte("# format 1.18\n"), data...)
	c.Logf("Data out:\n\n%s\n", data)
	err = ioutil.WriteFile(filename, data, 0644)
	c.Assert(err, jc.ErrorIsNil)

	// Reset test attributes.
	cfg, err := agent.ReadConfig(filename)
	c.Assert(err, jc.ErrorIsNil)
	s.ctx.realAgentConfig = cfg
}

func (s *migrateAgentEnvUUIDSuite) TestAgentEnvironmentUUID(c *gc.C) {
	c.Assert(s.ctx.realAgentConfig.Environment(), gc.Equals, s.State.EnvironTag())
}

func (s *migrateAgentEnvUUIDSuite) TestRemoveFuncWorks(c *gc.C) {
	s.removeEnvUUIDFromAgentConfig(c)
	c.Assert(s.ctx.realAgentConfig.Environment().Id(), gc.Equals, "")
}

func (s *migrateAgentEnvUUIDSuite) TestMigrationStep(c *gc.C) {
	s.removeEnvUUIDFromAgentConfig(c)
	upgrades.AddEnvironmentUUIDToAgentConfig(s.ctx)
	c.Assert(s.ctx.realAgentConfig.Environment(), gc.Equals, s.State.EnvironTag())
}

func (s *migrateAgentEnvUUIDSuite) TestMigrationStepIdempotent(c *gc.C) {
	s.removeEnvUUIDFromAgentConfig(c)
	upgrades.AddEnvironmentUUIDToAgentConfig(s.ctx)
	upgrades.AddEnvironmentUUIDToAgentConfig(s.ctx)
	c.Assert(s.ctx.realAgentConfig.Environment(), gc.Equals, s.State.EnvironTag())
}
