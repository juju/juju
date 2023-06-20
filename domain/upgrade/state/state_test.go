// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/database"
	"github.com/juju/juju/database/testing"
)

type stateSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) getUpgrade(c *gc.C, st *State, upgradeUUID string) (info, []infoControllerNode) {
	db, err := st.DB()
	c.Assert(err, jc.ErrorIsNil)

	var (
		info      info
		nodeInfos []infoControllerNode
	)
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		info, err = st.getUpgrade(ctx, tx, upgradeUUID)
		c.Assert(err, jc.ErrorIsNil)
		nodeInfos, err = st.getUpgradeNodes(ctx, tx, upgradeUUID)
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return info, nodeInfos
}

func (s *stateSuite) TestCreateUpgrade(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	uuid, err := st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	upgradeInfo, nodeInfos := s.getUpgrade(c, st, uuid)
	c.Assert(upgradeInfo, gc.Equals, info{
		UUID:            uuid,
		PreviousVersion: "3.0.0",
		TargetVersion:   "3.0.1",
		CreatedAt:       upgradeInfo.CreatedAt,
	})
	c.Assert(nodeInfos, gc.HasLen, 0)
}

func (s *stateSuite) TestSetControllerReady(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	uuid, err := st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)

	_, nodeInfos := s.getUpgrade(c, st, uuid)
	c.Assert(nodeInfos, gc.HasLen, 1)
	c.Assert(nodeInfos[0], gc.Equals, infoControllerNode{
		ControllerNodeID: "0",
	})
}

func (s *stateSuite) TestSetControllerReadyWithoutUpgrade(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	uuid := utils.MustNewUUID().String()
	err := st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(database.IsErrConstraintForeignKey(err), jc.IsTrue)
}

func (s *stateSuite) TestAllProvisionedControllerReadyTrue(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1)")
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetControllerReady(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	allProvisioned, err := st.AllProvisionedControllersReady(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allProvisioned, jc.IsTrue)
}

func (s *stateSuite) TestAllProvisionedControllerReadyFalse(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1), ('2', 2)")
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetControllerReady(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	allProvisioned, err := st.AllProvisionedControllersReady(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allProvisioned, jc.IsTrue)
}

func (s *stateSuite) TestStartUpgrade(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	uuid, err := st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)

	err = st.StartUpgrade(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	info, _ := s.getUpgrade(c, st, uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.StartedAt.Valid, jc.IsTrue)
}

func (s *stateSuite) TestStartUpgradeIdempotent(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	uuid, err := st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)

	err = st.StartUpgrade(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	info, _ := s.getUpgrade(c, st, uuid)
	c.Assert(info.StartedAt.Valid, jc.IsTrue)
	startedAt := info.StartedAt.String

	err = st.StartUpgrade(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	info, _ = s.getUpgrade(c, st, uuid)
	c.Assert(info.StartedAt.String, gc.Equals, startedAt)
}

func (s *stateSuite) TestStartUpgradeBeforeCreated(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	uuid := utils.MustNewUUID().String()
	err := st.StartUpgrade(context.Background(), uuid)
	c.Assert(err, gc.ErrorMatches, sql.ErrNoRows.Error())
}

func (s *stateSuite) TestSetControllerDone(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1), ('2', 2)")
	c.Assert(err, jc.ErrorIsNil)
	uuid, err := st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetControllerReady(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	err = st.SetControllerDone(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	_, nodeInfos := s.getUpgrade(c, st, uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nodeInfos, gc.HasLen, 2)
	c.Assert(nodeInfos[0], gc.Equals, infoControllerNode{ControllerNodeID: "0"})
	c.Assert(nodeInfos[1], gc.Equals, infoControllerNode{ControllerNodeID: "1", NodeUpgradeCompletedAt: nodeInfos[1].NodeUpgradeCompletedAt})
	c.Assert(nodeInfos[1].NodeUpgradeCompletedAt.Valid, jc.IsTrue)
}

func (s *stateSuite) TestSetControllerDoneNotExists(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	uuid, err := st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	err = st.SetControllerDone(context.Background(), uuid, "0")
	c.Assert(err, gc.ErrorMatches, `controller node "0" not ready`)
}

func (s *stateSuite) TestSetControllerDoneCompleteUpgrade(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1)")
	c.Assert(err, jc.ErrorIsNil)

	activeUpgrades, err := st.ActiveUpgrades(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activeUpgrades, gc.HasLen, 0)

	uuid, err := st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetControllerReady(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	err = st.SetControllerDone(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)

	activeUpgrades, err = st.ActiveUpgrades(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activeUpgrades, gc.HasLen, 1)

	err = st.SetControllerDone(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	activeUpgrades, err = st.ActiveUpgrades(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activeUpgrades, gc.HasLen, 0)
}

func (s *stateSuite) TestActiveUpgradesNoUpgrades(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	activeUpgrades, err := st.ActiveUpgrades(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activeUpgrades, gc.HasLen, 0)
}

func (s *stateSuite) TestActiveUpgradesSingular(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	uuid, err := st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	activeUpgrades, err := st.ActiveUpgrades(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activeUpgrades, gc.DeepEquals, []string{uuid})
}

func (s *stateSuite) TestActiveUpgradesMultiple(c *gc.C) {
	st := NewState(testing.TxnRunnerFactory(s.TxnRunner()))

	_, err := st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.CreateUpgrade(context.Background(), version.MustParse("3.0.1"), version.MustParse("3.0.2"))
	c.Assert(err, jc.ErrorIsNil)

	activeUpgrades, err := st.ActiveUpgrades(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activeUpgrades, gc.HasLen, 2)
}
