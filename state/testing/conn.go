// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

// TODO(ericsnow) The corresponding functions in state and state_test
// (see state_test.go and settings_test.go) should be dropped and these
// used instead.  Doing so is complicated by the fact that some of the
// test files are in the state package rather than state_test, resulting
// in import cycles when switching to these functions.

// NewMongoInfo returns information suitable for
// connecting to the testing state server's mongo database.
func NewMongoInfo() *mongo.MongoInfo {
	return &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{gitjujutesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
	}
}

// NewDialOpts returns configuration parameters for
// connecting to the testing state server.
func NewDialOpts() mongo.DialOpts {
	return mongo.DialOpts{
		Timeout: testing.LongWait,
	}
}

// Initialize initializes the state and returns it. If state was not
// already initialized, and cfg is nil, the minimal default environment
// configuration will be used.
func Initialize(c *gc.C, owner names.UserTag, cfg *config.Config, policy state.Policy) *state.State {
	if cfg == nil {
		cfg = testing.EnvironConfig(c)
	}
	mgoInfo := NewMongoInfo()
	dialOpts := NewDialOpts()

	st, err := state.Initialize(owner, mgoInfo, cfg, dialOpts, policy)
	c.Assert(err, jc.ErrorIsNil)
	return st
}

// NewState initializes a new state with default values for testing and
// returns it.
func NewState(c *gc.C) *state.State {
	owner := names.NewLocalUserTag("test-admin")
	cfg := testing.EnvironConfig(c)
	policy := MockPolicy{}
	return Initialize(c, owner, cfg, &policy)
}
