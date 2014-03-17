// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"path/filepath"
	"strings"

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
	s.PatchValue(&agent.DefaultDataDir, c.MkDir())
	s.PatchValue(&agent.DefaultLogDir, c.MkDir())
	s.PatchValue(upgrades.RootLogDir, c.MkDir())
}

func (s *migrateLocalProviderAgentConfigSuite) primeConfig(c *gc.C, job state.MachineJob, tag string) {
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
		"root-dir": c.MkDir(),
	})
	c.Assert(err, gc.IsNil)
	err = s.State.SetEnvironConfig(newCfg, envConfig)
	c.Assert(err, gc.IsNil)
}

func (s *migrateLocalProviderAgentConfigSuite) assertConfigProcessed(c *gc.C) {
	envConfig, err := s.State.EnvironConfig()
	c.Assert(err, gc.IsNil)
	allAttrs := envConfig.AllAttrs()

	expectedDataDir := filepath.Join(agent.DefaultDataDir)
	expectedLogDir := filepath.Join(agent.DefaultLogDir)
	expectedJobs := []params.MachineJob{params.JobHostUnits}
	tag := s.ctx.AgentConfig().Tag()

	namespace, _ := allAttrs["namespace"].(string)
	c.Assert(namespace, gc.Equals, "user-mylocal")
	container, _ := allAttrs["container"].(string)
	c.Assert(container, gc.Equals, "lxc")

	if tag == "machine-0" {
		expectedDataDir, _ = allAttrs["root-dir"].(string)
		expectedLogDir = filepath.Join(*upgrades.RootLogDir, "juju-"+namespace)
		expectedJobs = []params.MachineJob{params.JobManageEnviron}
	}

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
	switch {
	case tag == "machine-0":
		agentService := "juju-agent-user-mylocal"
		mongoService := "juju-db-user-mylocal"
		c.Assert(agentConfig.Value(agent.AgentServiceName), gc.Equals, agentService)
		c.Assert(agentConfig.Value(agent.MongoServiceName), gc.Equals, mongoService)
		c.Assert(agentConfig.Value(agent.ContainerType), gc.Equals, "")
	case strings.HasPrefix(tag, "machine-"):
		agentService := "jujud-" + tag
		c.Assert(agentConfig.Value(agent.AgentServiceName), gc.Equals, agentService)
		c.Assert(agentConfig.Value(agent.MongoServiceName), gc.Equals, "")
		c.Assert(agentConfig.Value(agent.ContainerType), gc.Equals, container)
	case strings.HasPrefix(tag, "unit-"):
		c.Assert(agentConfig.Value(agent.AgentServiceName), gc.Equals, "")
		c.Assert(agentConfig.Value(agent.MongoServiceName), gc.Equals, "")
		c.Assert(agentConfig.Value(agent.ContainerType), gc.Equals, container)
	}
}

func (s *migrateLocalProviderAgentConfigSuite) TestMigrateStateServer(c *gc.C) {
	s.primeConfig(c, state.JobManageEnviron, "machine-0")
	err := upgrades.MigrateLocalProviderAgentConfig(s.ctx, upgrades.StateServer)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)
}

func (s *migrateLocalProviderAgentConfigSuite) TestMigrateHostMachine(c *gc.C) {
	s.primeConfig(c, state.JobHostUnits, "machine-42")
	err := upgrades.MigrateLocalProviderAgentConfig(s.ctx, upgrades.HostMachine)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)
}

func (s *migrateLocalProviderAgentConfigSuite) TestMigrateHostMachineUnit(c *gc.C) {
	s.primeConfig(c, state.JobHostUnits, "unit-foo-0")
	err := upgrades.MigrateLocalProviderAgentConfig(s.ctx, upgrades.HostMachine)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)
}

func (s *migrateLocalProviderAgentConfigSuite) TestIdempotent(c *gc.C) {
	s.primeConfig(c, state.JobManageEnviron, "machine-0")
	err := upgrades.MigrateLocalProviderAgentConfig(s.ctx, upgrades.StateServer)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)

	err = upgrades.MigrateLocalProviderAgentConfig(s.ctx, upgrades.StateServer)
	c.Assert(err, gc.IsNil)
	s.assertConfigProcessed(c)
}
