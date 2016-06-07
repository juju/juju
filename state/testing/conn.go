// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/names"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

// Initialize initializes the state and returns it. If state was not
// already initialized, and cfg is nil, the minimal default model
// configuration will be used.
func Initialize(c *gc.C, owner names.UserTag, cfg *config.Config, policy state.Policy) *state.State {
	if cfg == nil {
		cfg = testing.ModelConfig(c)
	}
	mgoInfo := NewMongoInfo()
	dialOpts := mongotest.DialOpts()

	st, err := state.Initialize(state.InitializeParams{
		ControllerModelArgs: state.ModelArgs{
			Owner:  owner,
			Config: cfg,
			Cloud:  "dummy",
		},
		MongoInfo:        mgoInfo,
		MongoDialOpts:    dialOpts,
		Policy:           policy,
		PublicClouds:     TestClouds(),
		CloudCredentials: TestCredentials(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return st
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
	policy := MockPolicy{}
	return Initialize(c, owner, cfg, &policy)
}
