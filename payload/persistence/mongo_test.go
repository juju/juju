// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
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

func (s *PayloadsMongoSuite) TestUpsertOpsMissing(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	utxn := upsertPayloadTxn{
		payload: pl,
		exists:  false,
	}

	ops := utxn.ops()

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

func (s *PayloadsMongoSuite) TestUpsertOpsExists(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	utxn := upsertPayloadTxn{
		payload: pl,
		exists:  true,
	}

	ops := utxn.ops()

	f.Stub.CheckNoCalls(c)
	id := "payload#a-unit/0#payloadA"
	c.Check(ops, jc.DeepEquals, []txn.Op{{
		C:      "payloads",
		Id:     id,
		Assert: txn.DocExists,
		Remove: true,
	}, {
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

func (s *PayloadsMongoSuite) TestUpsertCheckAssertsMissing(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	utxn := upsertPayloadTxn{
		payload: pl,
	}

	err := utxn.checkAssertsAndUpdate(f.Queries)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
	c.Check(utxn.exists, jc.IsFalse)
}

func (s *PayloadsMongoSuite) TestUpsertCheckAssertsWasFound(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	utxn := upsertPayloadTxn{
		payload: pl,
		exists:  true,
	}

	err := utxn.checkAssertsAndUpdate(f.Queries)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
	c.Check(utxn.exists, jc.IsFalse)
}

func (s *PayloadsMongoSuite) TestUpsertCheckAssertsAlreadyExists(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	f.SetDocs(pl)
	utxn := upsertPayloadTxn{
		payload: pl,
	}

	err := utxn.checkAssertsAndUpdate(f.Queries)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
	c.Check(utxn.exists, jc.IsTrue)
}

func (s *PayloadsMongoSuite) TestUpsertCheckAssertsWasMissing(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	pl := f.NewPayload("0", "a-unit/0", "docker", "payloadA/payloadA-xyz")
	f.SetDocs(pl)
	utxn := upsertPayloadTxn{
		payload: pl,
		exists:  false,
	}

	err := utxn.checkAssertsAndUpdate(f.Queries)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
	c.Check(utxn.exists, jc.IsTrue)
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

	err := stxn.checkAssertsAndUpdate(f.Queries)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
}

func (s *PayloadsMongoSuite) TestSetStatusCheckAssertsMissing(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	stxn := setPayloadStatusTxn{"a-unit/0", "", payload.StateRunning}

	err := stxn.checkAssertsAndUpdate(f.Queries)

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

	err := rtxn.checkAssertsAndUpdate(f.Queries)
	c.Assert(err, jc.ErrorIsNil)

	f.Stub.CheckCallNames(c, "All")
}

func (s *PayloadsMongoSuite) TestRemoveCheckAssertsMissing(c *gc.C) {
	f := NewPayloadPersistenceFixture()
	rtxn := removePayloadTxn{"a-unit/0", "payloadA"}

	err := rtxn.checkAssertsAndUpdate(f.Queries)

	f.Stub.CheckCallNames(c, "All")
	c.Check(errors.Cause(err), gc.Equals, payload.ErrNotFound)
}
