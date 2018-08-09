// Copyright Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/model"
)

type MachineInternalSuite struct {
	testing.IsolationSuite
}

func (s *MachineInternalSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

var _ = gc.Suite(&MachineInternalSuite{})

func (s *MachineInternalSuite) TestCreateUpgradeLockTxnAssertsMachineAlive(c *gc.C) {
	arbitraryId := "1"
	arbitraryData := &upgradeSeriesLockDoc{}
	var found bool
	for _, op := range createUpgradeSeriesLockTxnOps(arbitraryId, arbitraryData) {
		assertVal, ok := op.Assert.(bson.D)
		if op.C == machinesC && ok && assertVal.Map()["life"] == Alive {
			found = true
			break
		}
	}
	c.Assert(found, jc.IsTrue, gc.Commentf("Transaction does not assert that machines are of status Alive"))
}

func (s *MachineInternalSuite) TestCreateUpgradeLockTxnAssertsDocDoesNOTExist(c *gc.C) {
	arbitraryId := "1"
	arbitraryData := &upgradeSeriesLockDoc{}
	expectedOp := txn.Op{
		C:      machineUpgradeSeriesLocksC,
		Id:     arbitraryId,
		Assert: txn.DocMissing,
		Insert: arbitraryData,
	}
	assertConstainsOP(c, expectedOp, createUpgradeSeriesLockTxnOps(arbitraryId, arbitraryData))
}

func (s *MachineInternalSuite) TestRemoveUpgradeLockTxnAssertsDocExists(c *gc.C) {
	arbitraryId := "1"
	expectedOp := txn.Op{
		C:      machineUpgradeSeriesLocksC,
		Id:     arbitraryId,
		Assert: txn.DocExists,
		Remove: true,
	}
	assertConstainsOP(c, expectedOp, removeUpgradeSeriesLockTxnOps(arbitraryId))
}

func (s *MachineInternalSuite) TestsetUpgradeSeriesTxnOpsBuildsCorrectUnitTransaction(c *gc.C) {
	arbitraryMachineID := "id"
	arbitraryUnitName := "application/0"
	arbitraryStatus := model.PrepareStarted
	arbitraryUpdateTime := bson.Now()
	expectedOp := txn.Op{
		C:  machineUpgradeSeriesLocksC,
		Id: arbitraryMachineID,
		Assert: bson.D{{"$and", []bson.D{
			{{"prepare-units", bson.D{{"$exists", true}}}},
			{{"prepare-units.0.id", "application/0"}},
			{{"prepare-units.0.status", bson.D{{"$ne", arbitraryStatus}}}}}}},
		Update: bson.D{
			{"$set", bson.D{{"prepare-units.0.status", arbitraryStatus}, {"prepare-units.0.timestamp", arbitraryUpdateTime}}}},
	}

	actualOps, err := setUpgradeSeriesTxnOps(arbitraryMachineID, arbitraryUnitName, 0, arbitraryStatus, arbitraryUpdateTime)
	c.Assert(err, jc.ErrorIsNil)
	expectedOpSt := fmt.Sprint(expectedOp.Update)
	actualOpSt := fmt.Sprint(actualOps[1].Update)
	c.Assert(actualOpSt, gc.Equals, expectedOpSt)
}

func (s *MachineInternalSuite) TestsetUpgradeSeriesTxnOpsShouldAssertAssignedMachineIsAlive(c *gc.C) {
	arbitraryMachineID := "id"
	arbitraryStatus := model.PrepareStarted
	arbitraryUnitName := "application/0"
	arbitraryUnitIndex := 0
	arbitraryUpdateTime := bson.Now()
	expectedOp := txn.Op{
		C:      machinesC,
		Id:     arbitraryMachineID,
		Assert: isAliveDoc,
	}

	actualOps, err := setUpgradeSeriesTxnOps(arbitraryMachineID, arbitraryUnitName, arbitraryUnitIndex, arbitraryStatus, arbitraryUpdateTime)
	c.Assert(err, jc.ErrorIsNil)
	expectedOpSt := fmt.Sprint(expectedOp)
	actualOpSt := fmt.Sprint(actualOps[0])
	c.Assert(actualOpSt, gc.Equals, expectedOpSt)
}

func assertConstainsOP(c *gc.C, expectedOp txn.Op, actualOps []txn.Op) {
	var found bool
	for _, actualOp := range actualOps {
		if actualOp == expectedOp {
			found = true
			break
		}
	}
	c.Assert(found, jc.IsTrue, gc.Commentf("expected %#v to contain %#v", actualOps, expectedOp))
}
