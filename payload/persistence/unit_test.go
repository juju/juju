// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence_test

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/persistence"
)

var _ = gc.Suite(&payloadsPersistenceSuite{})

type payloadsPersistenceSuite struct {
	persistence.BaseSuite
}

func (s *payloadsPersistenceSuite) TestTrackOkay(c *gc.C) {
	pl := s.NewPayload("docker", "payloadA/payloadA-xyz")

	wp := s.NewPersistence()
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	okay, err := wp.Track(id, pl)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.Stub.CheckCallNames(c, "All", "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "payloads",
			Id:     "payload#a-unit/0#f47ac10b-58cc-4372-a567-0e02b2c3d479",
			Assert: txn.DocMissing,
			Insert: &persistence.PayloadDoc{
				DocID:  "payload#a-unit/0#f47ac10b-58cc-4372-a567-0e02b2c3d479",
				UnitID: "a-unit/0",

				Name:  "payloadA",
				Type:  "docker",
				RawID: "payloadA-xyz",
				State: "running",
			},
		},
	}})
}

func (s *payloadsPersistenceSuite) TestTrackAlreadyExists(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	pl := s.NewPayload("docker", "payloadA/payloadA-xyz")
	s.SetDoc(id, pl)
	s.Stub.SetErrors(nil, txn.ErrAborted)

	wp := s.NewPersistence()
	okay, err := wp.Track(id, pl)

	c.Check(okay, jc.IsFalse)
	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
	s.Stub.CheckCallNames(c, "All")
	s.State.CheckOps(c, nil)
}

func (s *payloadsPersistenceSuite) TestTrackFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(nil, failure)
	pl := s.NewPayload("docker", "payloadA")

	pp := s.NewPersistence()
	_, err := pp.Track(id, pl)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.Stub.CheckCallNames(c, "All", "Run")
}

func (s *payloadsPersistenceSuite) TestSetStatusOkay(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.NewPayload("docker", "payloadA/payloadA-xyz")
	s.SetDoc(id, pl)

	pp := s.NewPersistence()
	okay, err := pp.SetStatus(id, payload.StateRunning)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsTrue)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "payloads",
			Id:     "payload#a-unit/0#f47ac10b-58cc-4372-a567-0e02b2c3d479",
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{
					{"state", payload.StateRunning},
				}},
			},
		},
	}})
}

func (s *payloadsPersistenceSuite) TestSetStatusMissing(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	s.Stub.SetErrors(txn.ErrAborted)

	pp := s.NewPersistence()
	okay, err := pp.SetStatus(id, payload.StateRunning)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(okay, jc.IsFalse)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "payloads",
			Id:     "payload#a-unit/0#f47ac10b-58cc-4372-a567-0e02b2c3d479",
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{
					{"state", payload.StateRunning},
				}},
			},
		},
	}})
}

func (s *payloadsPersistenceSuite) TestSetStatusFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.NewPayload("docker", "payloadA/payloadA-xyz")
	s.SetDoc(id, pl)
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, err := pp.SetStatus(id, payload.StateRunning)

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *payloadsPersistenceSuite) TestListOkay(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.NewPayload("docker", "payloadA/xyz")
	s.SetDoc(id, pl)
	other := s.NewPayload("docker", "payloadB/abc")
	s.SetDoc("f47ac10b-58cc-4372-a567-0e02b2c3d480", other)

	pp := s.NewPersistence()
	payloads, missing, err := pp.List(id)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(payloads, jc.DeepEquals, []payload.Payload{pl})
	c.Check(missing, gc.HasLen, 0)
}

func (s *payloadsPersistenceSuite) TestListSomeMissing(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.NewPayload("docker", "payloadB/abc")
	s.SetDoc(id, pl)
	other := s.NewPayload("docker", "payloadA/xyz")
	s.SetDoc("f47ac10b-58cc-4372-a567-0e02b2c3d480", other)

	missingID := "f47ac10b-58cc-4372-a567-0e02b2c3d481"
	pp := s.NewPersistence()
	payloads, missing, err := pp.List(id, missingID)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(payloads, jc.DeepEquals, []payload.Payload{pl})
	c.Check(missing, jc.DeepEquals, []string{missingID})
}

func (s *payloadsPersistenceSuite) TestListEmpty(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pp := s.NewPersistence()
	payloads, missing, err := pp.List(id)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(payloads, gc.HasLen, 0)
	c.Check(missing, jc.DeepEquals, []string{id})
}

func (s *payloadsPersistenceSuite) TestListFailure(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, _, err := pp.List()

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *payloadsPersistenceSuite) TestListAllOkay(c *gc.C) {
	existing := s.NewPayloads("docker", "payloadA/xyz", "payloadB/abc")
	for i, pl := range existing {
		s.SetDoc(fmt.Sprintf("%d", i), pl)
	}

	pp := s.NewPersistence()
	payloads, err := pp.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	sort.Sort(byName(payloads))
	sort.Sort(byName(existing))
	c.Check(payloads, jc.DeepEquals, existing)
}

func (s *payloadsPersistenceSuite) TestListAllEmpty(c *gc.C) {
	pp := s.NewPersistence()
	payloads, err := pp.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(payloads, gc.HasLen, 0)
}

type byName []payload.Payload

func (b byName) Len() int           { return len(b) }
func (b byName) Less(i, j int) bool { return b[i].FullID() < b[j].FullID() }
func (b byName) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

func (s *payloadsPersistenceSuite) TestListAllFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, err := pp.ListAll()

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *payloadsPersistenceSuite) TestUntrackOkay(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.NewPayload("docker", "payloadA/xyz")
	s.SetDoc(id, pl)

	pp := s.NewPersistence()
	found, err := pp.Untrack(id)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.IsTrue)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "payloads",
			Id:     "payload#a-unit/0#f47ac10b-58cc-4372-a567-0e02b2c3d479",
			Assert: txn.DocExists,
			Remove: true,
		},
	}})
}

func (s *payloadsPersistenceSuite) TestUntrackMissing(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	s.Stub.SetErrors(txn.ErrAborted)

	pp := s.NewPersistence()
	found, err := pp.Untrack(id)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(found, jc.IsFalse)
	s.Stub.CheckCallNames(c, "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "payloads",
			Id:     "payload#a-unit/0#f47ac10b-58cc-4372-a567-0e02b2c3d479",
			Assert: txn.DocExists,
			Remove: true,
		},
	}})
}

func (s *payloadsPersistenceSuite) TestUntrackFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, err := pp.Untrack(id)

	c.Check(errors.Cause(err), gc.Equals, failure)
}
