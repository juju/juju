// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"io/ioutil"
	"net"
	"path/filepath"

	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type bootstrapSuite struct {
	testing.BaseSuite
	mgoInst gitjujutesting.MgoInstance
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	// Don't use MgoSuite, because we need to ensure
	// we have a fresh mongo for each test case.
	s.mgoInst.EnableAuth = true
	err := s.mgoInst.Start(testing.Certs)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bootstrapSuite) TearDownTest(c *gc.C) {
	s.mgoInst.Destroy()
	s.BaseSuite.TearDownTest(c)
}

func (s *bootstrapSuite) TestInitializeState(c *gc.C) {
	dataDir := c.MkDir()

	lxcFakeNetConfig := filepath.Join(c.MkDir(), "lxc-net")
	netConf := []byte(`
  # comments ignored
LXC_BR= ignored
LXC_ADDR = "fooo"
LXC_BRIDGE="foobar" # detected
anything else ignored
LXC_BRIDGE="ignored"`[1:])
	err := ioutil.WriteFile(lxcFakeNetConfig, netConf, 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&network.InterfaceByNameAddrs, func(name string) ([]net.Addr, error) {
		c.Assert(name, gc.Equals, "foobar")
		return []net.Addr{
			&net.IPAddr{IP: net.IPv4(10, 0, 3, 1)},
			&net.IPAddr{IP: net.IPv4(10, 0, 3, 4)},
		}, nil
	})
	s.PatchValue(&network.LXCNetDefaultConfig, lxcFakeNetConfig)

	pwHash := utils.UserPasswordHash(testing.DefaultMongoPassword, utils.CompatSalt)
	configParams := agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: dataDir},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: version.Current,
		StateAddresses:    []string{s.mgoInst.Addr()},
		CACert:            testing.CACert,
		Password:          pwHash,
		Model:             testing.ModelTag,
	}
	servingInfo := params.StateServingInfo{
		Cert:           testing.ServerCert,
		PrivateKey:     testing.ServerKey,
		CAPrivateKey:   testing.CAKey,
		APIPort:        1234,
		StatePort:      s.mgoInst.Port(),
		SystemIdentity: "def456",
	}

	cfg, err := agent.NewStateMachineConfig(configParams, servingInfo)
	c.Assert(err, jc.ErrorIsNil)

	_, available := cfg.StateServingInfo()
	c.Assert(available, jc.IsTrue)
	expectBootstrapConstraints := constraints.MustParse("mem=1024M")
	expectModelConstraints := constraints.MustParse("mem=512M")
	expectHW := instance.MustParseHardware("mem=2048M")
	initialAddrs := network.NewAddresses(
		"zeroonetwothree",
		"0.1.2.3",
		"10.0.3.1", // lxc bridge address filtered.
		"10.0.3.4", // lxc bridge address filtered (-"-).
		"10.0.3.3", // not a lxc bridge address
	)
	mcfg := agent.BootstrapMachineConfig{
		Addresses:            initialAddrs,
		BootstrapConstraints: expectBootstrapConstraints,
		ModelConstraints:     expectModelConstraints,
		Jobs:                 []multiwatcher.MachineJob{multiwatcher.JobManageModel},
		InstanceId:           "i-bootstrap",
		Characteristics:      expectHW,
		SharedSecret:         "abc123",
	}
	filteredAddrs := network.NewAddresses(
		"zeroonetwothree",
		"0.1.2.3",
		"10.0.3.3",
	)
	envAttrs := dummy.SampleConfig().Delete("admin-secret").Merge(testing.Attrs{
		"agent-version": version.Current.String(),
		"state-id":      "1", // needed so policy can Open config
	})
	envCfg, err := config.New(config.NoDefaults, envAttrs)
	c.Assert(err, jc.ErrorIsNil)

	adminUser := names.NewLocalUserTag("agent-admin")
	st, m, err := agent.InitializeState(adminUser, cfg, envCfg, mcfg, mongo.DefaultDialOpts(), environs.NewStatePolicy())
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	err = cfg.Write()
	c.Assert(err, jc.ErrorIsNil)

	// Check that the environment has been set up.
	env, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	uuid, ok := envCfg.UUID()
	c.Assert(ok, jc.IsTrue)
	c.Assert(env.UUID(), gc.Equals, uuid)

	// Check that initial admin user has been set up correctly.
	modelTag := env.Tag().(names.ModelTag)
	s.assertCanLogInAsAdmin(c, modelTag, pwHash)
	user, err := st.User(env.Owner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid(testing.DefaultMongoPassword), jc.IsTrue)

	// Check that model configuration has been added, and
	// model constraints set.
	newEnvCfg, err := st.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newEnvCfg.AllAttrs(), gc.DeepEquals, envCfg.AllAttrs())
	gotModelConstraints, err := st.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotModelConstraints, gc.DeepEquals, expectModelConstraints)

	// Check that the bootstrap machine looks correct.
	c.Assert(m.Id(), gc.Equals, "0")
	c.Assert(m.Jobs(), gc.DeepEquals, []state.MachineJob{state.JobManageModel})
	c.Assert(m.Series(), gc.Equals, series.HostSeries())
	c.Assert(m.CheckProvisioned(agent.BootstrapNonce), jc.IsTrue)
	c.Assert(m.Addresses(), jc.DeepEquals, filteredAddrs)
	gotBootstrapConstraints, err := m.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotBootstrapConstraints, gc.DeepEquals, expectBootstrapConstraints)
	c.Assert(err, jc.ErrorIsNil)
	gotHW, err := m.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*gotHW, gc.DeepEquals, expectHW)

	// Check that the API host ports are initialised correctly.
	apiHostPorts, err := st.APIHostPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiHostPorts, jc.DeepEquals, [][]network.HostPort{
		network.AddressesWithPort(filteredAddrs, 1234),
	})

	// Check that the state serving info is initialised correctly.
	stateServingInfo, err := st.StateServingInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateServingInfo, jc.DeepEquals, state.StateServingInfo{
		APIPort:        1234,
		StatePort:      s.mgoInst.Port(),
		Cert:           testing.ServerCert,
		PrivateKey:     testing.ServerKey,
		CAPrivateKey:   testing.CAKey,
		SharedSecret:   "abc123",
		SystemIdentity: "def456",
	})

	// Check that the machine agent's config has been written
	// and that we can use it to connect to the state.
	machine0 := names.NewMachineTag("0")
	newCfg, err := agent.ReadConfig(agent.ConfigPath(dataDir, machine0))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newCfg.Tag(), gc.Equals, machine0)
	c.Assert(agent.Password(newCfg), gc.Not(gc.Equals), pwHash)
	c.Assert(agent.Password(newCfg), gc.Not(gc.Equals), testing.DefaultMongoPassword)
	info, ok := cfg.MongoInfo()
	c.Assert(ok, jc.IsTrue)
	st1, err := state.Open(newCfg.Model(), info, mongo.DefaultDialOpts(), environs.NewStatePolicy())
	c.Assert(err, jc.ErrorIsNil)
	defer st1.Close()
}

func (s *bootstrapSuite) TestInitializeStateWithStateServingInfoNotAvailable(c *gc.C) {
	configParams := agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: c.MkDir()},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: version.Current,
		StateAddresses:    []string{s.mgoInst.Addr()},
		CACert:            testing.CACert,
		Password:          "fake",
		Model:             testing.ModelTag,
	}
	cfg, err := agent.NewAgentConfig(configParams)
	c.Assert(err, jc.ErrorIsNil)

	_, available := cfg.StateServingInfo()
	c.Assert(available, jc.IsFalse)

	adminUser := names.NewLocalUserTag("agent-admin")
	_, _, err = agent.InitializeState(adminUser, cfg, nil, agent.BootstrapMachineConfig{}, mongo.DefaultDialOpts(), environs.NewStatePolicy())
	// InitializeState will fail attempting to get the api port information
	c.Assert(err, gc.ErrorMatches, "state serving information not available")
}

func (s *bootstrapSuite) TestInitializeStateFailsSecondTime(c *gc.C) {
	dataDir := c.MkDir()

	pwHash := utils.UserPasswordHash(testing.DefaultMongoPassword, utils.CompatSalt)
	configParams := agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: dataDir},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: version.Current,
		StateAddresses:    []string{s.mgoInst.Addr()},
		CACert:            testing.CACert,
		Password:          pwHash,
		Model:             testing.ModelTag,
	}
	cfg, err := agent.NewAgentConfig(configParams)
	c.Assert(err, jc.ErrorIsNil)
	cfg.SetStateServingInfo(params.StateServingInfo{
		APIPort:        5555,
		StatePort:      s.mgoInst.Port(),
		Cert:           "foo",
		PrivateKey:     "bar",
		SharedSecret:   "baz",
		SystemIdentity: "qux",
	})
	expectHW := instance.MustParseHardware("mem=2048M")
	mcfg := agent.BootstrapMachineConfig{
		BootstrapConstraints: constraints.MustParse("mem=1024M"),
		Jobs:                 []multiwatcher.MachineJob{multiwatcher.JobManageModel},
		InstanceId:           "i-bootstrap",
		Characteristics:      expectHW,
	}
	envAttrs := dummy.SampleConfig().Delete("admin-secret").Merge(testing.Attrs{
		"agent-version": version.Current.String(),
		"state-id":      "1", // needed so policy can Open config
	})
	envCfg, err := config.New(config.NoDefaults, envAttrs)
	c.Assert(err, jc.ErrorIsNil)

	adminUser := names.NewLocalUserTag("agent-admin")
	st, _, err := agent.InitializeState(adminUser, cfg, envCfg, mcfg, mongo.DefaultDialOpts(), environs.NewStatePolicy())
	c.Assert(err, jc.ErrorIsNil)
	st.Close()

	st, _, err = agent.InitializeState(adminUser, cfg, envCfg, mcfg, mongo.DefaultDialOpts(), environs.NewStatePolicy())
	if err == nil {
		st.Close()
	}
	c.Assert(err, gc.ErrorMatches, "failed to initialize mongo admin user: cannot set admin password: not authorized .*")
}

func (s *bootstrapSuite) TestMachineJobFromParams(c *gc.C) {
	var tests = []struct {
		name multiwatcher.MachineJob
		want state.MachineJob
		err  string
	}{{
		name: multiwatcher.JobHostUnits,
		want: state.JobHostUnits,
	}, {
		name: multiwatcher.JobManageModel,
		want: state.JobManageModel,
	}, {
		name: multiwatcher.JobManageNetworking,
		want: state.JobManageNetworking,
	}, {
		name: "invalid",
		want: -1,
		err:  `invalid machine job "invalid"`,
	}}
	for _, test := range tests {
		got, err := agent.MachineJobFromParams(test.name)
		if err != nil {
			c.Check(err, gc.ErrorMatches, test.err)
		}
		c.Check(got, gc.Equals, test.want)
	}
}

func (s *bootstrapSuite) assertCanLogInAsAdmin(c *gc.C, modelTag names.ModelTag, password string) {
	info := &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{s.mgoInst.Addr()},
			CACert: testing.CACert,
		},
		Tag:      nil, // admin user
		Password: password,
	}
	st, err := state.Open(modelTag, info, mongo.DefaultDialOpts(), environs.NewStatePolicy())
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	_, err = st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
}
