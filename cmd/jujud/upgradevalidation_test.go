// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
)

var _ = gc.Suite(&UpgradeValidationSuite{})

type UpgradeValidationSuite struct {
	agentSuite
}

func (s *UpgradeValidationSuite) Create1_10Machine(c *gc.C) (*state.Machine, *agent.Conf) {
	// Given the current connection to state, create a new machine, and 'reset'
	// the configuration so that it looks like how juju 1.10 would have
	// configured it
	m, err := s.State.InjectMachine("series", constraints.Value{}, "ardbeg-0", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = m.SetMongoPassword("machine-password")
	c.Assert(err, gc.IsNil)
	// We intentionally do *not* call m.SetPassword here, as it was not
	// done in 1.10, we also intentionally set the APIInfo.Password back to
	// the empty string.
	conf, _ := s.agentSuite.primeAgent(c, m.Tag(), "machine-password")
	conf.MachineNonce = state.BootstrapNonce
	conf.APIInfo.Password = ""
	conf.Write()
	c.Assert(err, gc.IsNil)
	return m, conf
}

func (s *UpgradeValidationSuite) TestEnsureAPIPasswordMachine(c *gc.C) {
	m, conf := s.Create1_10Machine(c)
	// Opening the API should fail as is
	apiState, newPassword, err := conf.OpenAPI(api.DialOpts{})
	c.Assert(apiState, gc.IsNil)
	c.Assert(newPassword, gc.Equals, "")
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password")

	err = EnsureAPIPassword(conf, m)
	c.Assert(err, gc.IsNil)
	// After EnsureAPIPassword we should be able to connect
	apiState, newPassword, err = conf.OpenAPI(api.DialOpts{})
	c.Assert(err, gc.IsNil)
	c.Assert(apiState, gc.NotNil)
	// We shouldn't need to set a new password
	c.Assert(newPassword, gc.Equals, "")
}

func (s *UpgradeValidationSuite) TestEnsureAPIPasswordMachineNoOp(c *gc.C) {
	m, conf := s.Create1_10Machine(c)
	// Set the API password to something, and record it, ensure that
	// EnsureAPIPassword doesn't change it on us
	m.SetPassword("frobnizzle")
	conf.APIInfo.Password = "frobnizzle"
	// We matched them, so we should be able to open the API
	apiState, newPassword, err := conf.OpenAPI(api.DialOpts{})
	c.Assert(apiState, gc.NotNil)
	c.Assert(newPassword, gc.Equals, "")
	c.Assert(err, gc.IsNil)
	apiState.Close()

	err = EnsureAPIPassword(conf, m)
	c.Assert(err, gc.IsNil)
	// After EnsureAPIPassword we should still be able to connect
	apiState, newPassword, err = conf.OpenAPI(api.DialOpts{})
	c.Assert(err, gc.IsNil)
	c.Assert(apiState, gc.NotNil)
	// We shouldn't need to set a new password
	c.Assert(newPassword, gc.Equals, "")
	// The password hasn't been changed
	c.Assert(conf.APIInfo.Password, gc.Equals, "frobnizzle")
}
