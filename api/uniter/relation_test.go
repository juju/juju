// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/leadership"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/status"
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
	s.apiRelation, err = s.uniter.Relation(s.stateRelation.Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationSuite) TearDownTest(c *gc.C) {
	s.uniterSuite.TearDownTest(c)
}

func (s *relationSuite) TestString(c *gc.C) {
	c.Assert(s.apiRelation.String(), gc.Equals, "wordpress:db mysql:server")
}

func (s *relationSuite) TestIdAndTag(c *gc.C) {
	c.Assert(s.apiRelation.Id(), gc.Equals, s.stateRelation.Id())
	c.Assert(s.apiRelation.Tag(), gc.Equals, s.stateRelation.Tag().(names.RelationTag))
}

func (s *relationSuite) TestOtherApplication(c *gc.C) {
	c.Assert(s.apiRelation.OtherApplication(), gc.Equals, "mysql")
}

func (s *relationSuite) TestRefresh(c *gc.C) {
	c.Assert(s.apiRelation.Life(), gc.Equals, life.Alive)
	c.Assert(s.apiRelation.Suspended(), jc.IsTrue)

	// EnterScope with mysqlUnit, so the relation will be set to dying
	// when destroyed later.
	myRelUnit, err := s.stateRelation.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = myRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, myRelUnit, true)

	// Destroy it - should set it to dying.
	err = s.stateRelation.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	// Update suspended as well.
	err = s.stateRelation.SetSuspended(false, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiRelation.Life(), gc.Equals, life.Alive)
	c.Assert(s.apiRelation.Suspended(), jc.IsTrue)

	err = s.apiRelation.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiRelation.Life(), gc.Equals, life.Dying)
	c.Assert(s.apiRelation.Suspended(), jc.IsFalse)

	// Leave scope with mysqlUnit, so the relation will be removed.
	err = myRelUnit.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.apiRelation.Life(), gc.Equals, life.Dying)
	c.Assert(s.apiRelation.Suspended(), jc.IsFalse)
	err = s.apiRelation.Refresh()
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *relationSuite) TestSetStatus(c *gc.C) {
	claimer := leadership.NewClient(s.st)
	err := claimer.ClaimLeadership("wordpress", "wordpress/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	err = s.apiRelation.SetStatus(relation.Suspended)
	c.Assert(err, jc.ErrorIsNil)
	relStatus, err := s.stateRelation.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relStatus.Status, gc.Equals, status.Suspended)
}

func (s *relationSuite) TestEndpoint(c *gc.C) {
	apiEndpoint, err := s.apiRelation.Endpoint()
	c.Assert(err, jc.ErrorIsNil)
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
	apiUnit, err := s.uniter.Unit(names.NewUnitTag("wordpress/0"))
	c.Assert(err, jc.ErrorIsNil)
	apiRelUnit, err := s.apiRelation.Unit(apiUnit.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiRelUnit, gc.NotNil)
	// We just ensure we get the correct type, more tests
	// are done in relationunit_test.go.
	c.Assert(apiRelUnit, gc.FitsTypeOf, (*uniter.RelationUnit)(nil))
}

func (s *relationSuite) TestRelationById(c *gc.C) {
	apiRel, err := s.uniter.RelationById(s.stateRelation.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiRel, gc.DeepEquals, s.apiRelation)

	// Add a relation to mysql application, which cannot be retrieved.
	otherRel, _, _ := s.addRelatedApplication(c, "mysql", "logging", s.mysqlUnit)

	// Test some invalid cases.
	for _, relId := range []int{-1, 42, otherRel.Id()} {
		apiRel, err = s.uniter.RelationById(relId)
		c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
		c.Assert(apiRel, gc.IsNil)
	}
}
