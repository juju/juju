// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
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
	err = s.activeRel.SetStatus(status.StatusInfo{Status: status.Joined}, status.NoopStatusHistoryRecorder)
	c.Assert(err, jc.ErrorIsNil)

	eps, err = s.State.InferEndpoints("wordpress2", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	s.suspendedRel, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	err = s.suspendedRel.SetStatus(status.StatusInfo{Status: status.Suspended}, status.NoopStatusHistoryRecorder)
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

	rc, err := s.State.RemoteConnectionStatus("offer-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc.TotalConnectionCount(), gc.Equals, 2)
	c.Assert(rc.ActiveConnectionCount(), gc.Equals, 1)

	all, err := s.State.OfferConnections("offer-uuid")
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

func (s *offerConnectionsSuite) TestAddOfferConnectionNotFound(c *gc.C) {
	// Note: missing RelationKey to trigger a not-found error.
	_, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, jc.ErrorIsNil)

	rc, err := s.State.RemoteConnectionStatus("offer-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc.TotalConnectionCount(), gc.Equals, 1)
	c.Assert(rc.ActiveConnectionCount(), gc.Equals, 0)
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

	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		RelationKey:     s.activeRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, jc.ErrorIs, errors.AlreadyExists)
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

	_, err = s.State.OfferConnectionForRelation("some-key")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	obtained, err := s.State.OfferConnectionForRelation(s.activeRel.Tag().Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained.RelationId(), gc.Equals, oc.RelationId())
	c.Assert(obtained.RelationKey(), gc.Equals, oc.RelationKey())
	c.Assert(obtained.OfferUUID(), gc.Equals, oc.OfferUUID())
}

func (s *offerConnectionsSuite) TestOfferConnectionsForUser(c *gc.C) {
	oc, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		RelationKey:     s.activeRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, jc.ErrorIsNil)

	obtained, err := s.State.OfferConnectionsForUser("mary")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.HasLen, 0)
	obtained, err = s.State.OfferConnectionsForUser("fred")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.HasLen, 1)
	c.Assert(obtained[0].OfferUUID(), gc.Equals, oc.OfferUUID())
	c.Assert(obtained[0].UserName(), gc.Equals, oc.UserName())
}

func (s *offerConnectionsSuite) TestAllOfferConnections(c *gc.C) {
	obtained, err := s.State.AllOfferConnections()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.HasLen, 0)

	oc1, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.activeRel.Id(),
		RelationKey:     s.activeRel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid1",
	})
	c.Assert(err, jc.ErrorIsNil)

	oc2, err := s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: testing.ModelTag.Id(),
		RelationId:      s.suspendedRel.Id(),
		RelationKey:     s.suspendedRel.Tag().Id(),
		Username:        "mary",
		OfferUUID:       "offer-uuid2",
	})
	c.Assert(err, jc.ErrorIsNil)

	obtained, err = s.State.AllOfferConnections()
	c.Assert(err, jc.ErrorIsNil)

	// Get strings for comparison. Comparing pointers is no good.
	obtainedStr := make([]string, len(obtained))
	for i, v := range obtained {
		obtainedStr[i] = v.String()
	}
	c.Assert(obtainedStr, jc.SameContents, []string{oc1.String(), oc2.String()})

}
