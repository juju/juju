package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
)

type RelationSuite struct {
	ConnSuite
	charm *state.Charm
}

var _ = Suite(&RelationSuite{})

func (s *RelationSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
}

func (s *RelationSuite) TestAddRelationErrors(c *C) {
	req, err := s.State.AddService("req", s.charm)
	c.Assert(err, IsNil)
	reqep := state.RelationEndpoint{"req", "ifce", "bar", state.RoleRequirer, state.ScopeGlobal}

	// Check we can't add a relation until both services exist.
	proep := state.RelationEndpoint{"pro", "ifce", "foo", state.RoleProvider, state.ScopeGlobal}
	err = s.State.AddRelation(proep, reqep)
	c.Assert(err, ErrorMatches, `can't add relation "pro:foo req:bar": service with name "pro" not found`)
	assertNoRelations(c, req)
	pro, err := s.State.AddService("pro", s.charm)
	c.Assert(err, IsNil)

	// Check that interfaces have to match.
	proep2 := state.RelationEndpoint{"pro", "other", "foo", state.RoleProvider, state.ScopeGlobal}
	err = s.State.AddRelation(proep2, reqep)
	c.Assert(err, ErrorMatches, `can't add relation "pro:foo req:bar": endpoints do not relate`)
	assertNoRelations(c, pro)
	assertNoRelations(c, req)

	// Check a variety of surprising endpoint combinations.
	err = s.State.AddRelation(reqep)
	c.Assert(err, ErrorMatches, `can't add relation "req:bar": single endpoint must be a peer relation`)
	assertNoRelations(c, req)

	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, state.ScopeGlobal}
	err = s.State.AddRelation(peerep, reqep)
	c.Assert(err, ErrorMatches, `can't add relation "peer:baz req:bar": endpoints do not relate`)
	assertNoRelations(c, peer)
	assertNoRelations(c, req)

	err = s.State.AddRelation(peerep, peerep)
	c.Assert(err, ErrorMatches, `can't add relation "peer:baz peer:baz": endpoints do not relate`)
	assertNoRelations(c, peer)

	err = s.State.AddRelation()
	c.Assert(err, ErrorMatches, `can't add relation "": can't relate 0 endpoints`)
	err = s.State.AddRelation(proep, reqep, peerep)
	c.Assert(err, ErrorMatches, `can't add relation "pro:foo req:bar peer:baz": can't relate 3 endpoints`)
}

func (s *RelationSuite) TestProviderRequirerRelation(c *C) {
	req, err := s.State.AddService("req", s.charm)
	c.Assert(err, IsNil)
	pro, err := s.State.AddService("pro", s.charm)
	c.Assert(err, IsNil)
	assertNoRelations(c, req)
	assertNoRelations(c, pro)

	// Add a relation, and check we can only do so once.
	proep := state.RelationEndpoint{"pro", "ifce", "foo", state.RoleProvider, state.ScopeGlobal}
	reqep := state.RelationEndpoint{"req", "ifce", "bar", state.RoleRequirer, state.ScopeGlobal}
	err = s.State.AddRelation(proep, reqep)
	c.Assert(err, IsNil)
	err = s.State.AddRelation(proep, reqep)
	c.Assert(err, ErrorMatches, `can't add relation "pro:foo req:bar": relation already exists`)
	assertOneRelation(c, pro, 0, proep, reqep)
	assertOneRelation(c, req, 0, reqep, proep)

	// Remove the relation, and check it can't be removed again.
	err = s.State.RemoveRelation(proep, reqep)
	c.Assert(err, IsNil)
	assertNoRelations(c, pro)
	assertNoRelations(c, req)
	err = s.State.RemoveRelation(proep, reqep)
	c.Assert(err, ErrorMatches, `can't remove relation "pro:foo req:bar": relation doesn't exist`)

	// Check that we can add it again if we want to; but this time,
	// give one of the endpoints container scope and check that both
	// resulting service relations get that scope.
	reqep.RelationScope = state.ScopeContainer
	err = s.State.AddRelation(proep, reqep)
	c.Assert(err, IsNil)
	// After adding relation, make proep container-scoped as well, for
	// simplicity of testing.
	proep.RelationScope = state.ScopeContainer
	assertOneRelation(c, pro, 1, proep, reqep)
	assertOneRelation(c, req, 1, reqep, proep)
}

func (s *RelationSuite) TestPeerRelation(c *C) {
	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)
	peerep := state.RelationEndpoint{"peer", "ifce", "baz", state.RolePeer, state.ScopeGlobal}
	assertNoRelations(c, peer)

	// Add a relation, and check we can only do so once.
	err = s.State.AddRelation(peerep)
	c.Assert(err, IsNil)
	err = s.State.AddRelation(peerep)
	c.Assert(err, ErrorMatches, `can't add relation "peer:baz": relation already exists`)
	assertOneRelation(c, peer, 0, peerep)

	// Remove the relation, and check it can't be removed again.
	err = s.State.RemoveRelation(peerep)
	c.Assert(err, IsNil)
	assertNoRelations(c, peer)
	err = s.State.RemoveRelation(peerep)
	c.Assert(err, ErrorMatches, `can't remove relation "peer:baz": relation doesn't exist`)
}

func assertNoRelations(c *C, srv *state.Service) {
	rels, err := srv.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, 0)
}

func assertOneRelation(c *C, srv *state.Service, relId int, endpoints ...state.RelationEndpoint) {
	rels, err := srv.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, 1)
	rel := rels[0]
	c.Assert(rel.Id(), Equals, relId)
	name := srv.Name()
	expectEp := endpoints[0]
	ep, err := rel.Endpoint(name)
	c.Assert(err, IsNil)
	c.Assert(ep, DeepEquals, expectEp)
	if len(endpoints) == 2 {
		expectEp = endpoints[1]
	}
	eps, err := rel.RelatedEndpoints(name)
	c.Assert(err, IsNil)
	c.Assert(eps, DeepEquals, []state.RelationEndpoint{expectEp})
}
