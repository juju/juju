// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

// Initialize initializes the state and returns it. If state was not
// already initialized, and cfg is nil, the minimal default environment
// configuration will be used.
func Initialize(c *gc.C, owner names.UserTag, cfg *config.Config, policy state.Policy) *state.State {
	if cfg == nil {
		cfg = testing.EnvironConfig(c)
	}
	mgoInfo := testing.NewMongoInfo()
	dialOpts := testing.NewDialOpts()

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
