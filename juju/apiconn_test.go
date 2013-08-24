// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
)

type NewAPIConnSuite struct {
	coretesting.LoggingSuite
}

var _ = gc.Suite(&NewAPIConnSuite{})

func (cs *NewAPIConnSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	cs.LoggingSuite.TearDownTest(c)
}

func (*NewAPIConnSuite) TestNewConn(c *gc.C) {
	cfg, err := config.New(map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"secret":          "pork",
		"admin-secret":    "really",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
	})
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg)
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)

	conn, err := juju.NewConn(env)
	c.Assert(err, gc.IsNil)

	c.Assert(conn.Environ, gc.Equals, env)
	c.Assert(conn.State, gc.NotNil)

	c.Assert(conn.Close(), gc.IsNil)
}
