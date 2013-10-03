// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
)

// ChangeEnvironConfig applies the given change function
// to the attributes from st.EnvironConfig and
// sets the state's environment configuration to the result.
func ChangeEnvironConfig(c *gc.C, st *state.State, change func(coretesting.Attrs) coretesting.Attrs) {
	cfg, err := st.EnvironConfig()
	c.Assert(err, gc.IsNil)
	newCfg, err := config.New(config.NoDefaults, change(cfg.AllAttrs()))
	c.Assert(err, gc.IsNil)
	err = st.SetEnvironConfig(newCfg)
	c.Assert(err, gc.IsNil)
}
