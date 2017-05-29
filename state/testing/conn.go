// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/state"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/testing"
)

type InitializeArgs struct {
	Owner                     names.UserTag
	InitialConfig             *config.Config
	ControllerConfig          map[string]interface{}
	ControllerInheritedConfig map[string]interface{}
	RegionConfig              cloud.RegionConfig
	NewPolicy                 state.NewPolicyFunc
	Clock                     clock.Clock
}

// Initialize initializes the state and returns it. If state was not
// already initialized, and cfg is nil, the minimal default model
// configuration will be used.
// This provides for tests still using a real clock from utils as tests are
// migrated to use the testing clock
func Initialize(c *gc.C, owner names.UserTag, cfg *config.Config, controllerInheritedConfig map[string]interface{}, regionConfig cloud.RegionConfig, newPolicy state.NewPolicyFunc) *state.State {
	return InitializeWithArgs(c, InitializeArgs{
		Owner:                     owner,
		InitialConfig:             cfg,
		ControllerInheritedConfig: controllerInheritedConfig,
		RegionConfig:              regionConfig,
		NewPolicy:                 newPolicy,
		Clock:                     &clock.WallClock,
	})
}

// InitializeWithArgs initializes the state and returns it. If state was not
// already initialized, and args.Config is nil, the minimal default model
// configuration will be used.
func InitializeWithArgs(c *gc.C, args InitializeArgs) *state.State {
	if args.InitialConfig == nil {
		args.InitialConfig = testing.ModelConfig(c)
	}
	mgoInfo := NewMongoInfo()
	dialOpts := mongotest.DialOpts()

	controllerCfg := testing.FakeControllerConfig()
	for k, v := range args.ControllerConfig {
		controllerCfg[k] = v
	}
	st, err := state.Initialize(state.InitializeParams{
		Clock:            args.Clock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			CloudName:   "dummy",
			CloudRegion: "dummy-region",
			Config:      args.InitialConfig,
			Owner:       args.Owner,
			StorageProviderRegistry: dummystorage.StorageProviders(),
		},
		ControllerInheritedConfig: args.ControllerInheritedConfig,
		Cloud: cloud.Cloud{
			Name:      "dummy",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			Regions: []cloud.Region{
				{
					Name:             "dummy-region",
					Endpoint:         "dummy-endpoint",
					IdentityEndpoint: "dummy-identity-endpoint",
					StorageEndpoint:  "dummy-storage-endpoint",
				},
				{
					Name:             "nether-region",
					Endpoint:         "nether-endpoint",
					IdentityEndpoint: "nether-identity-endpoint",
					StorageEndpoint:  "nether-storage-endpoint",
				},
				{
					Name:             "unused-region",
					Endpoint:         "unused-endpoint",
					IdentityEndpoint: "unused-identity-endpoint",
					StorageEndpoint:  "unused-storage-endpoint",
				},
			},
			RegionConfig: args.RegionConfig,
		},
		MongoInfo:     mgoInfo,
		MongoDialOpts: dialOpts,
		NewPolicy:     args.NewPolicy,
	})
	c.Assert(err, jc.ErrorIsNil)
	return st
}

// NewMongoInfo returns information suitable for
// connecting to the testing controller's mongo database.
func NewMongoInfo() *mongo.MongoInfo {
	return &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:      []string{jujutesting.MgoServer.Addr()},
			CACert:     testing.CACert,
			DisableTLS: !jujutesting.MgoServer.SSLEnabled(),
		},
	}
}

// NewState initializes a new state with default values for testing and
// returns it.
func NewState(c *gc.C) *state.State {
	owner := names.NewLocalUserTag("test-admin")
	cfg := testing.ModelConfig(c)
	newPolicy := func(*state.State) state.Policy { return &MockPolicy{} }
	return Initialize(c, owner, cfg, nil, nil, newPolicy)
}
