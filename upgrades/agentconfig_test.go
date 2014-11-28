// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"os"
	"path/filepath"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
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
