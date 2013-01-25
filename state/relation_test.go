package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
)

type RelationSuite struct {
	ConnSuite
}

var _ = Suite(&RelationSuite{})

func (s *RelationSuite) TestRelationErrors(c *C) {
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, IsNil)
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, IsNil)

	// Check we can't add a relation with services that don't exist.
	msep2 := mysqlEP
	msep2.ServiceName = "yoursql"
	_, err = s.State.AddRelation(msep2, wordpressEP)
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db yoursql:server": .*`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check that interfaces have to match.
	msep3 := mysqlEP
	msep3.Interface = "roflcopter"
	_, err = s.State.AddRelation(msep3, wordpressEP)
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db mysql:server": endpoints do not relate`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check a variety of surprising endpoint combinations.
	_, err = s.State.AddRelation(wordpressEP)
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db": single endpoint must be a peer relation`)
	assertNoRelations(c, wordpress)

	riak, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(riakEP, wordpressEP)
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db riak:ring": endpoints do not relate`)
	assertNoRelations(c, riak)
	assertNoRelations(c, wordpress)

	_, err = s.State.AddRelation(riakEP, riakEP)
	c.Assert(err, ErrorMatches, `cannot add relation "riak:ring riak:ring": endpoints do not relate`)
	assertNoRelations(c, riak)

	_, err = s.State.AddRelation()
	c.Assert(err, ErrorMatches, `cannot add relation "": cannot relate 0 endpoints`)
	_, err = s.State.AddRelation(mysqlEP, wordpressEP, riakEP)
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db mysql:server riak:ring": cannot relate 3 endpoints`)
}

func (s *RelationSuite) TestRetrieveSuccess(c *C) {
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, IsNil)
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, IsNil)
	expect, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, IsNil)
	rel, err := s.State.EndpointsRelation(wordpressEP, mysqlEP)
	check := func() {
		c.Assert(err, IsNil)
		c.Assert(rel.Id(), Equals, expect.Id())
		c.Assert(rel.String(), Equals, expect.String())
	}
	check()
	rel, err = s.State.EndpointsRelation(mysqlEP, wordpressEP)
	check()
	rel, err = s.State.Relation(expect.Id())
	check()
}

func (s *RelationSuite) TestRetrieveNotFound(c *C) {
	subway := state.Endpoint{"subway", "mongodb", "db", state.RoleRequirer, charm.ScopeGlobal}
	mongo := state.Endpoint{"mongo", "mongodb", "server", state.RoleProvider, charm.ScopeGlobal}
	_, err := s.State.EndpointsRelation(subway, mongo)
	c.Assert(err, ErrorMatches, `relation "subway:db mongo:server" not found`)
	c.Assert(state.IsNotFound(err), Equals, true)

	_, err = s.State.Relation(999)
	c.Assert(err, ErrorMatches, `relation 999 not found`)
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *RelationSuite) TestProviderRequirerRelation(c *C) {
	// Add a relation, and check we can only do so once.
	wordpress, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, IsNil)
	mysql, err := s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db mysql:server": .*`)
	assertOneRelation(c, mysql, 0, mysqlEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, mysqlEP)

	// Destroy the relation, and check we can destroy again without error.
	err = rel.Destroy()
	c.Assert(err, IsNil)
	err = rel.Destroy()
	c.Assert(err, IsNil)
	err = rel.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)

	// Check that we can add it again if we want to; but this time,
	// give one of the endpoints container scope and check that both
	// resulting service relations get that scope.
	wordpressEP.RelationScope = charm.ScopeContainer
	_, err = s.State.AddRelation(mysqlEP, wordpressEP)
	c.Assert(err, IsNil)
	// After adding relation, make mysqlEP container-scoped as well, for
	// simplicity of testing.
	mysqlEP.RelationScope = charm.ScopeContainer
	assertOneRelation(c, mysql, 2, mysqlEP, wordpressEP)
	assertOneRelation(c, wordpress, 2, wordpressEP, mysqlEP)
}

func (s *RelationSuite) TestRefresh(c *C) {
	riak, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(riakEP)
	c.Assert(err, IsNil)

	rels, err := riak.Relations()
	c.Assert(err, IsNil)
	rel1 := rels[0]
	err = rel.Destroy()
	c.Assert(err, IsNil)

	c.Assert(rel1.Life(), Equals, state.Alive)
	err = rel1.Refresh()
	c.Assert(state.IsNotFound(err), Equals, true)
}

func (s *RelationSuite) TestDestroy(c *C) {
	// Add a relation, and check we can only do so once.
	riak, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(riakEP)
	c.Assert(err, IsNil)

	err = rel.Destroy()
	c.Assert(err, IsNil)
	_, err = s.State.Relation(rel.Id())
	c.Assert(state.IsNotFound(err), Equals, true)
	_, err = s.State.EndpointsRelation(riakEP)
	c.Assert(state.IsNotFound(err), Equals, true)
	rels, err := riak.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, 0)
}

func (s *RelationSuite) TestPeerRelation(c *C) {
	riak, err := s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, IsNil)
	assertNoRelations(c, riak)

	// Add a relation, and check we can only do so once.
	rel, err := s.State.AddRelation(riakEP)
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(riakEP)
	c.Assert(err, ErrorMatches, `cannot add relation "riak:ring": .*`)
	assertOneRelation(c, riak, 0, riakEP)

	// Remove the relation, and check a second removal succeeds.
	err = rel.Destroy()
	c.Assert(err, IsNil)
	assertNoRelations(c, riak)
	err = rel.Destroy()
	c.Assert(err, IsNil)
}

func assertNoRelations(c *C, srv *state.Service) {
	rels, err := srv.Relations()
	c.Assert(err, IsNil)
	c.Assert(rels, HasLen, 0)
}

func assertOneRelation(c *C, srv *state.Service, relId int, endpoints ...state.Endpoint) {
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
	c.Assert(eps, DeepEquals, []state.Endpoint{expectEp})
}
