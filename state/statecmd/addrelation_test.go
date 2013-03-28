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

func (s *AddRelationSuite) TestSuccessfullyAddRelation(c *C) {
	s.setUpAddRelationScenario(c)
	endpoints, err := statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"wordpress", "mysql"},
	})
	c.Assert(err, IsNil)
	c.Assert(endpoints.Endpoints["wordpress"].Name, Equals, "db")
	c.Assert(endpoints.Endpoints["wordpress"].Interface, Equals, "mysql")
	c.Assert(endpoints.Endpoints["wordpress"].Scope, Equals, charm.RelationScope("global"))
	c.Assert(endpoints.Endpoints["mysql"].Name, Equals, "server")
	c.Assert(endpoints.Endpoints["mysql"].Interface, Equals, "mysql")
	c.Assert(endpoints.Endpoints["mysql"].Scope, Equals, charm.RelationScope("global"))
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
	c.Assert(res.Endpoints["mysql"].Name, Equals, "server")
	c.Assert(res.Endpoints["mysql"].Interface, Equals, "mysql")
	c.Assert(res.Endpoints["mysql"].Scope, Equals, charm.RelationScope("global"))
	c.Assert(res.Endpoints["wordpress"].Name, Equals, "db")
	c.Assert(res.Endpoints["wordpress"].Interface, Equals, "mysql")
	c.Assert(res.Endpoints["wordpress"].Scope, Equals, charm.RelationScope("global"))
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
	c.Assert(err, ErrorMatches, "a relation must involve two services")
}

func (s *AddRelationSuite) TestCallWithOneEndpointTooMany(c *C) {
	s.setUpAddRelationScenario(c)
	_, err := statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"wordpress", "mysql", "logging"},
	})
	c.Assert(err, ErrorMatches, "a relation must involve two services")
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
