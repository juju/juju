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
	role1, role2 state.RelationRole
	success      bool
}{
	{state.RoleProvider, state.RoleRequirer, true},
	{state.RoleRequirer, state.RolePeer, false},
	{state.RolePeer, state.RoleProvider, false},
	{state.RoleProvider, state.RoleProvider, false},
	{state.RoleRequirer, state.RoleRequirer, false},
	{state.RolePeer, state.RolePeer, false},
}

func (s *EndpointSuite) TestCanRelate(c *C) {
	for i, t := range canRelateTests {
		c.Logf("test %d", i)
		ep1 := state.Endpoint{"one-service", "ifce", "foo", t.role1, charm.ScopeGlobal}
		ep2 := state.Endpoint{"another-service", "ifce", "bar", t.role2, charm.ScopeGlobal}
		if t.success {
			c.Assert(ep1.CanRelateTo(ep2), Equals, true)
			c.Assert(ep2.CanRelateTo(ep1), Equals, true)
			ep1.Interface = "different"
		}
		c.Assert(ep1.CanRelateTo(ep2), Equals, false)
		c.Assert(ep2.CanRelateTo(ep1), Equals, false)
	}
	ep1 := state.Endpoint{"same-service", "ifce", "foo", state.RoleProvider, charm.ScopeGlobal}
	ep2 := state.Endpoint{"same-service", "ifce", "bar", state.RoleRequirer, charm.ScopeGlobal}
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
	ifce  string
	name  string
	role  state.RelationRole
	scope charm.RelationScope
	match bool
}{
	{"ifce-pro", "pro", state.RoleProvider, charm.ScopeGlobal, true},
	{"blah", "pro", state.RoleProvider, charm.ScopeGlobal, false},
	{"ifce-pro", "blah", state.RoleProvider, charm.ScopeGlobal, false},
	{"ifce-pro", "pro", state.RoleRequirer, charm.ScopeGlobal, false},
	{"ifce-pro", "pro", state.RoleProvider, charm.ScopeContainer, true},

	{"juju-info", "juju-info", state.RoleProvider, charm.ScopeGlobal, true},
	{"blah", "juju-info", state.RoleProvider, charm.ScopeGlobal, false},
	{"juju-info", "blah", state.RoleProvider, charm.ScopeGlobal, false},
	{"juju-info", "juju-info", state.RoleRequirer, charm.ScopeGlobal, false},
	{"juju-info", "juju-info", state.RoleProvider, charm.ScopeContainer, true},

	{"ifce-req", "req", state.RoleRequirer, charm.ScopeGlobal, true},
	{"blah", "req", state.RoleRequirer, charm.ScopeGlobal, false},
	{"ifce-req", "blah", state.RoleRequirer, charm.ScopeGlobal, false},
	{"ifce-req", "req", state.RolePeer, charm.ScopeGlobal, false},
	{"ifce-req", "req", state.RoleRequirer, charm.ScopeContainer, true},

	{"juju-info", "info", state.RoleRequirer, charm.ScopeContainer, true},
	{"blah", "info", state.RoleRequirer, charm.ScopeContainer, false},
	{"juju-info", "blah", state.RoleRequirer, charm.ScopeContainer, false},
	{"juju-info", "info", state.RolePeer, charm.ScopeContainer, false},
	{"juju-info", "info", state.RoleRequirer, charm.ScopeGlobal, false},

	{"ifce-peer", "peer", state.RolePeer, charm.ScopeGlobal, true},
	{"blah", "peer", state.RolePeer, charm.ScopeGlobal, false},
	{"ifce-peer", "blah", state.RolePeer, charm.ScopeGlobal, false},
	{"ifce-peer", "peer", state.RoleProvider, charm.ScopeGlobal, false},
	{"ifce-peer", "peer", state.RolePeer, charm.ScopeContainer, true},
}

func (s *EndpointSuite) TestImplementedBy(c *C) {
	for i, t := range implementedByTests {
		c.Logf("test %d", i)
		ep := state.Endpoint{"x", t.ifce, t.name, t.role, t.scope}
		c.Assert(ep.ImplementedBy(&dummyCharm{}), Equals, t.match)
	}
}
