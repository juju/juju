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
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/testing"
)

// Initialize initializes the state and returns it. If state was not
// already initialized, and cfg is nil, the minimal default model
// configuration will be used.
func Initialize(c *gc.C, owner names.UserTag, cfg *config.Config, controllerInheritedConfig map[string]interface{}, regionConfig cloud.RegionConfig, newPolicy state.NewPolicyFunc) *state.State {
	if cfg == nil {
		cfg = testing.ModelConfig(c)
	}
	mgoInfo := NewMongoInfo()
	dialOpts := mongotest.DialOpts()

	controllerCfg := testing.FakeControllerConfig()
	st, err := state.Initialize(state.InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			CloudName:   "dummy",
			CloudRegion: "dummy-region",
			Config:      cfg,
			Owner:       owner,
			StorageProviderRegistry: StorageProviders(),
		},
		ControllerInheritedConfig: controllerInheritedConfig,
		CloudName:                 "dummy",
		Cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			Regions: []cloud.Region{
				cloud.Region{
					Name:             "dummy-region",
					Endpoint:         "dummy-endpoint",
					IdentityEndpoint: "dummy-identity-endpoint",
					StorageEndpoint:  "dummy-storage-endpoint",
				},
				cloud.Region{
					Name:             "nether-region",
					Endpoint:         "nether-endpoint",
					IdentityEndpoint: "nether-identity-endpoint",
					StorageEndpoint:  "nether-storage-endpoint",
				},
			},
			RegionConfig: regionConfig,
		},
		MongoInfo:     mgoInfo,
		MongoDialOpts: dialOpts,
		NewPolicy:     newPolicy,
	})
	c.Assert(err, jc.ErrorIsNil)
	return st
}

func StorageProviders() storage.ProviderRegistry {
	return storage.ChainedProviderRegistry{
		storage.StaticProviderRegistry{
			map[storage.ProviderType]storage.Provider{
				"static": &dummystorage.StorageProvider{IsDynamic: false},
				"environscoped": &dummystorage.StorageProvider{
					StorageScope: storage.ScopeEnviron,
					IsDynamic:    true,
				},
				"environscoped-block": &dummystorage.StorageProvider{
					StorageScope: storage.ScopeEnviron,
					IsDynamic:    true,
					SupportsFunc: func(k storage.StorageKind) bool {
						return k == storage.StorageKindBlock
					},
				},
				"machinescoped": &dummystorage.StorageProvider{
					StorageScope: storage.ScopeMachine,
					IsDynamic:    true,
				},
			},
		},
		provider.CommonStorageProviders(),
	}
}

// NewMongoInfo returns information suitable for
// connecting to the testing controller's mongo database.
func NewMongoInfo() *mongo.MongoInfo {
	return &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{jujutesting.MgoServer.Addr()},
			CACert: testing.CACert,
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
