// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
)

type EndpointSuite struct {
}

var _ = Suite(&EndpointSuite{})

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

func (s *EndpointSuite) TestCanRelate(c *C) {
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
			c.Assert(ep1.CanRelateTo(ep2), Equals, true)
			c.Assert(ep2.CanRelateTo(ep1), Equals, true)
			ep1.Interface = "different"
		}
		c.Assert(ep1.CanRelateTo(ep2), Equals, false)
		c.Assert(ep2.CanRelateTo(ep1), Equals, false)
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
	c.Assert(ep1.CanRelateTo(ep2), Equals, false)
	c.Assert(ep2.CanRelateTo(ep1), Equals, false)
}

type dummyCharm struct{}

func (c *dummyCharm) Config() *charm.Config {
	panic("unused")
}

func (c *dummyCharm) Revision() int {
	panic("unused")
}

func (c *dummyCharm) Meta() *charm.Meta {
	return &charm.Meta{
		Provides: map[string]charm.Relation{
			"pro": {Interface: "ifce-pro", Scope: charm.ScopeGlobal},
		},
		Requires: map[string]charm.Relation{
			"req":  {Interface: "ifce-req", Scope: charm.ScopeGlobal},
			"info": {Interface: "juju-info", Scope: charm.ScopeContainer},
		},
		Peers: map[string]charm.Relation{
			"peer": {Interface: "ifce-peer", Scope: charm.ScopeGlobal},
		},
	}
}

var implementedByTests = []struct {
	ifce     string
	name     string
	role     charm.RelationRole
	scope    charm.RelationScope
	match    bool
	implicit bool
}{
	{"ifce-pro", "pro", charm.RoleProvider, charm.ScopeGlobal, true, false},
	{"blah", "pro", charm.RoleProvider, charm.ScopeGlobal, false, false},
	{"ifce-pro", "blah", charm.RoleProvider, charm.ScopeGlobal, false, false},
	{"ifce-pro", "pro", charm.RoleRequirer, charm.ScopeGlobal, false, false},
	{"ifce-pro", "pro", charm.RoleProvider, charm.ScopeContainer, true, false},

	{"juju-info", "juju-info", charm.RoleProvider, charm.ScopeGlobal, true, true},
	{"blah", "juju-info", charm.RoleProvider, charm.ScopeGlobal, false, false},
	{"juju-info", "blah", charm.RoleProvider, charm.ScopeGlobal, false, false},
	{"juju-info", "juju-info", charm.RoleRequirer, charm.ScopeGlobal, false, false},
	{"juju-info", "juju-info", charm.RoleProvider, charm.ScopeContainer, true, true},

	{"ifce-req", "req", charm.RoleRequirer, charm.ScopeGlobal, true, false},
	{"blah", "req", charm.RoleRequirer, charm.ScopeGlobal, false, false},
	{"ifce-req", "blah", charm.RoleRequirer, charm.ScopeGlobal, false, false},
	{"ifce-req", "req", charm.RolePeer, charm.ScopeGlobal, false, false},
	{"ifce-req", "req", charm.RoleRequirer, charm.ScopeContainer, true, false},

	{"juju-info", "info", charm.RoleRequirer, charm.ScopeContainer, true, false},
	{"blah", "info", charm.RoleRequirer, charm.ScopeContainer, false, false},
	{"juju-info", "blah", charm.RoleRequirer, charm.ScopeContainer, false, false},
	{"juju-info", "info", charm.RolePeer, charm.ScopeContainer, false, false},
	{"juju-info", "info", charm.RoleRequirer, charm.ScopeGlobal, false, false},

	{"ifce-peer", "peer", charm.RolePeer, charm.ScopeGlobal, true, false},
	{"blah", "peer", charm.RolePeer, charm.ScopeGlobal, false, false},
	{"ifce-peer", "blah", charm.RolePeer, charm.ScopeGlobal, false, false},
	{"ifce-peer", "peer", charm.RoleProvider, charm.ScopeGlobal, false, false},
	{"ifce-peer", "peer", charm.RolePeer, charm.ScopeContainer, true, false},
}

func (s *EndpointSuite) TestImplementedBy(c *C) {
	for i, t := range implementedByTests {
		c.Logf("test %d", i)
		ep := state.Endpoint{
			ServiceName: "x",
			Relation: charm.Relation{
				Interface: t.ifce,
				Name:      t.name,
				Role:      t.role,
				Scope:     t.scope,
			},
		}
		c.Assert(ep.ImplementedBy(&dummyCharm{}), Equals, t.match)
		c.Assert(ep.IsImplicit(), Equals, t.implicit)
	}
}
