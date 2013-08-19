// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/provider/dummy"
	"launchpad.net/juju-core/juju"
	coretesting "launchpad.net/juju-core/testing"
)

type NewAPIConnSuite struct {
	coretesting.LoggingSuite
}

var _ = Suite(&NewAPIConnSuite{})

func (cs *NewAPIConnSuite) TearDownTest(c *C) {
	dummy.Reset()
	cs.LoggingSuite.TearDownTest(c)
}

func (*NewAPIConnSuite) TestNewConn(c *C) {
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"secret":          "pork",
		"admin-secret":    "really",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
	}
	env, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, IsNil)

	conn, err := juju.NewConn(env)
	c.Assert(err, IsNil)

	c.Assert(conn.Environ, Equals, env)
	c.Assert(conn.State, NotNil)

	c.Assert(conn.Close(), IsNil)
}
