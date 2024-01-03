// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	"github.com/juju/clock"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type InitializeArgs struct {
	Owner                     names.UserTag
	AdminPassword             string
	InitialConfig             *config.Config
	ControllerConfig          map[string]interface{}
	ControllerInheritedConfig map[string]interface{}
	ControllerModelType       state.ModelType
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

	session, err := mgotesting.MgoServer.Dial()
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	controllerCfg := testing.FakeControllerConfig()
	for k, v := range args.ControllerConfig {
		controllerCfg[k] = v
	}

	modelType := state.ModelTypeIAAS
	if args.ControllerModelType != "" {
		modelType = args.ControllerModelType
	}

	ctlr, err := state.Initialize(state.InitializeParams{
		Clock:            args.Clock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Type:        modelType,
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
		CloudName:                 "dummy",
		MongoSession:              session,
		WatcherPollInterval:       10 * time.Millisecond,
		NewPolicy:                 args.NewPolicy,
		AdminPassword:             args.AdminPassword,
	})
	c.Assert(err, jc.ErrorIsNil)
	return ctlr
}

// NewMongoInfo returns information suitable for
// connecting to the testing controller's mongo database.
func NewMongoInfo() *mongo.MongoInfo {
	return &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:      []string{mgotesting.MgoServer.Addr()},
			CACert:     testing.CACert,
			DisableTLS: !mgotesting.MgoServer.SSLEnabled(),
		},
	}
}
