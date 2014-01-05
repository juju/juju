// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	jc "launchpad.net/juju-core/testing/checkers"
)

type RelationSuite struct {
	ConnSuite
}

var _ = gc.Suite(&RelationSuite{})

func (s *RelationSuite) TestAddRelationErrors(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, gc.IsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, gc.IsNil)
	riak := s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, gc.IsNil)

	// Check we can't add a relation with services that don't exist.
	yoursqlEP := mysqlEP
	yoursqlEP.ServiceName = "yoursql"
	_, err = s.State.AddRelation(yoursqlEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db yoursql:server": service "yoursql" does not exist`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check that interfaces have to match.
	msep3 := mysqlEP
	msep3.Interface = "roflcopter"
	_, err = s.State.AddRelation(msep3, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": endpoints do not relate`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check a variety of surprising endpoint combinations.
	_, err = s.State.AddRelation(wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db": relation must have two endpoints`)
	assertNoRelations(c, wordpress)

	_, err = s.State.AddRelation(riakEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db riak:ring": endpoints do not relate`)
	assertOneRelation(c, riak, 0, riakEP)
	assertNoRelations(c, wordpress)

	_, err = s.State.AddRelation(riakEP, riakEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "riak:ring riak:ring": endpoints do not relate`)
	assertOneRelation(c, riak, 0, riakEP)

	_, err = s.State.AddRelation()
	c.Assert(err, gc.ErrorMatches, `cannot add relation "": relation must have two endpoints`)
	_, err = s.State.AddRelation(mysqlEP, wordpressEP, riakEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server riak:ring": relation must have two endpoints`)
	assertOneRelation(c, riak, 0, riakEP)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check that a relation can't be added to a Dying service.
	_, err = wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	err = wordpress.Destroy()
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddRelation(mysqlEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": service "wordpress" is not alive`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)
}

func (s *RelationSuite) TestRetrieveSuccess(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, gc.IsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, gc.IsNil)
	expect, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, gc.IsNil)
	rel, err := s.State.EndpointsRelation(wordpressEP, mysqlEP)
	check := func() {
		c.Assert(err, gc.IsNil)
		c.Assert(rel.Id(), gc.Equals, expect.Id())
		c.Assert(rel.String(), gc.Equals, expect.String())
	}
	check()
	rel, err = s.State.EndpointsRelation(mysqlEP, wordpressEP)
	check()
	rel, err = s.State.Relation(expect.Id())
	check()
}

func (s *RelationSuite) TestRetrieveNotFound(c *gc.C) {
	subway := state.Endpoint{
		ServiceName: "subway",
		Relation: charm.Relation{
			Name:      "db",
			Interface: "mongodb",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		},
	}
	mongo := state.Endpoint{
		ServiceName: "mongo",
		Relation: charm.Relation{
			Name:      "server",
			Interface: "mongodb",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	_, err := s.State.EndpointsRelation(subway, mongo)
	c.Assert(err, gc.ErrorMatches, `relation "subway:db mongo:server" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	_, err = s.State.Relation(999)
	c.Assert(err, gc.ErrorMatches, `relation 999 not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *RelationSuite) TestAddRelation(c *gc.C) {
	// Add a relation.
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, gc.IsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, gc.IsNil)
	assertOneRelation(c, mysql, 0, mysqlEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, mysqlEP)

	// Check we cannot re-add the same relation, regardless of endpoint ordering.
	_, err = s.State.AddRelation(mysqlEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation already exists`)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation already exists`)
	assertOneRelation(c, mysql, 0, mysqlEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, mysqlEP)
}

func (s *RelationSuite) TestAddRelationSeriesNeedNotMatch(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, gc.IsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddSeriesCharm(c, "mysql", "otherseries"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, gc.IsNil)
	assertOneRelation(c, mysql, 0, mysqlEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, mysqlEP)
}

func (s *RelationSuite) TestAddContainerRelation(c *gc.C) {
	// Add a relation.
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("juju-info")
	c.Assert(err, gc.IsNil)
	logging := s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	loggingEP, err := logging.Endpoint("info")
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddRelation(wordpressEP, loggingEP)
	c.Assert(err, gc.IsNil)

	// Check that the endpoints both have container scope.
	wordpressEP.Scope = charm.ScopeContainer
	assertOneRelation(c, logging, 0, loggingEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, loggingEP)

	// Check we cannot re-add the same relation, regardless of endpoint ordering.
	_, err = s.State.AddRelation(loggingEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "logging:info wordpress:juju-info": relation already exists`)
	_, err = s.State.AddRelation(wordpressEP, loggingEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "logging:info wordpress:juju-info": relation already exists`)
	assertOneRelation(c, logging, 0, loggingEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, loggingEP)
}

func (s *RelationSuite) TestAddContainerRelationSeriesMustMatch(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("juju-info")
	c.Assert(err, gc.IsNil)
	logging := s.AddTestingService(c, "logging", s.AddSeriesCharm(c, "logging", "otherseries"))
	loggingEP, err := logging.Endpoint("info")
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddRelation(wordpressEP, loggingEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "logging:info wordpress:juju-info": principal and subordinate services' series must match`)
}

func (s *RelationSuite) TestDestroyRelation(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)

	// Test that the relation can be destroyed.
	err = rel.Destroy()
	c.Assert(err, gc.IsNil)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check that a second destroy is a no-op.
	err = rel.Destroy()
	c.Assert(err, gc.IsNil)

	// Create a new relation and check that refreshing the old does not find
	// the new.
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func (s *RelationSuite) TestDestroyPeerRelation(c *gc.C) {
	// Check that a peer relation cannot be destroyed directly.
	riakch := s.AddTestingCharm(c, "riak")
	riak := s.AddTestingService(c, "riak", riakch)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, gc.IsNil)
	rel := assertOneRelation(c, riak, 0, riakEP)
	err = rel.Destroy()
	c.Assert(err, gc.ErrorMatches, `cannot destroy relation "riak:ring": is a peer relation`)
	assertOneRelation(c, riak, 0, riakEP)

	// Check that it is destroyed when the service is destroyed.
	err = riak.Destroy()
	c.Assert(err, gc.IsNil)
	assertNoRelations(c, riak)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	// Create a new service (and hence a new relation in the background); check
	// that refreshing the old one does not accidentally get the new one.
	newriak := s.AddTestingService(c, "riak", riakch)
	assertOneRelation(c, newriak, 1, riakEP)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)
}

func assertNoRelations(c *gc.C, srv *state.Service) {
	rels, err := srv.Relations()
	c.Assert(err, gc.IsNil)
	c.Assert(rels, gc.HasLen, 0)
}

func assertOneRelation(c *gc.C, srv *state.Service, relId int, endpoints ...state.Endpoint) *state.Relation {
	rels, err := srv.Relations()
	c.Assert(err, gc.IsNil)
	c.Assert(rels, gc.HasLen, 1)
	rel := rels[0]
	c.Assert(rel.Id(), gc.Equals, relId)
	name := srv.Name()
	expectEp := endpoints[0]
	ep, err := rel.Endpoint(name)
	c.Assert(err, gc.IsNil)
	c.Assert(ep, gc.DeepEquals, expectEp)
	if len(endpoints) == 2 {
		expectEp = endpoints[1]
	}
	eps, err := rel.RelatedEndpoints(name)
	c.Assert(err, gc.IsNil)
	c.Assert(eps, gc.DeepEquals, []state.Endpoint{expectEp})
	return rel
}
