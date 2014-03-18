// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/upgrades"
	"launchpad.net/juju-core/version"
)

type migrateLocalProviderAgentConfigSuite struct {
	jujutesting.JujuConnSuite
	ctx upgrades.Context
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
	s.PatchValue(upgrades.ChownPath, func(path, username string) error { return nil })
}

func (s *migrateLocalProviderAgentConfigSuite) primeConfig(c *gc.C, job state.MachineJob, tag string) {
	rootDir := c.MkDir()
	sharedStorageDir := filepath.Join(rootDir, "shared-storage")
	c.Assert(os.MkdirAll(sharedStorageDir, 0755), gc.IsNil)
	localLogDir := filepath.Join(rootDir, "log")
	c.Assert(os.MkdirAll(localLogDir, 0755), gc.IsNil)

	initialConfig, err := agent.NewAgentConfig(agent.AgentConfigParams{
		Tag:               tag,
		Password:          "blah",
		CACert:            []byte(testing.CACert),
		StateAddresses:    []string{"localhost:1111"},
		DataDir:           agent.DefaultDataDir,
		LogDir:            agent.DefaultLogDir,
		UpgradedToVersion: version.MustParse("1.16.0"),
		Values: map[string]string{
			"SHARED_STORAGE_ADDR": "blah",
			"SHARED_STORAGE_DIR":  sharedStorageDir,
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(initialConfig.Write(), gc.IsNil)

	apiState, _ := s.OpenAPIAsNewMachine(c, job)
	s.ctx = &mockContext{
		realAgentConfig: initialConfig,
		apiState:        apiState,
		state:           s.State,
	}

	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	// Add in old environment settings.
	newCfg, err := envConfig.Apply(map[string]interface{}{
		"type":     "local",
		"name":     "mylocal",
		"root-dir": rootDir,
	})
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(newCfg, envConfig)
	c.Assert(err, gc.IsNil)
}

func (s *migrateLocalProviderAgentConfigSuite) assertConfigProcessed(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	allAttrs := envConfig.AllAttrs()

	namespace, _ := allAttrs["namespace"].(string)
	c.Assert(namespace, gc.Equals, "user-mylocal")
	container, _ := allAttrs["container"].(string)
	c.Assert(container, gc.Equals, "lxc")

	expectedDataDir, _ := allAttrs["root-dir"].(string)
	expectedSharedStorageDir := filepath.Join(expectedDataDir, "shared-storage")
	_, err = os.Lstat(expectedSharedStorageDir)
	c.Assert(err, gc.NotNil)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	expectedLogDir := filepath.Join(*upgrades.RootLogDir, "juju-"+namespace)
	expectedJobs := []params.MachineJob{params.JobManageEnviron}
	tag := s.ctx.AgentConfig().Tag()

	// We need to read the actual migrated agent config.
	configFilePath := agent.ConfigPath(expectedDataDir, tag)
	agentConfig, err := agent.ReadConf(configFilePath)
	c.Assert(err, gc.IsNil)

	c.Assert(agentConfig.DataDir(), gc.Equals, expectedDataDir)
	c.Assert(agentConfig.LogDir(), gc.Equals, expectedLogDir)
	c.Assert(agentConfig.Jobs(), gc.DeepEquals, expectedJobs)
	c.Assert(agentConfig.Value("SHARED_STORAGE_ADDR"), gc.Equals, "")
	c.Assert(agentConfig.Value("SHARED_STORAGE_DIR"), gc.Equals, "")
	c.Assert(agentConfig.Value(agent.Namespace), gc.Equals, namespace)
	agentService := "juju-agent-user-mylocal"
	mongoService := "juju-db-user-mylocal"
	c.Assert(agentConfig.Value(agent.AgentServiceName), gc.Equals, agentService)
	c.Assert(agentConfig.Value(agent.MongoServiceName), gc.Equals, mongoService)
	c.Assert(agentConfig.Value(agent.ContainerType), gc.Equals, "")
}

func (s *migrateLocalProviderAgentConfigSuite) TestMigrateStateServer(c *gc.C) {
	s.primeConfig(c, state.JobManageEnviron, "machine-0")
	err := upgrades.MigrateLocalProviderAgentConfig(s.ctx)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)
}

func (s *migrateLocalProviderAgentConfigSuite) TestIdempotent(c *gc.C) {
	s.primeConfig(c, state.JobManageEnviron, "machine-0")
	err := upgrades.MigrateLocalProviderAgentConfig(s.ctx)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)

	err = upgrades.MigrateLocalProviderAgentConfig(s.ctx)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)
}
