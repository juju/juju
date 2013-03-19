package statecmd_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
)

type DestroyRelationSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&DestroyRelationSuite{})

func (s *DestroyRelationSuite) setUpDestroyRelationScenario(c *C) {
	// Create some services.
	_, err := s.State.AddService("wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, IsNil)

	_, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)

	_, err = s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)

	_, err = s.State.AddService("riak", s.AddTestingCharm(c, "riak"))
	c.Assert(err, IsNil)
}

func (s *DestroyRelationSuite) TestNonExistentRelation(c *C) {
	s.setUpDestroyRelationScenario(c)
	err := statecmd.DestroyRelation(s.State, params.DestroyRelation{
		Endpoints: []string{"riak", "wordpress"},
	})
	c.Assert(err, ErrorMatches, "no relations found")
}

func (s *DestroyRelationSuite) TestSuccessfullyDestroyRelation(c *C) {
	s.setUpDestroyRelationScenario(c)

	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	err = statecmd.DestroyRelation(s.State, params.DestroyRelation{
		Endpoints: []string{"wordpress", "mysql"},
	})
	c.Assert(err, IsNil)

	// Show that the relation was removed.
	c.Assert(state.IsNotFound(rel.Refresh()), Equals, true)
}

func (s *DestroyRelationSuite) TestSuccessfullyDestroyRelationSwapped(c *C) {
	// Show that the order of the services listed in the DestroyRelation call
	// does not matter.  This is a repeat of the previous test with the service
	// names swapped.
	s.setUpDestroyRelationScenario(c)

	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	err = statecmd.DestroyRelation(s.State, params.DestroyRelation{
		Endpoints: []string{"mysql", "wordpress"},
	})
	c.Assert(err, IsNil)

	// Show that the relation was removed.
	c.Assert(state.IsNotFound(rel.Refresh()), Equals, true)
}

func (s *DestroyRelationSuite) TestCallWithOnlyOneEndpoint(c *C) {
	s.setUpDestroyRelationScenario(c)

	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	err = statecmd.DestroyRelation(s.State, params.DestroyRelation{
		Endpoints: []string{"wordpress"},
	})
	c.Assert(err, ErrorMatches, "a relation must involve two services")
}

func (s *DestroyRelationSuite) TestDestroyAlreadyDestroyedRelation(c *C) {
	s.setUpDestroyRelationScenario(c)

	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	err = statecmd.DestroyRelation(s.State, params.DestroyRelation{
		Endpoints: []string{"wordpress", "mysql"},
	})
	c.Assert(err, IsNil)

	// Show that the relation was removed.
	c.Assert(state.IsNotFound(rel.Refresh()), Equals, true)

	// And try to destroy it again.
	err = statecmd.DestroyRelation(s.State, params.DestroyRelation{
		Endpoints: []string{"wordpress", "mysql"},
	})
	c.Assert(err, ErrorMatches, `relation "wordpress:db mysql:server" not found`)
}
