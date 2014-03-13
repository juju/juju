// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"path/filepath"

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

	rootDir string
	dataDir string
	logDir  string
}

var _ = gc.Suite(&migrateLocalProviderAgentConfigSuite{})

func (s *migrateLocalProviderAgentConfigSuite) primeConfig(c *gc.C, job state.MachineJob, tag, rootDir string) {
	s.rootDir = rootDir
	s.dataDir = filepath.Join(s.rootDir, agent.DefaultDataDir)
	s.logDir = filepath.Join(s.rootDir, agent.DefaultLogDir)

	initialConfig, err := agent.NewAgentConfig(agent.AgentConfigParams{
		Tag:               tag,
		Password:          "blah",
		CACert:            []byte(testing.CACert),
		StateAddresses:    []string{"localhost:1111"},
		DataDir:           s.dataDir,
		LogDir:            s.logDir,
		UpgradedToVersion: version.MustParse("1.16.0"),
		Values: map[string]string{
			"SHARED_STORAGE_ADDR": "blah",
			"SHARED_STORAGE_DIR":  "foo",
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
		"root-dir": filepath.Join(s.rootDir, "home", "juju", "local"),
	})
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(newCfg, envConfig)
	c.Assert(err, gc.IsNil)

}

func (s *migrateLocalProviderAgentConfigSuite) assertConfigProcessed(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	allAttrs := envConfig.AllAttrs()

	expectedDataDir := filepath.Join(s.rootDir, agent.DefaultDataDir)
	expectedLogDir := filepath.Join(s.rootDir, agent.DefaultLogDir)
	expectedJobs := []params.MachineJob{params.JobHostUnits}
	tag := s.ctx.AgentConfig().Tag()
	if tag == "machine-0" {
		expectedDataDir, _ = allAttrs["root-dir"].(string)
		expectedLogDir = filepath.Join(expectedDataDir, "log")
		expectedJobs = []params.MachineJob{params.JobManageEnviron}
	}
	namespace, _ := allAttrs["namespace"].(string)
	c.Assert(namespace, gc.Matches, ".+-mylocal")

	agentService := "juju-agent-" + namespace
	mongoService := "juju-db-" + namespace

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
	c.Assert(agentConfig.Value(agent.AgentServiceName), gc.Equals, agentService)
	c.Assert(agentConfig.Value(agent.MongoServiceName), gc.Equals, mongoService)
	c.Assert(allAttrs["container"], gc.Equals, "lxc")
}

func (s *migrateLocalProviderAgentConfigSuite) TestMigrateStateServer(c *gc.C) {
	s.primeConfig(c, state.JobManageEnviron, "machine-0", c.MkDir())
	err := upgrades.MigrateLocalProviderAgentConfig(s.ctx, upgrades.StateServer)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)
}

func (s *migrateLocalProviderAgentConfigSuite) TestMigrateHostMachine(c *gc.C) {
	s.PatchValue(&agent.DefaultDataDir, c.MkDir())
	s.PatchValue(&agent.DefaultLogDir, c.MkDir())
	s.primeConfig(c, state.JobHostUnits, "machine-42", "")
	err := upgrades.MigrateLocalProviderAgentConfig(s.ctx, upgrades.HostMachine)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)
}

func (s *migrateLocalProviderAgentConfigSuite) TestIdempotent(c *gc.C) {
	s.primeConfig(c, state.JobManageEnviron, "machine-0", c.MkDir())
	err := upgrades.MigrateLocalProviderAgentConfig(s.ctx, upgrades.StateServer)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)

	err = upgrades.MigrateLocalProviderAgentConfig(s.ctx, upgrades.StateServer)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)
}
