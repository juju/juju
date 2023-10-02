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

	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/database"
)

type stateSuite struct {
	schematesting.ControllerSuite

	st *State

	upgradeUUID string
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.st = NewState(s.TxnRunnerFactory())

	// Add a completed upgrade before tests start
	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("2.9.42"), version.MustParse("3.0.0"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.StartUpgrade(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerDone(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)

	s.upgradeUUID = uuid
}

func (s *stateSuite) getUpgrade(c *gc.C, st *State, upgradeUUID string) (Info, []infoControllerNode) {
	db, err := s.st.DB()
	c.Assert(err, jc.ErrorIsNil)

	infoQ := `
SELECT * AS &Info.* FROM upgrade_info
WHERE uuid = $M.info_uuid`
	infoS, err := sqlair.Prepare(infoQ, Info{}, sqlair.M{})
	c.Assert(err, jc.ErrorIsNil)

	nodeInfosQ := `
SELECT * AS &infoControllerNode.* FROM upgrade_info_controller_node
WHERE upgrade_info_uuid = $M.info_uuid`
	nodeInfosS, err := sqlair.Prepare(nodeInfosQ, infoControllerNode{}, sqlair.M{})
	c.Assert(err, jc.ErrorIsNil)

	var (
		info      Info
		nodeInfos []infoControllerNode
	)
	err = db.Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, infoS, sqlair.M{"info_uuid": upgradeUUID}).Get(&info)
		c.Assert(err, jc.ErrorIsNil)
		err = tx.Query(ctx, nodeInfosS, sqlair.M{"info_uuid": upgradeUUID}).GetAll(&nodeInfos)
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return info, nodeInfos
}

func (s *stateSuite) TestCreateUpgrade(c *gc.C) {
	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	upgradeInfo, nodeInfos := s.getUpgrade(c, s.st, uuid)
	c.Check(upgradeInfo, gc.Equals, Info{
		UUID:            uuid,
		PreviousVersion: "3.0.0",
		TargetVersion:   "3.0.1",
		CreatedAt:       upgradeInfo.CreatedAt,
	})
	c.Check(nodeInfos, gc.HasLen, 0)
}

func (s *stateSuite) TestCreateUpgradeAlreadyExists(c *gc.C) {
	_, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.st.CreateUpgrade(context.Background(), version.MustParse("4.0.0"), version.MustParse("4.0.1"))
	c.Assert(database.IsErrConstraintUnique(err), jc.IsTrue)
}

func (s *stateSuite) TestSetControllerReady(c *gc.C) {
	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)

	_, nodeInfos := s.getUpgrade(c, s.st, uuid)
	c.Check(nodeInfos, gc.HasLen, 1)
	c.Check(nodeInfos[0], gc.Equals, infoControllerNode{
		ControllerNodeID: "0",
	})
}

func (s *stateSuite) TestSetControllerReadyWithoutUpgrade(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	err = s.st.SetControllerReady(context.Background(), uuid.String(), "0")
	c.Assert(err, gc.NotNil)
	c.Check(database.IsErrConstraintForeignKey(err), jc.IsTrue)
}

func (s *stateSuite) TestSetControllerReadyMultipleTimes(c *gc.C) {
	err := s.st.SetControllerReady(context.Background(), s.upgradeUUID, "0")
	c.Assert(err, gc.NotNil)
	c.Check(database.IsErrConstraintUnique(err), jc.IsTrue)
}

func (s *stateSuite) TestAllProvisionedControllerReadyTrue(c *gc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1)")
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	allProvisioned, err := s.st.AllProvisionedControllersReady(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(allProvisioned, jc.IsTrue)
}

func (s *stateSuite) TestAllProvisionedControllerReadyFalse(c *gc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1), ('2', 2)")
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	allProvisioned, err := s.st.AllProvisionedControllersReady(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(allProvisioned, jc.IsFalse)
}

func (s *stateSuite) TestAllProvisionedControllerReadyUnprovisionedController(c *gc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1)")
	c.Assert(err, jc.ErrorIsNil)
	_, err = db.Exec("INSERT INTO controller_node (controller_id) VALUES ('2')")
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	allProvisioned, err := s.st.AllProvisionedControllersReady(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(allProvisioned, jc.IsTrue)
}

func (s *stateSuite) TestStartUpgrade(c *gc.C) {
	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)

	err = s.st.StartUpgrade(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	info, _ := s.getUpgrade(c, s.st, uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(info.StartedAt.Valid, jc.IsTrue)
}

func (s *stateSuite) TestStartUpgradeIdempotent(c *gc.C) {
	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)

	err = s.st.StartUpgrade(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	info, _ := s.getUpgrade(c, s.st, uuid)
	c.Assert(info.StartedAt.Valid, jc.IsTrue)
	startedAt := info.StartedAt.String

	err = s.st.StartUpgrade(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)

	info, _ = s.getUpgrade(c, s.st, uuid)
	c.Check(info.StartedAt.String, gc.Equals, startedAt)
}

func (s *stateSuite) TestStartUpgradeBeforeCreated(c *gc.C) {
	uuid := utils.MustNewUUID().String()
	err := s.st.StartUpgrade(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
}

func (s *stateSuite) TestSetControllerDone(c *gc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1), ('2', 2)")
	c.Assert(err, jc.ErrorIsNil)
	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	err = s.st.SetControllerDone(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	_, nodeInfos := s.getUpgrade(c, s.st, uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(nodeInfos, gc.HasLen, 2)
	c.Check(nodeInfos[0], gc.Equals, infoControllerNode{ControllerNodeID: "0"})
	c.Check(nodeInfos[1], gc.Equals, infoControllerNode{ControllerNodeID: "1", NodeUpgradeCompletedAt: nodeInfos[1].NodeUpgradeCompletedAt})
	c.Check(nodeInfos[1].NodeUpgradeCompletedAt.Valid, jc.IsTrue)
}

func (s *stateSuite) TestSetControllerDoneNotExists(c *gc.C) {
	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	err = s.st.SetControllerDone(context.Background(), uuid, "0")
	c.Assert(err, gc.ErrorMatches, `controller node "0" not ready`)
}

func (s *stateSuite) TestSetControllerDoneCompleteUpgrade(c *gc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1)")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.st.ActiveUpgrade(context.Background())
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)

	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	err = s.st.SetControllerDone(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)

	activeUpgrade, err := s.st.ActiveUpgrade(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(activeUpgrade, gc.Not(gc.Equals), "")

	err = s.st.SetControllerDone(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.st.ActiveUpgrade(context.Background())
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
}

func (s *stateSuite) TestSetControllerDoneCompleteUpgradeEmptyCompletedAt(c *gc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1)")
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "0")
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.SetControllerReady(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	_, err = db.Exec(`
UPDATE upgrade_info_controller_node 
SET    node_upgrade_completed_at = ''
WHERE  upgrade_info_uuid = ?
       AND controller_node_id = 0`, uuid)
	c.Assert(err, jc.ErrorIsNil)

	activeUpgrade, err := s.st.ActiveUpgrade(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activeUpgrade, gc.Not(gc.Equals), "")

	err = s.st.SetControllerDone(context.Background(), uuid, "1")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.st.ActiveUpgrade(context.Background())
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
}

func (s *stateSuite) TestActiveUpgradesNoUpgrades(c *gc.C) {
	_, err := s.st.ActiveUpgrade(context.Background())
	c.Assert(err, jc.ErrorIs, sql.ErrNoRows)
}

func (s *stateSuite) TestActiveUpgradesSingular(c *gc.C) {
	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	activeUpgrade, err := s.st.ActiveUpgrade(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(activeUpgrade, gc.Equals, uuid)
}

func (s *stateSuite) TestUpgrade(c *gc.C) {
	uuid, err := s.st.CreateUpgrade(context.Background(), version.MustParse("3.0.0"), version.MustParse("3.0.1"))
	c.Assert(err, jc.ErrorIsNil)

	upgradeInfo, err := s.st.Upgrade(context.Background(), uuid)
	c.Check(upgradeInfo, gc.Equals, Info{
		UUID:            uuid,
		PreviousVersion: "3.0.0",
		TargetVersion:   "3.0.1",
		CreatedAt:       upgradeInfo.CreatedAt,
	})
	c.Assert(err, jc.ErrorIsNil)
}
