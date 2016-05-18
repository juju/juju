// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/payload"
)

// These tests are a low-level sanity check in support of more complete
// integration testing done in state/payloads_test.go.

type PayloadsMongoSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&PayloadsMongoSuite{})

func (s *PayloadsMongoSuite) TestInsertOps(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	itxn := insertPayloadTxn{pl.Unit, pl}

	ops := itxn.ops()

	f.Stub.CheckNoCalls(c)
	id := "payload#a-unit/0#payloadA"
	c.Check(ops, jc.DeepEquals, []txn.Op{{
		C:      "payloads",
		Id:     id,
		Assert: txn.DocMissing,
		Insert: &payloadDoc{
			DocID:     id,
			UnitID:    "a-unit/0",
			Name:      "payloadA",
			MachineID: "0",
			Type:      "docker",
			RawID:     "payloadA-xyz",
			State:     "running",
		},
	}})
}

func (s *PayloadsMongoSuite) TestInsertCheckAssertsMissing(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	itxn := insertPayloadTxn{pl.Unit, pl}

	err := itxn.checkAsserts(f.Queries)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
}

func (s *PayloadsMongoSuite) TestInsertCheckAssertsAlreadyExists(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	f.SetDocs(pl)
	itxn := insertPayloadTxn{pl.Unit, pl}

	err := itxn.checkAsserts(f.Queries)

	f.Stub.CheckCallNames(c, "All")
	c.Check(errors.Cause(err), gc.Equals, payload.ErrAlreadyExists)
}

func (s *PayloadsMongoSuite) TestSetStatusOps(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	stxn := setPayloadStatusTxn{pl.Unit, pl.Name, payload.StateRunning}

	ops := stxn.ops()

	f.Stub.CheckNoCalls(c)
	id := "payload#a-unit/0#payloadA"
	c.Check(ops, jc.DeepEquals, []txn.Op{{
		C:      "payloads",
		Id:     id,
		Assert: txn.DocExists,
		Update: bson.D{
			{"$set", bson.D{
				{"state", payload.StateRunning},
			}},
		},
	}})
}

func (s *PayloadsMongoSuite) TestSetStatusCheckAssertsExists(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	f.SetDocs(pl)
	stxn := setPayloadStatusTxn{pl.Unit, pl.Name, payload.StateRunning}

	err := stxn.checkAsserts(f.Queries)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
}

func (s *PayloadsMongoSuite) TestSetStatusCheckAssertsMissing(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	stxn := setPayloadStatusTxn{"a-unit/0", "", payload.StateRunning}

	err := stxn.checkAsserts(f.Queries)

	f.Stub.CheckCallNames(c, "All")
	c.Check(errors.Cause(err), gc.Equals, payload.ErrNotFound)
}

func (s *PayloadsMongoSuite) TestRemoveOps(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/xyz")
	rtxn := removePayloadTxn{pl.Unit, pl.Name}

	ops := rtxn.ops()

	f.Stub.CheckNoCalls(c)
	id := "payload#a-unit/0#payloadA"
	c.Check(ops, jc.DeepEquals, []txn.Op{{
		C:      "payloads",
		Id:     id,
		Assert: txn.DocExists,
		Remove: true,
	}})
}

func (s *PayloadsMongoSuite) TestRemoveCheckAssertsExists(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/xyz")
	f.SetDocs(pl)
	rtxn := removePayloadTxn{pl.Unit, pl.Name}

	err := rtxn.checkAsserts(f.Queries)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
}

func (s *PayloadsMongoSuite) TestRemoveCheckAssertsMissing(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	rtxn := removePayloadTxn{"a-unit/0", "payloadA"}

	err := rtxn.checkAsserts(f.Queries)

	f.Stub.CheckCallNames(c, "All")
	c.Check(errors.Cause(err), gc.Equals, jujutxn.ErrNoOperations)
}
