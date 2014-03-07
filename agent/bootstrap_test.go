// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

type bootstrapSuite struct {
	testbase.LoggingSuite
	testing.MgoSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *bootstrapSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
}

func (s *bootstrapSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *bootstrapSuite) TestInitializeState(c *gc.C) {
	dataDir := c.MkDir()

	pwHash := utils.UserPasswordHash(testing.DefaultMongoPassword, utils.CompatSalt)
	cfg, err := agent.NewAgentConfig(agent.AgentConfigParams{
		DataDir:           dataDir,
		Tag:               "machine-0",
		UpgradedToVersion: version.Current.Number,
		StateAddresses:    []string{testing.MgoServer.Addr()},
		CACert:            []byte(testing.CACert),
		Password:          pwHash,
	})
	c.Assert(err, gc.IsNil)
	expectConstraints := constraints.MustParse("mem=1024M")
	expectHW := instance.MustParseHardware("mem=2048M")
	mcfg := agent.BootstrapMachineConfig{
		Constraints:     expectConstraints,
		Jobs:            []params.MachineJob{params.JobHostUnits},
		InstanceId:      "i-bootstrap",
		Characteristics: expectHW,
	}
	envAttrs := testing.FakeConfig().Delete("admin-secret").Merge(testing.Attrs{
		"agent-version": version.Current.Number.String(),
	})
	envCfg, err := config.New(config.NoDefaults, envAttrs)
	c.Assert(err, gc.IsNil)

	st, m, err := cfg.InitializeState(envCfg, mcfg, state.DialOpts{}, environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	defer st.Close()

	// Check that initial admin user has been set up correctly.
	s.assertCanLogInAsAdmin(c, pwHash)
	user, err := st.User("admin")
	c.Assert(err, gc.IsNil)
	c.Assert(user.PasswordValid(testing.DefaultMongoPassword), jc.IsTrue)

	// Check that environment configuration has been added.
	newEnvCfg, err := st.EnvironConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(newEnvCfg.AllAttrs(), gc.DeepEquals, envCfg.AllAttrs())

	// Check that the bootstrap machine looks correct.
	c.Assert(m.Id(), gc.Equals, "0")
	c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{state.JobHostUnits})
	c.Assert(m.Series(), gc.Equals, version.Current.Series)
	c.Assert(m.CheckProvisioned(state.BootstrapNonce), jc.IsTrue)
	gotConstraints, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(gotConstraints, gc.DeepEquals, expectConstraints)
	c.Assert(err, gc.IsNil)
	gotHW, err := m.HardwareCharacteristics()
	c.Assert(err, gc.IsNil)
	c.Assert(*gotHW, gc.DeepEquals, expectHW)

	// Check that the machine agent's config has been written
	// and that we can use it to connect to the state.
	newCfg, err := agent.ReadConf(agent.ConfigPath(dataDir, "machine-0"))
	c.Assert(err, gc.IsNil)
	c.Assert(newCfg.Tag(), gc.Equals, "machine-0")
	c.Assert(agent.Password(newCfg), gc.Not(gc.Equals), pwHash)
	c.Assert(agent.Password(newCfg), gc.Not(gc.Equals), testing.DefaultMongoPassword)
	st1, err := cfg.OpenState(environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	defer st1.Close()
}

func (s *bootstrapSuite) TestInitializeStateFailsSecondTime(c *gc.C) {
	dataDir := c.MkDir()
	pwHash := utils.UserPasswordHash(testing.DefaultMongoPassword, utils.CompatSalt)
	cfg, err := agent.NewAgentConfig(agent.AgentConfigParams{
		DataDir:           dataDir,
		Tag:               "machine-0",
		UpgradedToVersion: version.Current.Number,
		StateAddresses:    []string{testing.MgoServer.Addr()},
		CACert:            []byte(testing.CACert),
		Password:          pwHash,
	})
	c.Assert(err, gc.IsNil)
	expectConstraints := constraints.MustParse("mem=1024M")
	expectHW := instance.MustParseHardware("mem=2048M")
	mcfg := agent.BootstrapMachineConfig{
		Constraints:     expectConstraints,
		Jobs:            []params.MachineJob{params.JobHostUnits},
		InstanceId:      "i-bootstrap",
		Characteristics: expectHW,
	}
	envAttrs := testing.FakeConfig().Delete("admin-secret").Merge(testing.Attrs{
		"agent-version": version.Current.Number.String(),
	})
	envCfg, err := config.New(config.NoDefaults, envAttrs)
	c.Assert(err, gc.IsNil)

	st, _, err := cfg.InitializeState(envCfg, mcfg, state.DialOpts{}, environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	err = st.SetAdminMongoPassword("")
	c.Check(err, gc.IsNil)
	st.Close()

	st, _, err = cfg.InitializeState(envCfg, mcfg, state.DialOpts{}, environs.NewStatePolicy())
	if err == nil {
		st.Close()
	}
	c.Assert(err, gc.ErrorMatches, "cannot initialize bootstrap user: user already exists")
}

func (*bootstrapSuite) assertCanLogInAsAdmin(c *gc.C, password string) {
	info := &state.Info{
		Addrs:    []string{testing.MgoServer.Addr()},
		CACert:   []byte(testing.CACert),
		Tag:      "",
		Password: password,
	}
	st, err := state.Open(info, state.DialOpts{}, environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	defer st.Close()
	_, err = st.Machine("0")
	c.Assert(err, gc.IsNil)
}
