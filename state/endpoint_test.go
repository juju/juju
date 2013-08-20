// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
)

type EndpointSuite struct {
}

var _ = gc.Suite(&EndpointSuite{})

var canRelateTests = []struct {
	role1, role2 charm.RelationRole
	success      bool
}{
	{charm.RoleProvider, charm.RoleRequirer, true},
	{charm.RoleRequirer, charm.RolePeer, false},
	{charm.RolePeer, charm.RoleProvider, false},
	{charm.RoleProvider, charm.RoleProvider, false},
	{charm.RoleRequirer, charm.RoleRequirer, false},
	{charm.RolePeer, charm.RolePeer, false},
}

func (s *EndpointSuite) TestCanRelate(c *gc.C) {
	for i, t := range canRelateTests {
		c.Logf("test %d", i)
		ep1 := state.Endpoint{
			ServiceName: "one-service",
			Relation: charm.Relation{
				Interface: "ifce",
				Name:      "foo",
				Role:      t.role1,
				Scope:     charm.ScopeGlobal,
			},
		}
		ep2 := state.Endpoint{
			ServiceName: "another-service",
			Relation: charm.Relation{
				Interface: "ifce",
				Name:      "bar",
				Role:      t.role2,
				Scope:     charm.ScopeGlobal,
			},
		}
		if t.success {
			c.Assert(ep1.CanRelateTo(ep2), gc.Equals, true)
			c.Assert(ep2.CanRelateTo(ep1), gc.Equals, true)
			ep1.Interface = "different"
		}
		c.Assert(ep1.CanRelateTo(ep2), gc.Equals, false)
		c.Assert(ep2.CanRelateTo(ep1), gc.Equals, false)
	}
	ep1 := state.Endpoint{
		ServiceName: "same-service",
		Relation: charm.Relation{
			Interface: "ifce",
			Name:      "foo",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	ep2 := state.Endpoint{
		ServiceName: "same-service",
		Relation: charm.Relation{
			Interface: "ifce",
			Name:      "bar",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		},
	}
	c.Assert(ep1.CanRelateTo(ep2), gc.Equals, false)
	c.Assert(ep2.CanRelateTo(ep1), gc.Equals, false)
}
