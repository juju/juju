// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	"github.com/juju/clock"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

type InitializeArgs struct {
	Owner                     names.UserTag
	InitialConfig             *config.Config
	ControllerConfig          map[string]interface{}
	ControllerInheritedConfig map[string]interface{}
	ControllerModelType       state.ModelType
	RegionConfig              cloud.RegionConfig
	NewPolicy                 state.NewPolicyFunc
	Clock                     clock.Clock
}

// InitializeWithArgs initializes the state and returns it. If state was not
// already initialized, and args.Config is nil, the minimal default model
// configuration will be used.
func InitializeWithArgs(c *tc.C, args InitializeArgs) *state.Controller {
	if args.InitialConfig == nil {
		args.InitialConfig = testing.ModelConfig(c)
	}

	controllerCfg := testing.FakeControllerConfig()
	for k, v := range args.ControllerConfig {
		controllerCfg[k] = v
	}

	modelType := state.ModelTypeIAAS
	if args.ControllerModelType != "" {
		modelType = args.ControllerModelType
	}

	ctlr, err := state.Initialize(state.InitializeParams{
		SSHServerHostKey: testing.SSHServerHostKey,
		Clock:            args.Clock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: state.ModelArgs{
			Name:        args.InitialConfig.Name(),
			UUID:        coremodel.UUID(args.InitialConfig.UUID()),
			Type:        modelType,
			CloudName:   "dummy",
			CloudRegion: "dummy-region",
			Owner:       args.Owner,
		},
		ControllerInheritedConfig: args.ControllerInheritedConfig,
		CloudName:                 "dummy",
		WatcherPollInterval:       10 * time.Millisecond,
		NewPolicy:                 args.NewPolicy,
	})
	c.Assert(err, tc.ErrorIsNil)
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
