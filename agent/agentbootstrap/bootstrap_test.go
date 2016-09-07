// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap_test

import (
	"io/ioutil"
	"net"
	"path/filepath"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agentbootstrap"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
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
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&network.InterfaceByNameAddrs, func(name string) ([]net.Addr, error) {
		if name == "foobar" {
			// The addresses on the LXC bridge
			return []net.Addr{
				&net.IPAddr{IP: net.IPv4(10, 0, 3, 1)},
				&net.IPAddr{IP: net.IPv4(10, 0, 3, 4)},
			}, nil
		} else if name == network.DefaultLXDBridge {
			// The addresses on the LXD bridge
			return []net.Addr{
				&net.IPAddr{IP: net.IPv4(10, 0, 4, 1)},
				&net.IPAddr{IP: net.IPv4(10, 0, 4, 4)},
			}, nil
		}
		c.Fatalf("unknown bridge in testing: %v", name)
		return nil, nil
	})
	s.PatchValue(&network.LXCNetDefaultConfig, lxcFakeNetConfig)

	configParams := agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: dataDir},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: jujuversion.Current,
		StateAddresses:    []string{s.mgoInst.Addr()},
		CACert:            testing.CACert,
		Password:          testing.DefaultMongoPassword,
		Controller:        testing.ControllerTag,
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
		"10.0.4.1", // lxd bridge address filtered.
		"10.0.4.4", // lxd bridge address filtered.
		"10.0.4.5", // not an lxd bridge address
	)
	filteredAddrs := network.NewAddresses(
		"zeroonetwothree",
		"0.1.2.3",
		"10.0.3.3",
		"10.0.4.5",
	)

	modelAttrs := testing.FakeConfig().Merge(testing.Attrs{
		"agent-version":  jujuversion.Current.String(),
		"not-for-hosted": "foo",
	})
	modelCfg, err := config.New(config.NoDefaults, modelAttrs)
	c.Assert(err, jc.ErrorIsNil)
	controllerCfg := testing.FakeControllerConfig()

	hostedModelUUID := utils.MustNewUUID().String()
	hostedModelConfigAttrs := map[string]interface{}{
		"name": "hosted",
		"uuid": hostedModelUUID,
	}
	controllerInheritedConfig := map[string]interface{}{
		"apt-mirror": "http://mirror",
		"no-proxy":   "value",
	}
	regionConfig := cloud.RegionConfig{
		"some-region": cloud.Attrs{
			"no-proxy": "a-value",
		},
	}
	var envProvider fakeProvider
	args := agentbootstrap.InitializeStateParams{
		StateInitializationParams: instancecfg.StateInitializationParams{
			BootstrapMachineConstraints:             expectBootstrapConstraints,
			BootstrapMachineInstanceId:              "i-bootstrap",
			BootstrapMachineHardwareCharacteristics: &expectHW,
			ControllerCloud: cloud.Cloud{
				Type:         "dummy",
				AuthTypes:    []cloud.AuthType{cloud.EmptyAuthType},
				Regions:      []cloud.Region{{Name: "dummy-region"}},
				RegionConfig: regionConfig,
			},
			ControllerCloudName:       "dummy",
			ControllerCloudRegion:     "dummy-region",
			ControllerConfig:          controllerCfg,
			ControllerModelConfig:     modelCfg,
			ModelConstraints:          expectModelConstraints,
			ControllerInheritedConfig: controllerInheritedConfig,
			HostedModelConfig:         hostedModelConfigAttrs,
		},
		BootstrapMachineAddresses: initialAddrs,
		BootstrapMachineJobs:      []multiwatcher.MachineJob{multiwatcher.JobManageModel},
		SharedSecret:              "abc123",
		Provider: func(t string) (environs.EnvironProvider, error) {
			c.Assert(t, gc.Equals, "dummy")
			return &envProvider, nil
		},
		StorageProviderRegistry: provider.CommonStorageProviders(),
	}

	adminUser := names.NewLocalUserTag("agent-admin")
	st, m, err := agentbootstrap.InitializeState(
		adminUser, cfg, args, mongotest.DialOpts(), state.NewPolicyFunc(nil),
	)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()

	err = cfg.Write()
	c.Assert(err, jc.ErrorIsNil)

	// Check that the environment has been set up.
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.UUID(), gc.Equals, modelCfg.UUID())

	// Check that initial admin user has been set up correctly.
	modelTag := model.Tag().(names.ModelTag)
	controllerTag := names.NewControllerTag(controllerCfg.ControllerUUID())
	s.assertCanLogInAsAdmin(c, modelTag, controllerTag, testing.DefaultMongoPassword)
	user, err := st.User(model.Owner())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid(testing.DefaultMongoPassword), jc.IsTrue)

	// Check controller config
	controllerCfg, err = st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerCfg, jc.DeepEquals, controller.Config{
		"controller-uuid":         testing.ControllerTag.Id(),
		"ca-cert":                 testing.CACert,
		"state-port":              1234,
		"api-port":                17777,
		"set-numa-control-policy": false,
	})

	// Check that controller model configuration has been added, and
	// model constraints set.
	newModelCfg, err := st.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	// Add in the cloud attributes.
	expectedCfg, err := config.New(config.UseDefaults, modelAttrs)
	c.Assert(err, jc.ErrorIsNil)
	expectedAttrs := expectedCfg.AllAttrs()
	expectedAttrs["apt-mirror"] = "http://mirror"
	expectedAttrs["no-proxy"] = "value"
	c.Assert(newModelCfg.AllAttrs(), jc.DeepEquals, expectedAttrs)

	gotModelConstraints, err := st.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotModelConstraints, gc.DeepEquals, expectModelConstraints)

	// Check that the hosted model has been added, model constraints
	// set, and its config contains the same authorized-keys as the
	// controller model.
	hostedModelSt, err := st.ForModel(names.NewModelTag(hostedModelUUID))
	c.Assert(err, jc.ErrorIsNil)
	defer hostedModelSt.Close()
	gotModelConstraints, err = hostedModelSt.ModelConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotModelConstraints, gc.DeepEquals, expectModelConstraints)
	hostedModel, err := hostedModelSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hostedModel.Name(), gc.Equals, "hosted")
	c.Assert(hostedModel.CloudRegion(), gc.Equals, "dummy-region")
	hostedCfg, err := hostedModelSt.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	_, hasUnexpected := hostedCfg.AllAttrs()["not-for-hosted"]
	c.Assert(hasUnexpected, jc.IsFalse)
	c.Assert(hostedCfg.AuthorizedKeys(), gc.Equals, newModelCfg.AuthorizedKeys())

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
	info, ok := cfg.MongoInfo()
	c.Assert(ok, jc.IsTrue)
	c.Assert(info.Password, gc.Not(gc.Equals), testing.DefaultMongoPassword)
	st1, err := state.Open(newCfg.Model(), newCfg.Controller(), info, mongotest.DialOpts(), nil)
	c.Assert(err, jc.ErrorIsNil)
	defer st1.Close()

	// Make sure that the hosted model Environ's Create method is called.
	envProvider.CheckCallNames(c,
		"PrepareConfig",
		"Validate",
		"Open",
		"Create",
	)
	envProvider.CheckCall(c, 2, "Open", environs.OpenParams{
		Cloud: environs.CloudSpec{
			Type:   "dummy",
			Name:   "dummy",
			Region: "dummy-region",
		},
		Config: hostedCfg,
	})
	envProvider.CheckCall(c, 3, "Create", environs.CreateParams{
		ControllerUUID: controllerCfg.ControllerUUID(),
	})
}

func (s *bootstrapSuite) TestInitializeStateWithStateServingInfoNotAvailable(c *gc.C) {
	configParams := agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: c.MkDir()},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: jujuversion.Current,
		StateAddresses:    []string{s.mgoInst.Addr()},
		CACert:            testing.CACert,
		Password:          "fake",
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
	}
	cfg, err := agent.NewAgentConfig(configParams)
	c.Assert(err, jc.ErrorIsNil)

	_, available := cfg.StateServingInfo()
	c.Assert(available, jc.IsFalse)

	args := agentbootstrap.InitializeStateParams{}

	adminUser := names.NewLocalUserTag("agent-admin")
	_, _, err = agentbootstrap.InitializeState(adminUser, cfg, args, mongotest.DialOpts(), nil)
	// InitializeState will fail attempting to get the api port information
	c.Assert(err, gc.ErrorMatches, "state serving information not available")
}

func (s *bootstrapSuite) TestInitializeStateFailsSecondTime(c *gc.C) {
	dataDir := c.MkDir()

	configParams := agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: dataDir},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: jujuversion.Current,
		StateAddresses:    []string{s.mgoInst.Addr()},
		CACert:            testing.CACert,
		Password:          testing.DefaultMongoPassword,
		Controller:        testing.ControllerTag,
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
	modelAttrs := dummy.SampleConfig().Delete("admin-secret").Merge(testing.Attrs{
		"agent-version": jujuversion.Current.String(),
	})
	modelCfg, err := config.New(config.NoDefaults, modelAttrs)
	c.Assert(err, jc.ErrorIsNil)

	hostedModelConfigAttrs := map[string]interface{}{
		"name": "hosted",
		"uuid": utils.MustNewUUID().String(),
	}

	args := agentbootstrap.InitializeStateParams{
		StateInitializationParams: instancecfg.StateInitializationParams{
			BootstrapMachineInstanceId: "i-bootstrap",
			ControllerCloud: cloud.Cloud{
				Type:      "dummy",
				AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			},
			ControllerCloudName:   "dummy",
			ControllerConfig:      testing.FakeControllerConfig(),
			ControllerModelConfig: modelCfg,
			HostedModelConfig:     hostedModelConfigAttrs,
		},
		BootstrapMachineJobs: []multiwatcher.MachineJob{multiwatcher.JobManageModel},
		SharedSecret:         "abc123",
		Provider: func(t string) (environs.EnvironProvider, error) {
			return &fakeProvider{}, nil
		},
		StorageProviderRegistry: provider.CommonStorageProviders(),
	}

	adminUser := names.NewLocalUserTag("agent-admin")
	st, _, err := agentbootstrap.InitializeState(
		adminUser, cfg, args, mongotest.DialOpts(), state.NewPolicyFunc(nil),
	)
	c.Assert(err, jc.ErrorIsNil)
	st.Close()

	st, _, err = agentbootstrap.InitializeState(
		adminUser, cfg, args, mongotest.DialOpts(), state.NewPolicyFunc(nil),
	)
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
		name: "invalid",
		want: -1,
		err:  `invalid machine job "invalid"`,
	}}
	for _, test := range tests {
		got, err := agentbootstrap.MachineJobFromParams(test.name)
		if err != nil {
			c.Check(err, gc.ErrorMatches, test.err)
		}
		c.Check(got, gc.Equals, test.want)
	}
}

func (s *bootstrapSuite) assertCanLogInAsAdmin(c *gc.C, modelTag names.ModelTag, controllerTag names.ControllerTag, password string) {
	info := &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{s.mgoInst.Addr()},
			CACert: testing.CACert,
		},
		Tag:      nil, // admin user
		Password: password,
	}
	st, err := state.Open(modelTag, controllerTag, info, mongotest.DialOpts(), state.NewPolicyFunc(nil))
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	_, err = st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
}

type fakeProvider struct {
	environs.EnvironProvider
	gitjujutesting.Stub
}

func (p *fakeProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
	p.MethodCall(p, "PrepareConfig", args)
	return args.Config, p.NextErr()
}

func (p *fakeProvider) Validate(newCfg, oldCfg *config.Config) (*config.Config, error) {
	p.MethodCall(p, "Validate", newCfg, oldCfg)
	return newCfg, p.NextErr()
}

func (p *fakeProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	p.MethodCall(p, "Open", args)
	return &fakeEnviron{Stub: &p.Stub}, p.NextErr()
}

type fakeEnviron struct {
	environs.Environ
	*gitjujutesting.Stub
}

func (e *fakeEnviron) Create(args environs.CreateParams) error {
	e.MethodCall(e, "Create", args)
	return e.NextErr()
}
