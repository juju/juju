// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statecmd_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/statecmd"
	jc "launchpad.net/juju-core/testing/checkers"
)

type DestroyRelationSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&DestroyRelationSuite{})

func (s *DestroyRelationSuite) setUpDestroyRelationScenario(c *gc.C) {
	// Create some services.
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	s.AddTestingService(c, "riak", s.AddTestingCharm(c, "riak"))
}

func (s *DestroyRelationSuite) TestAttemptDestroyingNonExistentRelation(c *gc.C) {
	s.setUpDestroyRelationScenario(c)
	err := statecmd.DestroyRelation(s.State, params.DestroyRelation{
		Endpoints: []string{"riak", "wordpress"},
	})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *DestroyRelationSuite) TestSuccessfullyDestroyRelation(c *gc.C) {
	s.setUpDestroyRelationScenario(c)

	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)

	err = statecmd.DestroyRelation(s.State, params.DestroyRelation{
		Endpoints: []string{"wordpress", "mysql"},
	})
	c.Assert(err, gc.IsNil)

	// Show that the relation was removed.
	c.Assert(rel.Refresh(), jc.Satisfies, errors.IsNotFoundError)
}

func (s *DestroyRelationSuite) TestSuccessfullyDestroyRelationSwapped(c *gc.C) {
	// Show that the order of the services listed in the DestroyRelation call
	// does not matter.  This is a repeat of the previous test with the service
	// names swapped.
	s.setUpDestroyRelationScenario(c)

	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)

	err = statecmd.DestroyRelation(s.State, params.DestroyRelation{
		Endpoints: []string{"mysql", "wordpress"},
	})
	c.Assert(err, gc.IsNil)

	// Show that the relation was removed.
	c.Assert(rel.Refresh(), jc.Satisfies, errors.IsNotFoundError)
}

func (s *DestroyRelationSuite) TestAttemptDestroyingWithOnlyOneEndpoint(c *gc.C) {
	s.setUpDestroyRelationScenario(c)

	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)

	err = statecmd.DestroyRelation(s.State, params.DestroyRelation{
		Endpoints: []string{"wordpress"},
	})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *DestroyRelationSuite) TestAttemptDestroyingPeerRelation(c *gc.C) {
	s.setUpDestroyRelationScenario(c)

	err := statecmd.DestroyRelation(s.State, params.DestroyRelation{
		Endpoints: []string{"riak:ring"},
	})
	c.Assert(err, gc.ErrorMatches, `cannot destroy relation "riak:ring": is a peer relation`)
}

func (s *DestroyRelationSuite) TestAttemptDestroyingAlreadyDestroyedRelation(c *gc.C) {
	s.setUpDestroyRelationScenario(c)

	// Add a relation between wordpress and mysql.
	eps, err := s.State.InferEndpoints([]string{"wordpress", "mysql"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)

	err = statecmd.DestroyRelation(s.State, params.DestroyRelation{
		Endpoints: []string{"wordpress", "mysql"},
	})
	c.Assert(err, gc.IsNil)

	// Show that the relation was removed.
	c.Assert(rel.Refresh(), jc.Satisfies, errors.IsNotFoundError)

	// And try to destroy it again.
	err = statecmd.DestroyRelation(s.State, params.DestroyRelation{
		Endpoints: []string{"wordpress", "mysql"},
	})
	c.Assert(err, gc.ErrorMatches, `relation "wordpress:db mysql:server" not found`)
}
