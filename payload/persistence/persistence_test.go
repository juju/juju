// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence_test

import (
	"fmt"
	"reflect"
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

func (s *payloadsPersistenceSuite) TestListAllOkay(c *gc.C) {
	p1 := s.NewPayload("docker", "spam/spam-xyz")
	p1.Labels = []string{"a-tag"}
	s.SetDoc("0", p1)
	p2 := s.NewPayload("docker", "eggs/eggs-xyz")
	p2.Labels = []string{"a-tag"}
	s.SetDoc("0", p2)
	persist := s.NewPersistence()

	payloads, err := persist.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	checkPayloads(c, payloads, p1, p2)
	s.Stub.CheckCallNames(c, "All")
}

func (s *payloadsPersistenceSuite) TestListAllEmpty(c *gc.C) {
	persist := s.NewPersistence()

	payloads, err := persist.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(payloads, gc.HasLen, 0)
	s.Stub.CheckCallNames(c, "All")
}

func (s *payloadsPersistenceSuite) TestListAllFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)
	persist := s.NewPersistence()

	_, err := persist.ListAll()

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *payloadsPersistenceSuite) TestTrackOkay(c *gc.C) {
	id := "payload#a-unit/0#payloadA"
	pl := s.NewPayload("docker", "payloadA/payloadA-xyz")

	wp := s.NewPersistence()
	stID := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	err := wp.Track("a-unit/0", stID, pl)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All", "All", "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "payloads",
			Id:     id,
			Assert: txn.DocMissing,
			Insert: &persistence.PayloadDoc{
				DocID:     id,
				UnitID:    "a-unit/0",
				Name:      "payloadA",
				MachineID: "0",
				StateID:   stID,
				Type:      "docker",
				RawID:     "payloadA-xyz",
				State:     "running",
			},
		},
	}})
}

func (s *payloadsPersistenceSuite) TestTrackRetryOkay(c *gc.C) {
	id := "payload#a-unit/0#payloadA"
	pl := s.NewPayload("docker", "payloadA/payloadA-xyz")
	s.Stub.SetErrors(nil, nil, txn.ErrAborted)

	wp := s.NewPersistence()
	stID := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	err := wp.Track("a-unit/0", stID, pl)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All", "All", "Run", "All", "All")
	s.State.CheckOps(c, [][]txn.Op{
		{{
			C:      "payloads",
			Id:     id,
			Assert: txn.DocMissing,
			Insert: &persistence.PayloadDoc{
				DocID:     id,
				UnitID:    "a-unit/0",
				Name:      "payloadA",
				MachineID: "0",
				StateID:   stID,
				Type:      "docker",
				RawID:     "payloadA-xyz",
				State:     "running",
			},
		}}, {{
			C:      "payloads",
			Id:     id,
			Assert: txn.DocMissing,
			Insert: &persistence.PayloadDoc{
				DocID:     id,
				UnitID:    "a-unit/0",
				Name:      "payloadA",
				MachineID: "0",
				StateID:   stID,
				Type:      "docker",
				RawID:     "payloadA-xyz",
				State:     "running",
			},
		}},
	})
}

func (s *payloadsPersistenceSuite) TestTrackIDAlreadyExists(c *gc.C) {
	stID := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	pl := s.NewPayload("docker", "payloadA/payloadA-xyz")
	s.SetDoc(stID, pl)

	wp := s.NewPersistence()
	err := wp.Track("a-unit/0", stID, pl)

	s.Stub.CheckCallNames(c, "All")
	c.Check(errors.Cause(err), gc.Equals, payload.ErrAlreadyExists)
}

func (s *payloadsPersistenceSuite) TestTrackNameAlreadyExists(c *gc.C) {
	stID := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	pl := s.NewPayload("docker", "payloadA/payloadA-xyz")
	s.SetDoc("not-"+stID, pl)

	wp := s.NewPersistence()
	err := wp.Track("a-unit/0", stID, pl)

	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
	s.Stub.CheckCallNames(c, "All", "All")
	s.State.CheckNoOps(c)
}

func (s *payloadsPersistenceSuite) TestTrackLookupFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)
	pl := s.NewPayload("docker", "payloadA")

	pp := s.NewPersistence()
	err := pp.Track("a-unit/0", id, pl)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.Stub.CheckCallNames(c, "All")
}

func (s *payloadsPersistenceSuite) TestTrackInsertFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(nil, nil, failure)
	pl := s.NewPayload("docker", "payloadA")

	pp := s.NewPersistence()
	err := pp.Track("a-unit/0", id, pl)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.Stub.CheckCallNames(c, "All", "All", "Run")
}

func (s *payloadsPersistenceSuite) TestSetStatusOkay(c *gc.C) {
	id := "payload#a-unit/0#payloadA"
	stID := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.NewPayload("docker", "payloadA/payloadA-xyz")
	s.SetDoc(stID, pl)

	pp := s.NewPersistence()
	err := pp.SetStatus("a-unit/0", stID, payload.StateRunning)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All", "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "payloads",
			Id:     id,
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{
					{"state", payload.StateRunning},
				}},
			},
		}, {
			C:      "payloads",
			Id:     id,
			Assert: bson.D{{"state-id", stID}},
		},
	}})
}

func (s *payloadsPersistenceSuite) TestSetStatusMissing(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"

	pp := s.NewPersistence()
	err := pp.SetStatus("a-unit/0", id, payload.StateRunning)

	c.Check(errors.Cause(err), gc.Equals, payload.ErrNotFound)
	s.Stub.CheckCallNames(c, "All")
	s.State.CheckOps(c, nil)
}

func (s *payloadsPersistenceSuite) TestSetStatusFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.NewPayload("docker", "payloadA/payloadA-xyz")
	s.SetDoc(id, pl)
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	err := pp.SetStatus("a-unit/0", id, payload.StateRunning)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.State.CheckOps(c, nil)
}

func (s *payloadsPersistenceSuite) TestListOkay(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.NewPayload("docker", "payloadA/xyz")
	s.SetDoc(id, pl)
	other := s.NewPayload("docker", "payloadB/abc")
	s.SetDoc("f47ac10b-58cc-4372-a567-0e02b2c3d480", other)

	pp := s.NewPersistence()
	payloads, missing, err := pp.List("a-unit/0", id)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(payloads, jc.DeepEquals, []payload.FullPayloadInfo{pl})
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
	payloads, missing, err := pp.List("a-unit/0", id, missingID)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(payloads, jc.DeepEquals, []payload.FullPayloadInfo{pl})
	c.Check(missing, jc.DeepEquals, []string{missingID})
}

func (s *payloadsPersistenceSuite) TestListEmpty(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pp := s.NewPersistence()
	payloads, missing, err := pp.List("a-unit/0", id)
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
	_, _, err := pp.List("a-unit/0")

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *payloadsPersistenceSuite) TestListAllForUnitOkay(c *gc.C) {
	existing := s.NewPayloads("docker", "payloadA/xyz", "payloadB/abc")
	for i, pl := range existing {
		s.SetDoc(fmt.Sprintf("%d", i), pl)
	}

	pp := s.NewPersistence()
	payloads, err := pp.ListAllForUnit("a-unit/0")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	sort.Sort(byName(payloads))
	sort.Sort(byName(existing))
	c.Check(payloads, jc.DeepEquals, existing)
}

func (s *payloadsPersistenceSuite) TestListAllForUnitEmpty(c *gc.C) {
	pp := s.NewPersistence()
	payloads, err := pp.ListAllForUnit("a-unit/0")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	s.State.CheckNoOps(c)
	c.Check(payloads, gc.HasLen, 0)
}

type byName []payload.FullPayloadInfo

func (b byName) Len() int           { return len(b) }
func (b byName) Less(i, j int) bool { return b[i].FullID() < b[j].FullID() }
func (b byName) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

func (s *payloadsPersistenceSuite) TestListAllForUnitFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, err := pp.ListAllForUnit("a-unit/0")

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *payloadsPersistenceSuite) TestUntrackOkay(c *gc.C) {
	id := "payload#a-unit/0#payloadA"
	stID := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.NewPayload("docker", "payloadA/xyz")
	s.SetDoc(stID, pl)

	pp := s.NewPersistence()
	err := pp.Untrack("a-unit/0", stID)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All", "Run")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "payloads",
			Id:     id,
			Assert: txn.DocExists,
			Remove: true,
		}, {
			C:      "payloads",
			Id:     id,
			Assert: bson.D{{"state-id", stID}},
		},
	}})
}

func (s *payloadsPersistenceSuite) TestUntrackCheckFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	err := pp.Untrack("a-unit/0", id)

	s.Stub.CheckCallNames(c, "All")
	c.Check(errors.Cause(err), gc.Equals, failure)
	s.State.CheckOps(c, nil)
}

func (s *payloadsPersistenceSuite) TestUntrackRetryFailed(c *gc.C) {
	id := "payload#a-unit/0#payloadA"
	stID := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := s.NewPayload("docker", "payloadA/xyz")
	s.SetDoc(stID, pl)
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(nil, txn.ErrAborted, failure)

	pp := s.NewPersistence()
	err := pp.Untrack("a-unit/0", stID)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.Stub.CheckCallNames(c, "All", "Run", "All")
	s.State.CheckOps(c, [][]txn.Op{{
		{
			C:      "payloads",
			Id:     id,
			Assert: txn.DocExists,
			Remove: true,
		}, {
			C:      "payloads",
			Id:     id,
			Assert: bson.D{{"state-id", stID}},
		},
	}})
}

func checkPayloads(c *gc.C, payloads []payload.FullPayloadInfo, expectedList ...payload.FullPayloadInfo) {
	remainder := make([]payload.FullPayloadInfo, len(payloads))
	copy(remainder, payloads)
	var noMatch []payload.FullPayloadInfo
	for _, expected := range expectedList {
		found := false
		for i, payload := range remainder {
			if reflect.DeepEqual(payload, expected) {
				remainder = append(remainder[:i], remainder[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			noMatch = append(noMatch, expected)
		}
	}

	ok1 := c.Check(noMatch, gc.HasLen, 0)
	ok2 := c.Check(remainder, gc.HasLen, 0)
	if !ok1 || !ok2 {
		c.Logf("<<<<<<<<\nexpected:")
		for _, payload := range expectedList {
			c.Logf("%#v", payload)
		}
		c.Logf("--------\ngot:")
		for _, payload := range payloads {
			c.Logf("%#v", payload)
		}
		c.Logf(">>>>>>>>")
	}
}
