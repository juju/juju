// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agentbootstrap"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	corenetwork "github.com/juju/juju/core/network"
	jujuversion "github.com/juju/juju/core/version"
	domainconstraints "github.com/juju/juju/domain/constraints"
	modelstate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/mongo/mongotest"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	jujujujutesting "github.com/juju/juju/juju/testing"
)

type bootstrapSuite struct {
	testing.BaseSuite
	mgoInst mgotesting.MgoInstance
}

func TestBootstrapSuite(t *stdtesting.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	// Don't use MgoSuite, because we need to ensure
	// we have a fresh mongo for each test case.
	s.mgoInst.EnableAuth = true
	s.mgoInst.EnableReplicaSet = true
	err := s.mgoInst.Start(testing.Certs)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *bootstrapSuite) TearDownTest(c *tc.C) {
	s.mgoInst.Destroy()
	s.BaseSuite.TearDownTest(c)
}

func getModelAssertion(c *tc.C, modelUUID coremodel.UUID) database.BootstrapOpt {
	return func(ctx context.Context, controller, model coredatabase.TxnRunner) error {
		modelState := modelstate.NewModelState(func() (coredatabase.TxnRunner, error) {
			return model, nil
		}, loggertesting.WrapCheckLog(c))

		info, err := modelState.GetModel(ctx)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(info.UUID, tc.Equals, modelUUID)
		return nil
	}
}

func getModelConstraintAssertion(c *tc.C, cons constraints.Value) database.BootstrapOpt {
	return func(ctx context.Context, controller, model coredatabase.TxnRunner) error {
		modelState := modelstate.NewModelState(func() (coredatabase.TxnRunner, error) {
			return model, nil
		}, loggertesting.WrapCheckLog(c))

		expectedConsVal := domainconstraints.DecodeConstraints(cons)
		data, err := modelState.GetModelConstraints(c.Context())
		c.Check(err, tc.ErrorIsNil)
		c.Check(data, tc.DeepEquals, expectedConsVal)
		return nil
	}
}

func (s *bootstrapSuite) TestInitializeState(c *tc.C) {
	dataDir := c.MkDir()

	s.PatchValue(&network.AddressesForInterfaceName, func(name string) ([]string, error) {
		if name == network.DefaultLXDBridge {
			return []string{
				"10.0.4.1",
				"10.0.4.4",
			}, nil
		}
		c.Fatalf("unknown bridge in testing: %v", name)
		return nil, nil
	})

	configParams := agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: dataDir},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: jujuversion.Current,
		APIAddresses:      []string{"localhost:17070"},
		CACert:            testing.CACert,
		Password:          testing.DefaultMongoPassword,
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
	}
	servingInfo := controller.StateServingInfo{
		Cert:           testing.ServerCert,
		PrivateKey:     testing.ServerKey,
		CAPrivateKey:   testing.CAKey,
		APIPort:        1234,
		StatePort:      s.mgoInst.Port(),
		SystemIdentity: "def456",
	}

	cfg, err := agent.NewStateMachineConfig(configParams, servingInfo)
	c.Assert(err, tc.ErrorIsNil)

	_, available := cfg.StateServingInfo()
	c.Assert(available, tc.IsTrue)
	expectBootstrapConstraints := constraints.MustParse("mem=1024M")
	expectModelConstraints := constraints.MustParse("mem=512M")
	initialAddrs := corenetwork.NewMachineAddresses([]string{
		"zeroonetwothree",
		"0.1.2.3",
		"10.0.3.3", // not a lxc bridge address
		"10.0.4.1", // lxd bridge address filtered.
		"10.0.4.4", // lxd bridge address filtered.
		"10.0.4.5", // not a lxd bridge address
	}).AsProviderAddresses()

	modelAttrs := testing.FakeConfig().Merge(testing.Attrs{
		"agent-version":  jujuversion.Current.String(),
		"charmhub-url":   charmhub.DefaultServerURL,
		"not-for-hosted": "foo",
	})
	modelCfg, err := config.New(config.NoDefaults, modelAttrs)
	c.Assert(err, tc.ErrorIsNil)
	controllerCfg := testing.FakeControllerConfig()
	controllerModelUUID := coremodel.UUID(modelCfg.UUID())

	controllerInheritedConfig := map[string]interface{}{
		"apt-mirror": "http://mirror",
		"no-proxy":   "value",
	}
	regionConfig := cloud.RegionConfig{
		"some-region": cloud.Attrs{
			"no-proxy": "a-value",
		},
	}
	registry := provider.CommonStorageProviders()
	var envProvider fakeProvider
	stateInitParams := instancecfg.StateInitializationParams{
		BootstrapMachineConstraints: expectBootstrapConstraints,
		BootstrapMachineInstanceId:  "i-bootstrap",
		BootstrapMachineDisplayName: "test-display-name",
		ControllerCloud: cloud.Cloud{
			Name:         "dummy",
			Type:         "dummy",
			AuthTypes:    []cloud.AuthType{cloud.EmptyAuthType},
			Regions:      []cloud.Region{{Name: "dummy-region"}},
			RegionConfig: regionConfig,
		},
		ControllerCloudRegion:         "dummy-region",
		ControllerConfig:              controllerCfg,
		ControllerModelConfig:         modelCfg,
		ControllerModelEnvironVersion: 666,
		ModelConstraints:              expectModelConstraints,
		ControllerInheritedConfig:     controllerInheritedConfig,
		StoragePools: map[string]storage.Attrs{
			"spool": {
				"type": "loop",
				"foo":  "bar",
			},
		},
		SSHServerHostKey: testing.SSHServerHostKey,
	}
	adminUser := names.NewLocalUserTag("agent-admin")
	bootstrap, err := agentbootstrap.NewAgentBootstrap(
		agentbootstrap.AgentBootstrapArgs{
			AgentConfig:               cfg,
			BootstrapEnviron:          &fakeEnviron{},
			AdminUser:                 adminUser,
			StateInitializationParams: stateInitParams,
			MongoDialOpts:             mongotest.DialOpts(),
			BootstrapMachineAddresses: initialAddrs,
			SharedSecret:              "abc123",
			StorageProviderRegistry:   registry,
			BootstrapDqlite: getBootstrapDqliteWithDummyCloudTypeWithAssertions(c,
				getModelAssertion(c, controllerModelUUID),
				getModelConstraintAssertion(c, expectModelConstraints),
			),
			Provider: func(t string) (environs.EnvironProvider, error) {
				c.Assert(t, tc.Equals, "dummy")
				return &envProvider, nil
			},
			Logger: loggertesting.WrapCheckLog(c),
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	err = bootstrap.Initialize(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *bootstrapSuite) TestInitializeStateWithStateServingInfoNotAvailable(c *tc.C) {
	configParams := agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: c.MkDir()},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: jujuversion.Current,
		APIAddresses:      []string{"localhost:17070"},
		CACert:            testing.CACert,
		Password:          "fake",
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
	}
	cfg, err := agent.NewAgentConfig(configParams)
	c.Assert(err, tc.ErrorIsNil)

	_, available := cfg.StateServingInfo()
	c.Assert(available, tc.IsFalse)

	adminUser := names.NewLocalUserTag("agent-admin")

	bootstrap, err := agentbootstrap.NewAgentBootstrap(
		agentbootstrap.AgentBootstrapArgs{
			AgentConfig:               cfg,
			BootstrapEnviron:          &fakeEnviron{},
			AdminUser:                 adminUser,
			StateInitializationParams: instancecfg.StateInitializationParams{},
			MongoDialOpts:             mongotest.DialOpts(),
			SharedSecret:              "abc123",
			StorageProviderRegistry:   provider.CommonStorageProviders(),
			BootstrapDqlite:           getBootstrapDqliteWithDummyCloudTypeWithAssertions(c),
			Logger:                    loggertesting.WrapCheckLog(c),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	err = bootstrap.Initialize(c.Context())

	// InitializeState will fail attempting to get the api port information
	c.Assert(err, tc.ErrorMatches, "state serving information not available")
}

func (s *bootstrapSuite) TestInitializeStateFailsSecondTime(c *tc.C) {
	dataDir := c.MkDir()

	configParams := agent.AgentConfigParams{
		Paths:             agent.Paths{DataDir: dataDir},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: jujuversion.Current,
		APIAddresses:      []string{"localhost:17070"},
		CACert:            testing.CACert,
		Password:          testing.DefaultMongoPassword,
		Controller:        testing.ControllerTag,
		Model:             testing.ModelTag,
	}
	cfg, err := agent.NewAgentConfig(configParams)
	c.Assert(err, tc.ErrorIsNil)
	cfg.SetStateServingInfo(controller.StateServingInfo{
		APIPort:        5555,
		StatePort:      s.mgoInst.Port(),
		Cert:           testing.CACert,
		PrivateKey:     testing.CAKey,
		SharedSecret:   "baz",
		SystemIdentity: "qux",
	})
	modelAttrs := testing.FakeConfig().Delete("admin-secret").Merge(testing.Attrs{
		"agent-version": jujuversion.Current.String(),
		"charmhub-url":  charmhub.DefaultServerURL,
	})
	modelCfg, err := config.New(config.NoDefaults, modelAttrs)
	c.Assert(err, tc.ErrorIsNil)

	args := instancecfg.StateInitializationParams{
		BootstrapMachineInstanceId:  "i-bootstrap",
		BootstrapMachineDisplayName: "test-display-name",
		ControllerCloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			Regions:   []cloud.Region{{Name: "dummy-region"}},
		},
		ControllerConfig:      testing.FakeControllerConfig(),
		ControllerModelConfig: modelCfg,
		SSHServerHostKey:      testing.SSHServerHostKey,
	}

	adminUser := names.NewLocalUserTag("agent-admin")
	bootstrap, err := agentbootstrap.NewAgentBootstrap(
		agentbootstrap.AgentBootstrapArgs{
			AgentConfig:               cfg,
			BootstrapEnviron:          &fakeEnviron{},
			AdminUser:                 adminUser,
			StateInitializationParams: args,
			MongoDialOpts:             mongotest.DialOpts(),
			SharedSecret:              "abc123",
			StorageProviderRegistry:   provider.CommonStorageProviders(),
			BootstrapDqlite:           getBootstrapDqliteWithDummyCloudTypeWithAssertions(c),
			Provider: func(t string) (environs.EnvironProvider, error) {
				return &fakeProvider{}, nil
			},
			Logger: loggertesting.WrapCheckLog(c),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	err = bootstrap.Initialize(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	bootstrap, err = agentbootstrap.NewAgentBootstrap(
		agentbootstrap.AgentBootstrapArgs{
			AgentConfig:               cfg,
			BootstrapEnviron:          &fakeEnviron{},
			AdminUser:                 adminUser,
			StateInitializationParams: args,
			MongoDialOpts:             mongotest.DialOpts(),
			SharedSecret:              "baz",
			StorageProviderRegistry:   provider.CommonStorageProviders(),
			BootstrapDqlite:           database.BootstrapDqlite,
			Logger:                    loggertesting.WrapCheckLog(c),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	err = bootstrap.Initialize(c.Context())
	c.Assert(err, tc.ErrorIs, errors.AlreadyExists)
}

type fakeProvider struct {
	environs.EnvironProvider
	testhelpers.Stub
	environ *fakeEnviron
}

func (p *fakeProvider) ValidateCloud(_ context.Context, spec cloudspec.CloudSpec) error {
	p.MethodCall(p, "ValidateCloud", spec)
	return p.NextErr()
}

func (p *fakeProvider) Validate(_ context.Context, newCfg, oldCfg *config.Config) (*config.Config, error) {
	p.MethodCall(p, "Validate", newCfg, oldCfg)
	return newCfg, p.NextErr()
}

func (p *fakeProvider) Open(_ context.Context, args environs.OpenParams) (environs.Environ, error) {
	p.MethodCall(p, "Open", args)
	p.environ = &fakeEnviron{Stub: &p.Stub, provider: p}
	return p.environ, p.NextErr()
}

func (p *fakeProvider) Version() int {
	p.MethodCall(p, "Version")
	p.PopNoErr()
	return 123
}

type fakeEnviron struct {
	environs.Environ
	*testhelpers.Stub
	provider *fakeProvider
}

func (e *fakeEnviron) Provider() environs.EnvironProvider {
	e.MethodCall(e, "Provider")
	e.PopNoErr()
	return e.provider
}

func getBootstrapDqliteWithDummyCloudTypeWithAssertions(c *tc.C,
	assertions ...database.BootstrapOpt,
) agentbootstrap.DqliteInitializerFunc {
	return func(
		ctx context.Context,
		mgr database.BootstrapNodeManager,
		modelUUID coremodel.UUID,
		logger logger.Logger,
		opts ...database.BootstrapOpt,
	) error {

		// The dummy cloud type needs to be inserted before the other operations.
		opts = append([]database.BootstrapOpt{
			jujujujutesting.InsertDummyCloudType,
		}, opts...)

		// The assertions need to be inserted after the other operations.
		called := 0
		for _, assertion := range assertions {
			opts = append(opts, func(ctx context.Context, controller, model coredatabase.TxnRunner) error {
				called++
				return assertion(ctx, controller, model)
			})
		}
		defer func() {
			c.Assert(called, tc.Equals, len(assertions))
		}()

		return database.BootstrapDqlite(ctx, mgr, modelUUID, logger, opts...)
	}
}
