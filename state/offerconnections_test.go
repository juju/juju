// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"fmt"
	"github.com/juju/errors"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testing"
)

type offerConnectionsSuite struct {
	ConnSuite

	suspendedRel *state.Relation
	activeRel    *state.Relation
}

var _ = gc.Suite(&offerConnectionsSuite{})

func (s *offerConnectionsSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	wpCh := s.AddTestingCharm(c, "wordpress")
	s.AddTestingApplication(c, "wordpress", wpCh)
	s.AddTestingApplication(c, "wordpress2", wpCh)

	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	s.activeRel, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	err = s.activeRel.SetStatus(status.StatusInfo{Status: status.Joined})
	c.Assert(err, jc.ErrorIsNil)

	eps, err = s.State.InferEndpoints("wordpress2", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	s.suspendedRel, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	err = s.suspendedRel.SetStatus(status.StatusInfo{Status: status.Suspended})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *offerConnectionsSuite) TestAddOfferConnection(c *gc.C) {
	oc, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.suspendedRel.Id(),
		RelationKey:     s.suspendedRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(oc.SourceModelUUID(), gc.Equals, testing.ModelTag.Id())
	c.Assert(oc.RelationId(), gc.Equals, s.suspendedRel.Id())
	c.Assert(oc.RelationKey(), gc.Equals, s.suspendedRel.Tag().Id())
	c.Assert(oc.OfferUUID(), gc.Equals, "offer-uuid")
	c.Assert(oc.UserName(), gc.Equals, "fred")

	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		RelationKey:     s.activeRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, jc.ErrorIsNil)

	anotherState, err := s.State.ForModel(s.IAASModel.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	rc, err := anotherState.RemoteConnectionStatus("offer-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc.TotalConnectionCount(), gc.Equals, 2)
	c.Assert(rc.ActiveConnectionCount(), gc.Equals, 1)

	all, err := anotherState.OfferConnections("offer-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(all, gc.HasLen, 2)
	c.Assert(all[0].SourceModelUUID(), gc.Equals, testing.ModelTag.Id())
	c.Assert(all[0].RelationId(), gc.Equals, s.suspendedRel.Id())
	c.Assert(all[0].RelationKey(), gc.Equals, s.suspendedRel.Tag().Id())
	c.Assert(all[0].OfferUUID(), gc.Equals, "offer-uuid")
	c.Assert(all[0].UserName(), gc.Equals, "fred")
	c.Assert(all[0].String(),
		gc.Equals, fmt.Sprintf(`connection to "offer-uuid" by "fred" for relation %d`, s.suspendedRel.Id()))
	c.Assert(all[1].SourceModelUUID(), gc.Equals, testing.ModelTag.Id())
	c.Assert(all[1].RelationId(), gc.Equals, s.activeRel.Id())
	c.Assert(all[1].RelationKey(), gc.Equals, s.activeRel.Tag().Id())
	c.Assert(all[1].OfferUUID(), gc.Equals, "offer-uuid")
	c.Assert(all[1].UserName(), gc.Equals, "fred")
	c.Assert(all[1].String(),
		gc.Equals, fmt.Sprintf(`connection to "offer-uuid" by "fred" for relation %d`, s.activeRel.Id()))
}

func (s *offerConnectionsSuite) TestAddOfferConnectionTwice(c *gc.C) {
	_, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		RelationKey:     s.activeRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, jc.ErrorIsNil)

	anotherState, err := s.State.ForModel(s.IAASModel.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		RelationKey:     s.activeRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *offerConnectionsSuite) TestOfferConnectionForRelation(c *gc.C) {
	oc, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		RelationKey:     s.activeRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, jc.ErrorIsNil)

	anotherState, err := s.State.ForModel(s.IAASModel.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	defer anotherState.Close()

	_, err = anotherState.OfferConnectionForRelation("some-key")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	obtained, err := anotherState.OfferConnectionForRelation(s.activeRel.Tag().Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained.RelationId(), gc.Equals, oc.RelationId())
	c.Assert(obtained.RelationKey(), gc.Equals, oc.RelationKey())
	c.Assert(obtained.OfferUUID(), gc.Equals, oc.OfferUUID())
}
