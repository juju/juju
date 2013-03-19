package statecmd_test

import (
	. "launchpad.net/gocheck"
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
	err := statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"wordpress", "mysql"},
	})
	c.Assert(err, IsNil)
	// Show that the relation was added.
	wpSvc, _ := s.State.Service("wordpress")
	relCount, err := wpSvc.Relations()
	c.Assert(len(relCount), Equals, 1)
	mySvc, _ := s.State.Service("mysql")
	relCount, err = mySvc.Relations()
	c.Assert(len(relCount), Equals, 1)
}

func (s *AddRelationSuite) TestSuccessfullyAddRelationSwapped(c *C) {
	s.setUpAddRelationScenario(c)
	err := statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"mysql", "wordpress"},
	})
	c.Assert(err, IsNil)
	// Show that the relation was added.
	wpSvc, _ := s.State.Service("wordpress")
	relCount, err := wpSvc.Relations()
	c.Assert(len(relCount), Equals, 1)
	mySvc, _ := s.State.Service("mysql")
	relCount, err = mySvc.Relations()
	c.Assert(len(relCount), Equals, 1)
}

func (s *AddRelationSuite) TestCallWithOnlyOneEndpoint(c *C) {
	s.setUpAddRelationScenario(c)
	err := statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"wordpress"},
	})
	c.Assert(err, ErrorMatches, "a relation must involve two services")
}

func (s *AddRelationSuite) TestCallWithOneEndpointTooMany(c *C) {
	s.setUpAddRelationScenario(c)
	err := statecmd.AddRelation(s.State, params.AddRelation{
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
	err = statecmd.AddRelation(s.State, params.AddRelation{
		Endpoints: []string{"wordpress", "mysql"},
	})
	c.Assert(err, ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation already exists`)
}
