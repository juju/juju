// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/uniter"
	jc "launchpad.net/juju-core/testing/checkers"
)

type relationSuite struct {
	uniterSuite
	commonRelationSuiteMixin

	apiRelation *uniter.Relation
}

var _ = gc.Suite(&relationSuite{})

func (s *relationSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)
	s.commonRelationSuiteMixin.SetUpTest(c, s.uniterSuite)

	var err error
	s.apiRelation, err = s.uniter.Relation(s.stateRelation.Tag())
	c.Assert(err, gc.IsNil)
}

func (s *relationSuite) TearDownTest(c *gc.C) {
	s.uniterSuite.TearDownTest(c)
}

func (s *relationSuite) TestString(c *gc.C) {
	c.Assert(s.apiRelation.String(), gc.Equals, "wordpress:db mysql:server")
}

func (s *relationSuite) TestId(c *gc.C) {
	c.Assert(s.apiRelation.Id(), gc.Equals, s.stateRelation.Id())
}

func (s *relationSuite) TestRefresh(c *gc.C) {
	c.Assert(s.apiRelation.Life(), gc.Equals, params.Alive)

	// EnterScope with mysqlUnit, so the relation will be set to dying
	// when destroyed later.
	myRelUnit, err := s.stateRelation.Unit(s.mysqlUnit)
	c.Assert(err, gc.IsNil)
	err = myRelUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, myRelUnit, true)

	// Destroy it - should set it to dying.
	err = s.stateRelation.Destroy()
	c.Assert(err, gc.IsNil)
	c.Assert(s.apiRelation.Life(), gc.Equals, params.Alive)

	err = s.apiRelation.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.apiRelation.Life(), gc.Equals, params.Dying)

	// Leave scope with mysqlUnit, so the relation will be removed.
	err = myRelUnit.LeaveScope()
	c.Assert(err, gc.IsNil)

	c.Assert(s.apiRelation.Life(), gc.Equals, params.Dying)
	err = s.apiRelation.Refresh()
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *relationSuite) TestEndpoint(c *gc.C) {
	apiEndpoint, err := s.apiRelation.Endpoint()
	c.Assert(err, gc.IsNil)
	c.Assert(apiEndpoint, gc.DeepEquals, &uniter.Endpoint{
		charm.Relation{
			Name:      "db",
			Role:      "requirer",
			Interface: "mysql",
			Optional:  false,
			Limit:     1,
			Scope:     "global",
		},
	})
}

func (s *relationSuite) TestUnit(c *gc.C) {
	_, err := s.apiRelation.Unit(nil)
	c.Assert(err, gc.ErrorMatches, "unit is nil")

	apiUnit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)
	apiRelUnit, err := s.apiRelation.Unit(apiUnit)
	c.Assert(err, gc.IsNil)
	c.Assert(apiRelUnit, gc.NotNil)
	// We just ensure we get the correct type, more tests
	// are done in relationunit_test.go.
	c.Assert(apiRelUnit, gc.FitsTypeOf, (*uniter.RelationUnit)(nil))
}

func (s *relationSuite) TestRelationById(c *gc.C) {
	apiRel, err := s.uniter.RelationById(s.stateRelation.Id())
	c.Assert(err, gc.IsNil)
	c.Assert(apiRel, gc.DeepEquals, s.apiRelation)

	// Add a relation to mysql service, which cannot be retrived.
	otherRel, _, _ := s.addRelatedService(c, "mysql", "logging", s.mysqlUnit)

	// Test some invalid cases.
	for _, relId := range []int{-1, 42, otherRel.Id()} {
		apiRel, err = s.uniter.RelationById(relId)
		c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
		c.Assert(apiRel, gc.IsNil)
	}
}
