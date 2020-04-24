// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/clock"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/testing"
)

type InitializeArgs struct {
	Owner                     names.UserTag
	AdminPassword             string
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
func Initialize(c *gc.C, owner names.UserTag, cfg *config.Config, controllerInheritedConfig map[string]interface{}, regionConfig cloud.RegionConfig, newPolicy state.NewPolicyFunc) *state.Controller {
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
func InitializeWithArgs(c *gc.C, args InitializeArgs) *state.Controller {
	if args.InitialConfig == nil {
		args.InitialConfig = testing.ModelConfig(c)
	}
	if args.AdminPassword == "" {
		args.AdminPassword = "admin-secret"
	}

	session, err := jujutesting.MgoServer.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	controllerCfg := testing.FakeControllerConfig()
	for k, v := range args.ControllerConfig {
		controllerCfg[k] = v
	}
	ctlr, err := state.Initialize(state.InitializeParams{
		Clock:            args.Clock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:        state.ModelTypeIAAS,
			CloudName:   "dummy",
			CloudRegion: "dummy-region",
			Config:      args.InitialConfig,
			Owner:       args.Owner,
			StorageProviderRegistry: storage.ChainedProviderRegistry{
				dummystorage.StorageProviders(),
				provider.CommonStorageProviders(),
			},
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
				{
					Name:             "dotty.region",
					Endpoint:         "dotty.endpoint",
					IdentityEndpoint: "dotty.identity-endpoint",
					StorageEndpoint:  "dotty.storage-endpoint",
				},
			},
			RegionConfig: args.RegionConfig,
		},
		MongoSession:  session,
		NewPolicy:     args.NewPolicy,
		AdminPassword: args.AdminPassword,
	})
	c.Assert(err, jc.ErrorIsNil)
	return ctlr
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
