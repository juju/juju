// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

type MachineInternalSuite struct {
}

var _ = gc.Suite(&MachineInternalSuite{})

func (s *MachineInternalSuite) TestCreateUpgradePrepareLockTxnAssertsMachineAlive(c *gc.C) {
	arbitraryId := "1"
	arbitraryData := &upgradeSeriesLock{}
	for _, op := range createUpgradeSeriesPrepareLockTxnOps(arbitraryId, arbitraryData) {
		assertVal, ok := op.Assert.(bson.D)
		if op.C == machinesC && ok && assertVal.Map()["life"] == Alive {
			c.SucceedNow()
		}
	}
	c.Fatal("Transaction does not assert that machines are of status Alive")
}

func (s *MachineInternalSuite) TestCreateUpgradePrepareLockTxnAssertsDocDoesNOTExist(c *gc.C) {
	arbitraryId := "1"
	arbitraryData := &upgradeSeriesLock{}
	expectedOp := txn.Op{
		C:      machineUpgradeSeriesLocksC,
		Id:     arbitraryId,
		Assert: txn.DocMissing,
		Insert: arbitraryData,
	}
	assertConstainsOP(c, expectedOp, createUpgradeSeriesPrepareLockTxnOps(arbitraryId, arbitraryData))
}

func (s *MachineInternalSuite) TestRemoveUpgradePrepareLockTxnAssertsDocExists(c *gc.C) {
	arbitraryId := "1"
	expectedOp := txn.Op{
		C:      machineUpgradeSeriesLocksC,
		Id:     arbitraryId,
		Assert: txn.DocExists,
		Remove: true,
	}
	assertConstainsOP(c, expectedOp, removeUpgradeSeriesPrepareLockTxnOps(arbitraryId))
}

func assertConstainsOP(c *gc.C, expectedOp txn.Op, actualOps []txn.Op) {
	for _, actualOp := range actualOps {
		if actualOp == expectedOp {
			c.SucceedNow()
		}
	}
	c.Fatalf("expected %#v to contain %#v", actualOps, expectedOp)
	return
}
