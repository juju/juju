// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence_test

import (
	"reflect"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/persistence"
)

var _ = gc.Suite(&PayloadsPersistenceSuite{})

type PayloadsPersistenceSuite struct {
	testing.IsolationSuite

	Stub *testing.Stub
	db   *persistence.StubPersistenceBase
}

func (s *PayloadsPersistenceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.Stub = &testing.Stub{}
	s.db = &persistence.StubPersistenceBase{Stub: s.Stub}
}

func (s *PayloadsPersistenceSuite) NewPersistence() *persistence.Persistence {
	return persistence.NewPersistence(s.db)
}

func (s *PayloadsPersistenceSuite) TestListAllOkay(c *gc.C) {
	p1 := newPayload("0", "a-unit/0", "docker", "spam/spam-xyz")
	p1.Labels = []string{"a-tag"}
	p2 := newPayload("0", "a-unit/0", "docker", "eggs/eggs-xyz")
	p2.Labels = []string{"a-tag"}
	s.db.SetDocs(p1, p2)
	persist := s.NewPersistence()

	payloads, err := persist.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	checkPayloads(c, payloads, p1, p2)
	s.Stub.CheckCallNames(c, "All")
}

func (s *PayloadsPersistenceSuite) TestListAllEmpty(c *gc.C) {
	persist := s.NewPersistence()

	payloads, err := persist.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(payloads, gc.HasLen, 0)
	s.Stub.CheckCallNames(c, "All")
}

func (s *PayloadsPersistenceSuite) TestListAllFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)
	persist := s.NewPersistence()

	_, err := persist.ListAll()

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *PayloadsPersistenceSuite) TestTrackOkay(c *gc.C) {
	stID := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := newPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	wp := s.NewPersistence()

	err := wp.Track("a-unit/0", stID, pl)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "Run")
}

func (s *PayloadsPersistenceSuite) TestTrackFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)
	pl := newPayload("0", "a-unit/0", "docker", "payloadA")
	pp := s.NewPersistence()

	err := pp.Track("a-unit/0", id, pl)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.Stub.CheckCallNames(c, "Run")
}

func (s *PayloadsPersistenceSuite) TestSetStatusOkay(c *gc.C) {
	stID := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := newPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	s.db.SetDoc(stID, pl)
	pp := s.NewPersistence()

	err := pp.SetStatus("a-unit/0", stID, payload.StateRunning)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "Run")
}

func (s *PayloadsPersistenceSuite) TestSetStatusFailed(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := newPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	s.db.SetDoc(id, pl)
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	err := pp.SetStatus("a-unit/0", id, payload.StateRunning)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.Stub.CheckCallNames(c, "Run")
}

func (s *PayloadsPersistenceSuite) TestListOkay(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := newPayload("0", "a-unit/0", "docker", "payloadA/xyz")
	s.db.SetDoc(id, pl)
	other := newPayload("0", "a-unit/0", "docker", "payloadB/abc")
	s.db.AddDoc("f47ac10b-58cc-4372-a567-0e02b2c3d480", other)

	pp := s.NewPersistence()
	payloads, missing, err := pp.List("a-unit/0", id)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	c.Check(payloads, jc.DeepEquals, []payload.FullPayloadInfo{pl})
	c.Check(missing, gc.HasLen, 0)
}

func (s *PayloadsPersistenceSuite) TestListSomeMissing(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := newPayload("0", "a-unit/0", "docker", "payloadB/abc")
	s.db.SetDoc(id, pl)
	other := newPayload("0", "a-unit/0", "docker", "payloadA/xyz")
	s.db.AddDoc("f47ac10b-58cc-4372-a567-0e02b2c3d480", other)

	missingID := "f47ac10b-58cc-4372-a567-0e02b2c3d481"
	pp := s.NewPersistence()
	payloads, missing, err := pp.List("a-unit/0", id, missingID)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	c.Check(payloads, jc.DeepEquals, []payload.FullPayloadInfo{pl})
	c.Check(missing, jc.DeepEquals, []string{missingID})
}

func (s *PayloadsPersistenceSuite) TestListEmpty(c *gc.C) {
	id := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pp := s.NewPersistence()
	payloads, missing, err := pp.List("a-unit/0", id)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	c.Check(payloads, gc.HasLen, 0)
	c.Check(missing, jc.DeepEquals, []string{id})
}

func (s *PayloadsPersistenceSuite) TestListFailure(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, _, err := pp.List("a-unit/0")

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *PayloadsPersistenceSuite) TestListAllForUnitOkay(c *gc.C) {
	existing := newPayloads("0", "a-unit/0", "docker", "payloadA/xyz", "payloadB/abc")
	s.db.SetDocs(existing...)

	pp := s.NewPersistence()
	payloads, err := pp.ListAllForUnit("a-unit/0")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	sort.Sort(byName(payloads))
	sort.Sort(byName(existing))
	c.Check(payloads, jc.DeepEquals, existing)
}

func (s *PayloadsPersistenceSuite) TestListAllForUnitEmpty(c *gc.C) {
	pp := s.NewPersistence()
	payloads, err := pp.ListAllForUnit("a-unit/0")
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "All")
	c.Check(payloads, gc.HasLen, 0)
}

type byName []payload.FullPayloadInfo

func (b byName) Len() int           { return len(b) }
func (b byName) Less(i, j int) bool { return b[i].FullID() < b[j].FullID() }
func (b byName) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

func (s *PayloadsPersistenceSuite) TestListAllForUnitFailed(c *gc.C) {
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	_, err := pp.ListAllForUnit("a-unit/0")

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *PayloadsPersistenceSuite) TestUntrackOkay(c *gc.C) {
	stID := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := newPayload("0", "a-unit/0", "docker", "payloadA/xyz")
	s.db.SetDoc(stID, pl)

	pp := s.NewPersistence()
	err := pp.Untrack("a-unit/0", stID)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "Run")
}

func (s *PayloadsPersistenceSuite) TestUntrackFailed(c *gc.C) {
	stID := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	pl := newPayload("0", "a-unit/0", "docker", "payloadA/xyz")
	s.db.SetDoc(stID, pl)
	failure := errors.Errorf("<failed!>")
	s.Stub.SetErrors(failure)

	pp := s.NewPersistence()
	err := pp.Untrack("a-unit/0", stID)

	c.Check(errors.Cause(err), gc.Equals, failure)
	s.Stub.CheckCallNames(c, "Run")
}

func newPayloads(machine, unit, pType string, ids ...string) []payload.FullPayloadInfo {
	var payloads []payload.FullPayloadInfo
	for _, id := range ids {
		pl := newPayload(machine, unit, pType, id)
		payloads = append(payloads, pl)
	}
	return payloads
}

var newPayload = persistence.NewPayload

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
