// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/state"
)

var _ = gc.Suite(&unitPayloadsSuite{})

type unitPayloadsSuite struct {
	basePayloadsSuite
}

func (s *unitPayloadsSuite) TestTrackOkay(c *gc.C) {
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}

	err := ps.Track(pl.Payload)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "List", "Track")
	c.Check(s.persist.payloads, gc.HasLen, 1)
	s.persist.checkPayload(c, pl.Name, pl)
}

func (s *unitPayloadsSuite) TestTrackInvalid(c *gc.C) {
	pl := s.newPayload("", "payloadA/payloadA-xyz")

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}

	err := ps.Track(pl.Payload)

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *unitPayloadsSuite) TestTrackEnsureDefinitionFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}

	err := ps.Track(pl.Payload)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitPayloadsSuite) TestTrackInsertFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}

	err := ps.Track(pl.Payload)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitPayloadsSuite) TestTrackAlreadyExists(c *gc.C) {
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")
	s.persist.setPayload(&pl)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}

	err := ps.Track(pl.Payload)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *unitPayloadsSuite) TestSetStatusOkay(c *gc.C) {
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")
	s.persist.setPayload(&pl)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}

	err := ps.SetStatus(pl.Name, payload.StateRunning)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "SetStatus")
	current := s.persist.payloads[pl.Name]
	c.Check(current.Status, jc.DeepEquals, payload.StateRunning)
}

func (s *unitPayloadsSuite) TestSetStatusFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")
	s.persist.setPayload(&pl)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}
	err := ps.SetStatus(pl.Name, payload.StateRunning)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitPayloadsSuite) TestSetStatusMissing(c *gc.C) {
	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}
	err := ps.SetStatus("payloadA", payload.StateRunning)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *unitPayloadsSuite) TestListOkay(c *gc.C) {
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")
	other := s.newPayload("docker", "payloadB/payloadB-abc")
	s.persist.setPayload(&pl)
	s.persist.setPayload(&other)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
	}
	results, err := ps.List(pl.Name)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "List")
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID:      pl.Name,
		Payload: &pl,
	}})
}

func (s *unitPayloadsSuite) TestListAll(c *gc.C) {
	pl1 := s.newPayload("docker", "payloadA/payloadA-xyz")
	pl2 := s.newPayload("docker", "payloadB/payloadB-abc")
	s.persist.setPayload(&pl1)
	s.persist.setPayload(&pl2)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}
	results, err := ps.List()
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "ListAll")
	c.Assert(results, gc.HasLen, 2)
	if results[0].Payload.Name == "payloadA" {
		c.Check(results[0].Payload, jc.DeepEquals, &pl1)
		c.Check(results[1].Payload, jc.DeepEquals, &pl2)
	} else {
		c.Check(results[0].Payload, jc.DeepEquals, &pl2)
		c.Check(results[1].Payload, jc.DeepEquals, &pl1)
	}
}

func (s *unitPayloadsSuite) TestListFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}
	_, err := ps.List()

	s.stub.CheckCallNames(c, "ListAll")
	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *unitPayloadsSuite) TestListMissing(c *gc.C) {
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")
	s.persist.setPayload(&pl)
	missingName := "not-" + pl.Name

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}
	results, err := ps.List(pl.Name, missingName)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.HasLen, 2)
	c.Check(results[1].Error, gc.NotNil)
	results[1].Error = nil
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID:      pl.Name,
		Payload: &pl,
	}, {
		ID:       missingName,
		NotFound: true,
	}})
}

func (s *unitPayloadsSuite) TestUntrackOkay(c *gc.C) {
	pl := s.newPayload("docker", "payloadA/payloadA-xyz")
	s.persist.setPayload(&pl)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}
	err := ps.Untrack(pl.Name)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Untrack")
	c.Check(s.persist.payloads, gc.HasLen, 0)
}

func (s *unitPayloadsSuite) TestUntrackMissing(c *gc.C) {
	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}
	err := ps.Untrack("payloadA")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Untrack")
	c.Check(s.persist.payloads, gc.HasLen, 0)
}

func (s *unitPayloadsSuite) TestUntrackFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.stub.SetErrors(failure)

	ps := state.UnitPayloads{
		Persist: s.persist,
		Unit:    "a-service/0",
		Machine: "0",
	}
	err := ps.Untrack("payloadA")

	s.stub.CheckCallNames(c, "Untrack")
	c.Check(errors.Cause(err), gc.Equals, failure)
}
