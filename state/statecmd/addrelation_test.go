// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

type AddRelationSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&AddRelationSuite{})

func (s *AddRelationSuite) setUpAddRelationScenario(c *C) {
	// Create some services.
	_, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)
	_, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)
	_, err = s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
}

func (s *AddRelationSuite) checkEndpoints(c *C, res params.AddRelationResults) {
	c.Assert(res.Endpoints["wordpress"], DeepEquals, charm.Relation{
		Name:      "db",
		Role:      charm.RelationRole("requirer"),
		Interface: "mysql",
		Optional:  false,
		Limit:     1,
		Scope:     charm.RelationScope("global"),
	})
	c.Assert(res.Endpoints["mysql"], DeepEquals, charm.Relation{
		Name:      "server",
		Role:      charm.RelationRole("provider"),
		Interface: "mysql",
		Optional:  false,
		Limit:     0,
		Scope:     charm.RelationScope("global"),
	})
}

func (s *AddRelationSuite) TestSuccessfullyAddRelation(c *C) {
	s.setUpAddRelationScenario(c)
	res, err := statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"wordpress", "mysql"},
	})
	c.Assert(err, IsNil)
	s.checkEndpoints(c, res)
	// Show that the relation was added.
	wpSvc, err := s.State.Service("wordpress")
	c.Assert(err, IsNil)
	rels, err := wpSvc.Relations()
	c.Assert(len(rels), Equals, 1)
	mySvc, err := s.State.Service("mysql")
	c.Assert(err, IsNil)
	rels, err = mySvc.Relations()
	c.Assert(len(rels), Equals, 1)
}

func (s *AddRelationSuite) TestSuccessfullyAddRelationSwapped(c *C) {
	s.setUpAddRelationScenario(c)
	res, err := statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"mysql", "wordpress"},
	})
	c.Assert(err, IsNil)
	s.checkEndpoints(c, res)
	// Show that the relation was added.
	wpSvc, err := s.State.Service("wordpress")
	c.Assert(err, IsNil)
	rels, err := wpSvc.Relations()
	c.Assert(len(rels), Equals, 1)
	mySvc, err := s.State.Service("mysql")
	c.Assert(err, IsNil)
	rels, err = mySvc.Relations()
	c.Assert(len(rels), Equals, 1)
}

func (s *AddRelationSuite) TestCallWithOnlyOneEndpoint(c *C) {
	s.setUpAddRelationScenario(c)
	_, err := statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"wordpress"},
	})
	c.Assert(err, ErrorMatches, "no relations found")
}

func (s *AddRelationSuite) TestCallWithOneEndpointTooMany(c *C) {
	s.setUpAddRelationScenario(c)
	_, err := statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"wordpress", "mysql", "logging"},
	})
	c.Assert(err, ErrorMatches, "cannot relate 3 endpoints")
}

func (s *AddRelationSuite) TestAddAlreadyAddedRelation(c *C) {
	s.setUpAddRelationScenario(c)
	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	// And try to add it again.
	_, err = statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"wordpress", "mysql"},
	})
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation already exists`)
}
