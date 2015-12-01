// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/state"
)

var _ = gc.Suite(&unitPayloadsSuite{})

type unitPayloadsSuite struct {
	basePayloadsSuite
	id string
}

func (s *unitPayloadsSuite) newID() (string, error) {
	s.stub.AddCall("newID")
	if err := s.stub.NextErr(); err != nil {
		return "", errors.Trace(err)
	}

	return s.id, nil
}

func (s *unitPayloadsSuite) TestNewID(c *gc.C) {
	id, err := state.NewID()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(id, jc.Satisfies, utils.IsValidUUIDString)
}

func (s *unitPayloadsSuite) TestTrackOkay(c *gc.C) {
	s.id = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	ps = state.SetNewID(ps, s.newID)

	err := ps.Track(pl)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "newID", "Track")
	c.Check(s.persist.payloads, gc.HasLen, 1)
	s.persist.checkPayload(c, s.id, pl)
}

func (s *unitPayloadsSuite) TestTrackInvalid(c *gc.C) {
	s.id = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.newPayload("", "payloadA/payloadA-xyz")

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	ps = state.SetNewID(ps, s.newID)

	err := ps.Track(pl)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *unitPayloadsSuite) TestTrackEnsureDefinitionFailed(c *gc.C) {
	s.id = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	ps = state.SetNewID(ps, s.newID)

	err := ps.Track(pl)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitPayloadsSuite) TestTrackInsertFailed(c *gc.C) {
	s.id = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	ps = state.SetNewID(ps, s.newID)

	err := ps.Track(pl)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitPayloadsSuite) TestTrackAlreadyExists(c *gc.C) {
	s.id = "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")
	s.persist.setPayload(s.id, &pl)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	ps = state.SetNewID(ps, s.newID)

	err := ps.Track(pl)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *unitPayloadsSuite) TestSetStatusOkay(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")
	s.persist.setPayload(id, &pl)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}

	err := ps.SetStatus(id, payload.StateRunning)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "SetStatus")
	current := s.persist.payloads[id]
	c.Check(current.Status, jc.DeepEquals, payload.StateRunning)
}

func (s *unitPayloadsSuite) TestSetStatusFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")
	s.persist.setPayload(id, &pl)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	err := ps.SetStatus(id, payload.StateRunning)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitPayloadsSuite) TestSetStatusMissing(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	err := ps.SetStatus(id, payload.StateRunning)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *unitPayloadsSuite) TestListOkay(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	otherID := "f47ac10b-58cc-4372-a567-0e02b2c3d480"
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")
	other := s.newPayload("docker", "payloadB/payloadB-abc")
	s.persist.setPayload(id, &pl)
	s.persist.setPayload(otherID, &other)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	results, err := ps.List(id)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "List")
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID:      id,
		Payload: &payload.FullPayloadInfo{Payload: pl},
	}})
}

func (s *unitPayloadsSuite) TestListAll(c *gc.C) {
	id1 := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	id2 := "f47ac10b-58cc-4372-a567-0e02b2c3d480"
	pl1 := s.newPayload("docker", "payloadA/payloadA-xyz")
	pl2 := s.newPayload("docker", "payloadB/payloadB-abc")
	s.persist.setPayload(id1, &pl1)
	s.persist.setPayload(id2, &pl2)
	fpi1 := payload.FullPayloadInfo{Payload: pl1}
	fpi2 := payload.FullPayloadInfo{Payload: pl2}

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	results, err := ps.List()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListAll", "LookUp", "LookUp")
	c.Assert(results, gc.HasLen, 2)
	if results[0].Payload.Name == "payloadA" {
		c.Check(results[0].Payload, jc.DeepEquals, &fpi1)
		c.Check(results[1].Payload, jc.DeepEquals, &fpi2)
	} else {
		c.Check(results[0].Payload, jc.DeepEquals, &fpi2)
		c.Check(results[1].Payload, jc.DeepEquals, &fpi1)
	}
}

func (s *unitPayloadsSuite) TestListFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	_, err := ps.List()

	s.stub.CheckCallNames(c, "ListAll")
	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitPayloadsSuite) TestListMissing(c *gc.C) {
	missingID := "f47ac10b-58cc-4372-a567-0e02b2c3d480"
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")
	s.persist.setPayload(id, &pl)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	results, err := ps.List(id, missingID)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 2)
	c.Check(results[1].Error, gc.NotNil)
	results[1].Error = nil
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID:      id,
		Payload: &payload.FullPayloadInfo{Payload: pl},
	}, {
		ID:       missingID,
		NotFound: true,
	}})
}

func (s *unitPayloadsSuite) TestUntrackOkay(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")
	s.persist.setPayload(id, &pl)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	err := ps.Untrack(id)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Untrack")
	c.Check(s.persist.payloads, gc.HasLen, 0)
}

func (s *unitPayloadsSuite) TestUntrackMissing(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	err := ps.Untrack(id)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Untrack")
	c.Check(s.persist.payloads, gc.HasLen, 0)
}

func (s *unitPayloadsSuite) TestUntrackFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	err := ps.Untrack(id)

	s.stub.CheckCallNames(c, "Untrack")
	c.Check(errors.Cause(err), gc.Equals, failure)
}
