// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

type bootstrapSuite struct {
	testing.BaseSuite
	testing.MgoSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *bootstrapSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
}

func (s *bootstrapSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *bootstrapSuite) TestInitializeState(c *gc.C) {
	dataDir := c.MkDir()

	pwHash := utils.UserPasswordHash(testing.DefaultMongoPassword, utils.CompatSalt)
	configParams := agent.AgentConfigParams{
		DataDir:           dataDir,
		Tag:               "machine-0",
		UpgradedToVersion: version.Current.Number,
		StateAddresses:    []string{testing.MgoServer.Addr()},
		CACert:            testing.CACert,
		Password:          pwHash,
	}
	servingInfo := params.StateServingInfo{
		Cert:           testing.ServerCert,
		PrivateKey:     testing.ServerKey,
		APIPort:        1234,
		StatePort:      testing.MgoServer.Port(),
		SystemIdentity: "def456",
	}

	cfg, err := agent.NewStateMachineConfig(configParams, servingInfo)
	c.Assert(err, gc.IsNil)

	_, available := cfg.StateServingInfo()
	c.Assert(available, gc.Equals, true)
	expectConstraints := constraints.MustParse("mem=1024M")
	expectHW := instance.MustParseHardware("mem=2048M")
	mcfg := agent.BootstrapMachineConfig{
		Addresses:       instance.NewAddresses("0.1.2.3", "zeroonetwothree"),
		Constraints:     expectConstraints,
		Jobs:            []params.MachineJob{params.JobHostUnits},
		InstanceId:      "i-bootstrap",
		Characteristics: expectHW,
		SharedSecret:    "abc123",
	}
	envAttrs := dummy.SampleConfig().Delete("admin-secret").Merge(testing.Attrs{
		"agent-version": version.Current.Number.String(),
		"state-id":      "1", // needed so policy can Open config
	})
	envCfg, err := config.New(config.NoDefaults, envAttrs)
	c.Assert(err, gc.IsNil)

	st, m, err := agent.InitializeState(cfg, envCfg, mcfg, state.DialOpts{}, environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	defer st.Close()

	err = cfg.Write()
	c.Assert(err, gc.IsNil)

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
	c.Assert(m.Addresses(), gc.DeepEquals, mcfg.Addresses)
	gotConstraints, err := m.Constraints()
	c.Assert(err, gc.IsNil)
	c.Assert(gotConstraints, gc.DeepEquals, expectConstraints)
	c.Assert(err, gc.IsNil)
	gotHW, err := m.HardwareCharacteristics()
	c.Assert(err, gc.IsNil)
	c.Assert(*gotHW, gc.DeepEquals, expectHW)
	gotAddrs := m.Addresses()
	c.Assert(gotAddrs, gc.DeepEquals, mcfg.Addresses)

	// Check that the API host ports are initialised correctly.
	apiHostPorts, err := st.APIHostPorts()
	c.Assert(err, gc.IsNil)
	c.Assert(apiHostPorts, gc.DeepEquals, [][]instance.HostPort{
		instance.AddressesWithPort(mcfg.Addresses, 1234),
	})

	// Check that the state serving info is initialised correctly.
	stateServingInfo, err := st.StateServingInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(stateServingInfo, jc.DeepEquals, params.StateServingInfo{
		APIPort:        1234,
		StatePort:      testing.MgoServer.Port(),
		Cert:           testing.ServerCert,
		PrivateKey:     testing.ServerKey,
		SharedSecret:   "abc123",
		SystemIdentity: "def456",
	})

	// Check that the machine agent's config has been written
	// and that we can use it to connect to the state.
	newCfg, err := agent.ReadConfig(agent.ConfigPath(dataDir, "machine-0"))
	c.Assert(err, gc.IsNil)
	c.Assert(newCfg.Tag(), gc.Equals, "machine-0")
	c.Assert(agent.Password(newCfg), gc.Not(gc.Equals), pwHash)
	c.Assert(agent.Password(newCfg), gc.Not(gc.Equals), testing.DefaultMongoPassword)
	info, ok := cfg.StateInfo()
	c.Assert(ok, jc.IsTrue)
	st1, err := state.Open(info, state.DialOpts{}, environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	defer st1.Close()
}

func (s *bootstrapSuite) TestInitializeStateWithStateServingInfoNotAvailable(c *gc.C) {
	configParams := agent.AgentConfigParams{
		DataDir:           c.MkDir(),
		Tag:               "machine-0",
		UpgradedToVersion: version.Current.Number,
		StateAddresses:    []string{testing.MgoServer.Addr()},
		CACert:            testing.CACert,
		Password:          "fake",
	}
	cfg, err := agent.NewAgentConfig(configParams)
	c.Assert(err, gc.IsNil)

	_, available := cfg.StateServingInfo()
	c.Assert(available, gc.Equals, false)

	_, _, err = agent.InitializeState(cfg, nil, agent.BootstrapMachineConfig{}, state.DialOpts{}, environs.NewStatePolicy())
	// InitializeState will fail attempting to get the api port information
	c.Assert(err, gc.ErrorMatches, "state serving information not available")
}

func (s *bootstrapSuite) TestInitializeStateFailsSecondTime(c *gc.C) {
	dataDir := c.MkDir()

	pwHash := utils.UserPasswordHash(testing.DefaultMongoPassword, utils.CompatSalt)
	configParams := agent.AgentConfigParams{
		DataDir:           dataDir,
		Tag:               "machine-0",
		UpgradedToVersion: version.Current.Number,
		StateAddresses:    []string{testing.MgoServer.Addr()},
		CACert:            testing.CACert,
		Password:          pwHash,
	}
	cfg, err := agent.NewAgentConfig(configParams)
	c.Assert(err, gc.IsNil)
	cfg.SetStateServingInfo(params.StateServingInfo{
		APIPort:        5555,
		StatePort:      testing.MgoServer.Port(),
		Cert:           "foo",
		PrivateKey:     "bar",
		SharedSecret:   "baz",
		SystemIdentity: "qux",
	})
	expectConstraints := constraints.MustParse("mem=1024M")
	expectHW := instance.MustParseHardware("mem=2048M")
	mcfg := agent.BootstrapMachineConfig{
		Constraints:     expectConstraints,
		Jobs:            []params.MachineJob{params.JobHostUnits},
		InstanceId:      "i-bootstrap",
		Characteristics: expectHW,
	}
	envAttrs := dummy.SampleConfig().Delete("admin-secret").Merge(testing.Attrs{
		"agent-version": version.Current.Number.String(),
		"state-id":      "1", // needed so policy can Open config
	})
	envCfg, err := config.New(config.NoDefaults, envAttrs)
	c.Assert(err, gc.IsNil)

	st, _, err := agent.InitializeState(cfg, envCfg, mcfg, state.DialOpts{}, environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	err = st.SetAdminMongoPassword("")
	c.Check(err, gc.IsNil)
	st.Close()

	st, _, err = agent.InitializeState(cfg, envCfg, mcfg, state.DialOpts{}, environs.NewStatePolicy())
	if err == nil {
		st.Close()
	}
	c.Assert(err, gc.ErrorMatches, "failed to initialize state: cannot create log collection: unauthorized mongo access: unauthorized")
}

func (*bootstrapSuite) assertCanLogInAsAdmin(c *gc.C, password string) {
	info := &state.Info{
		Addrs:    []string{testing.MgoServer.Addr()},
		CACert:   testing.CACert,
		Tag:      "",
		Password: password,
	}
	st, err := state.Open(info, state.DialOpts{}, environs.NewStatePolicy())
	c.Assert(err, gc.IsNil)
	defer st.Close()
	_, err = st.Machine("0")
	c.Assert(err, gc.IsNil)
}
