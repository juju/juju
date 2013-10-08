// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statecmd_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

type AddRelationSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&AddRelationSuite{})

func (s *AddRelationSuite) setUpAddRelationScenario(c *gc.C) {
	// Create some services.
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
}

func (s *AddRelationSuite) checkEndpoints(c *gc.C, res params.AddRelationResults) {
	c.Assert(res.Endpoints["wordpress"], gc.DeepEquals, charm.Relation{
		Name:      "db",
		Role:      charm.RelationRole("requirer"),
		Interface: "mysql",
		Optional:  false,
		Limit:     1,
		Scope:     charm.RelationScope("global"),
	})
	c.Assert(res.Endpoints["mysql"], gc.DeepEquals, charm.Relation{
		Name:      "server",
		Role:      charm.RelationRole("provider"),
		Interface: "mysql",
		Optional:  false,
		Limit:     0,
		Scope:     charm.RelationScope("global"),
	})
}

func (s *AddRelationSuite) TestSuccessfullyAddRelation(c *gc.C) {
	s.setUpAddRelationScenario(c)
	res, err := statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"wordpress", "mysql"},
	})
	c.Assert(err, gc.IsNil)
	s.checkEndpoints(c, res)
	// Show that the relation was added.
	wpSvc, err := s.State.Service("wordpress")
	c.Assert(err, gc.IsNil)
	rels, err := wpSvc.Relations()
	c.Assert(len(rels), gc.Equals, 1)
	mySvc, err := s.State.Service("mysql")
	c.Assert(err, gc.IsNil)
	rels, err = mySvc.Relations()
	c.Assert(len(rels), gc.Equals, 1)
}

func (s *AddRelationSuite) TestSuccessfullyAddRelationSwapped(c *gc.C) {
	s.setUpAddRelationScenario(c)
	res, err := statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"mysql", "wordpress"},
	})
	c.Assert(err, gc.IsNil)
	s.checkEndpoints(c, res)
	// Show that the relation was added.
	wpSvc, err := s.State.Service("wordpress")
	c.Assert(err, gc.IsNil)
	rels, err := wpSvc.Relations()
	c.Assert(len(rels), gc.Equals, 1)
	mySvc, err := s.State.Service("mysql")
	c.Assert(err, gc.IsNil)
	rels, err = mySvc.Relations()
	c.Assert(len(rels), gc.Equals, 1)
}

func (s *AddRelationSuite) TestCallWithOnlyOneEndpoint(c *gc.C) {
	s.setUpAddRelationScenario(c)
	_, err := statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"wordpress"},
	})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *AddRelationSuite) TestCallWithOneEndpointTooMany(c *gc.C) {
	s.setUpAddRelationScenario(c)
	_, err := statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"wordpress", "mysql", "logging"},
	})
	c.Assert(err, gc.ErrorMatches, "cannot relate 3 endpoints")
}

func (s *AddRelationSuite) TestAddAlreadyAddedRelation(c *gc.C) {
	s.setUpAddRelationScenario(c)
	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	// And try to add it again.
	_, err = statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"wordpress", "mysql"},
	})
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation already exists`)
}
