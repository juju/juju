// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
	persistence "github.com/juju/juju/state"
)

type PayloadsPersistenceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&PayloadsPersistenceSuite{})

func (s *PayloadsPersistenceSuite) TestListAllOkay(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	p1 := f.NewPayload("0", "a-unit/0", "docker", "spam/spam-xyz")
	p1.Labels = []string{"a-tag"}
	p2 := f.NewPayload("0", "a-unit/0", "docker", "eggs/eggs-xyz")
	p2.Labels = []string{"a-tag"}
	f.SetDocs(p1, p2)
	persist := f.NewPersistence()

	payloads, err := persist.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	f.CheckPayloads(c, payloads, p1, p2)
	f.Stub.CheckCallNames(c, "All")
}

func (s *PayloadsPersistenceSuite) TestListAllEmpty(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	persist := f.NewPersistence()

	payloads, err := persist.ListAll()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(payloads, gc.HasLen, 0)
	f.Stub.CheckCallNames(c, "All")
}

func (s *PayloadsPersistenceSuite) TestListAllFailed(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	failure := errors.Errorf("<failed!>")
	f.Stub.SetErrors(failure)
	persist := f.NewPersistence()

	_, err := persist.ListAll()

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *PayloadsPersistenceSuite) TestTrackOkay(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	wp := f.NewPersistence()

	err := wp.Track(pl)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "Run")
}

func (s *PayloadsPersistenceSuite) TestTrackFailed(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	failure := errors.Errorf("<failed!>")
	f.Stub.SetErrors(failure)
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA")
	pp := f.NewPersistence()

	err := pp.Track(pl)

	c.Check(errors.Cause(err), gc.Equals, failure)
	f.Stub.CheckCallNames(c, "Run")
}

func (s *PayloadsPersistenceSuite) TestSetStatusOkay(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	f.SetDocs(pl)
	pp := f.NewPersistence()

	err := pp.SetStatus(pl.Unit, pl.Name, payload.StateRunning)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "Run")
}

func (s *PayloadsPersistenceSuite) TestSetStatusFailed(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	f.SetDocs(pl)
	failure := errors.Errorf("<failed!>")
	f.Stub.SetErrors(failure)
	pp := f.NewPersistence()

	err := pp.SetStatus(pl.Unit, pl.Name, payload.StateRunning)

	c.Check(errors.Cause(err), gc.Equals, failure)
	f.Stub.CheckCallNames(c, "Run")
}

func (s *PayloadsPersistenceSuite) TestListOkay(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/xyz")
	other := f.NewPayload("0", "a-unit/0", "docker", "payloadB/abc")
	f.SetDocs(pl, other)
	pp := f.NewPersistence()

	payloads, missing, err := pp.List(pl.Unit, pl.Name)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
	c.Check(payloads, jc.DeepEquals, []payload.FullPayloadInfo{pl})
	c.Check(missing, gc.HasLen, 0)
}

func (s *PayloadsPersistenceSuite) TestListSomeMissing(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadB/abc")
	other := f.NewPayload("0", "a-unit/0", "docker", "payloadA/xyz")
	f.SetDocs(pl, other)
	missingName := "not-" + pl.Name
	pp := f.NewPersistence()

	payloads, missing, err := pp.List(pl.Unit, pl.Name, missingName)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
	c.Check(payloads, jc.DeepEquals, []payload.FullPayloadInfo{pl})
	c.Check(missing, jc.DeepEquals, []string{missingName})
}

func (s *PayloadsPersistenceSuite) TestListEmpty(c *gc.C) {
	name := "payloadA"
	f := persistence.NewPayloadPersistenceFixture()
	pp := f.NewPersistence()

	payloads, missing, err := pp.List("a-unit/0", name)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
	c.Check(payloads, gc.HasLen, 0)
	c.Check(missing, jc.DeepEquals, []string{name})
}

func (s *PayloadsPersistenceSuite) TestListFailure(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	failure := errors.Errorf("<failed!>")
	f.Stub.SetErrors(failure)
	pp := f.NewPersistence()

	_, _, err := pp.List("a-unit/0")

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *PayloadsPersistenceSuite) TestListAllForUnitOkay(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	existing := f.NewPayloads("0", "a-unit/0", "docker", "payloadA/xyz", "payloadB/abc")
	f.SetDocs(existing...)
	pp := f.NewPersistence()

	payloads, err := pp.ListAllForUnit("a-unit/0")
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
	sort.Sort(byName(payloads))
	sort.Sort(byName(existing))
	c.Check(payloads, jc.DeepEquals, existing)
}

func (s *PayloadsPersistenceSuite) TestListAllForUnitEmpty(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	pp := f.NewPersistence()

	payloads, err := pp.ListAllForUnit("a-unit/0")
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
	c.Check(payloads, gc.HasLen, 0)
}

type byName []payload.FullPayloadInfo

func (b byName) Len() int           { return len(b) }
func (b byName) Less(i, j int) bool { return b[i].FullID() < b[j].FullID() }
func (b byName) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

func (s *PayloadsPersistenceSuite) TestListAllForUnitFailed(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	failure := errors.Errorf("<failed!>")
	f.Stub.SetErrors(failure)
	pp := f.NewPersistence()

	_, err := pp.ListAllForUnit("a-unit/0")

	c.Check(errors.Cause(err), gc.Equals, failure)
}

func (s *PayloadsPersistenceSuite) TestUntrackOkay(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/xyz")
	f.SetDocs(pl)
	pp := f.NewPersistence()

	err := pp.Untrack(pl.Unit, pl.Name)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "Run")
}

func (s *PayloadsPersistenceSuite) TestUntrackFailed(c *gc.C) {
	f := persistence.NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/xyz")
	f.SetDocs(pl)
	failure := errors.Errorf("<failed!>")
	f.Stub.SetErrors(failure)
	pp := f.NewPersistence()

	err := pp.Untrack(pl.Unit, pl.Name)

	c.Check(errors.Cause(err), gc.Equals, failure)
	f.Stub.CheckCallNames(c, "Run")
}
