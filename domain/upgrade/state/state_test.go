// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"errors"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/upgrade"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainupgrade "github.com/juju/juju/domain/upgrade"
	upgradeerrors "github.com/juju/juju/domain/upgrade/errors"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerSuite

	st *State

	upgradeUUID domainupgrade.UUID
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.st = NewState(s.TxnRunnerFactory())

	// Add a completed upgrade before tests start
	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("2.9.42"), semversion.MustParse("3.0.0"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.StartUpgrade(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetDBUpgradeCompleted(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerDone(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)

	s.upgradeUUID = uuid
}

func (s *stateSuite) TestEnsureUpgradeTypesMatchCore(c *tc.C) {
	db := s.DB()

	// This locks in the behaviour that the upgrade types in the database
	// should match the upgrade types in the core upgrade package.

	rows, err := db.Query(`SELECT id, type FROM upgrade_state_type`)
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	received := make(map[upgrade.State]string)

	// Ensure all the upgrade types that are in the database are also in the
	// core upgrade package.
	for rows.Next() {
		var (
			id   int
			name string
		)
		err = rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)

		c.Check(upgrade.State(id).String(), tc.Equals, name)

		// Ensure that we don't have any entries that are not parsable.
		state, err := upgrade.ParseState(name)
		c.Assert(err, tc.ErrorIsNil)

		received[state] = name
	}

	c.Assert(rows.Err(), tc.ErrorIsNil)

	// Ensure all the upgrade types in the core upgrade package are also in the
	// database.
	for state, name := range upgrade.States {
		r, ok := received[state]
		c.Check(ok, tc.IsTrue)
		c.Check(r, tc.Equals, name)
	}
}

func (s *stateSuite) TestCreateUpgrade(c *tc.C) {
	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	upgradeInfo, err := s.st.UpgradeInfo(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(upgradeInfo, tc.DeepEquals, upgrade.Info{
		UUID:            uuid.String(),
		PreviousVersion: "3.0.0",
		TargetVersion:   "3.0.1",
		State:           upgrade.Created,
	})

	nodeInfos := s.getUpgrade(c, s.st, uuid)
	c.Check(nodeInfos, tc.HasLen, 0)
}

func (s *stateSuite) TestCreateUpgradeAlreadyExists(c *tc.C) {
	_, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.st.CreateUpgrade(c.Context(), semversion.MustParse("4.0.0"), semversion.MustParse("4.0.1"))
	c.Assert(err, tc.ErrorIs, upgradeerrors.AlreadyExists)
}

func (s *stateSuite) TestSetControllerReady(c *tc.C) {
	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)

	nodeInfos := s.getUpgrade(c, s.st, uuid)
	c.Check(nodeInfos, tc.HasLen, 1)
	c.Check(nodeInfos[0], tc.Equals, ControllerNodeInfo{
		ControllerNodeID: "0",
	})
}

func (s *stateSuite) TestSetControllerReadyWithoutUpgrade(c *tc.C) {
	uuid := uuid.MustNewUUID().String()

	err := s.st.SetControllerReady(c.Context(), domainupgrade.UUID(uuid), "0")
	c.Check(err, tc.ErrorIs, upgradeerrors.NotFound)
}

// Setting the controller ready multiple times should not cause an error.
func (s *stateSuite) TestSetControllerReadyMultipleTimes(c *tc.C) {
	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Check(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Check(err, tc.ErrorIsNil)
}

func (s *stateSuite) TestAllProvisionedControllersReadyTrue(c *tc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1)")
	c.Assert(err, tc.ErrorIsNil)

	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "1")
	c.Assert(err, tc.ErrorIsNil)

	allProvisioned, err := s.st.AllProvisionedControllersReady(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(allProvisioned, tc.IsTrue)
}

func (s *stateSuite) TestAllProvisionedControllersReadyFalse(c *tc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1), ('2', 2)")
	c.Assert(err, tc.ErrorIsNil)

	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "1")
	c.Assert(err, tc.ErrorIsNil)

	allProvisioned, err := s.st.AllProvisionedControllersReady(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(allProvisioned, tc.IsFalse)
}

func (s *stateSuite) TestAllProvisionedControllersReadyMultipleControllers(c *tc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1)")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('2', 2)")
	c.Assert(err, tc.ErrorIsNil)

	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "1")
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "2")
	c.Assert(err, tc.ErrorIsNil)

	allProvisioned, err := s.st.AllProvisionedControllersReady(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(allProvisioned, tc.IsTrue)
}

func (s *stateSuite) TestAllProvisionedControllersReadyMultipleControllersWithoutAllBeingReady(c *tc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1)")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('2', 2)")
	c.Assert(err, tc.ErrorIsNil)

	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "1")
	c.Assert(err, tc.ErrorIsNil)

	allProvisioned, err := s.st.AllProvisionedControllersReady(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(allProvisioned, tc.IsFalse)
}

func (s *stateSuite) TestAllProvisionedControllersReadyUnprovisionedController(c *tc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1)")
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.Exec("INSERT INTO controller_node (controller_id) VALUES ('2')")
	c.Assert(err, tc.ErrorIsNil)

	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "1")
	c.Assert(err, tc.ErrorIsNil)

	allProvisioned, err := s.st.AllProvisionedControllersReady(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(allProvisioned, tc.IsTrue)
}

func (s *stateSuite) TestStartUpgrade(c *tc.C) {
	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)

	err = s.st.StartUpgrade(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)

	s.ensureUpgradeInfoState(c, uuid, upgrade.Started)
}

func (s *stateSuite) TestStartUpgradeCalledMultipleTimes(c *tc.C) {
	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)

	err = s.st.StartUpgrade(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)

	s.ensureUpgradeInfoState(c, uuid, upgrade.Started)

	err = s.st.StartUpgrade(c.Context(), uuid)
	c.Assert(err, tc.ErrorIs, upgradeerrors.AlreadyStarted)

	s.ensureUpgradeInfoState(c, uuid, upgrade.Started)
}

func (s *stateSuite) TestStartUpgradeBeforeCreated(c *tc.C) {
	uuid := uuid.MustNewUUID().String()
	err := s.st.StartUpgrade(c.Context(), domainupgrade.UUID(uuid))
	c.Assert(err, tc.ErrorIs, upgradeerrors.NotFound)
}

func (s *stateSuite) TestSetControllerDone(c *tc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1), ('2', 2)")
	c.Assert(err, tc.ErrorIsNil)
	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "1")
	c.Assert(err, tc.ErrorIsNil)

	err = s.st.SetControllerDone(c.Context(), uuid, "1")
	c.Assert(err, tc.ErrorIsNil)

	nodeInfos := s.getUpgrade(c, s.st, uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nodeInfos, tc.HasLen, 2)
	c.Check(nodeInfos[0], tc.Equals, ControllerNodeInfo{ControllerNodeID: "0"})
	c.Check(nodeInfos[1], tc.Equals, ControllerNodeInfo{ControllerNodeID: "1", NodeUpgradeCompletedAt: nodeInfos[1].NodeUpgradeCompletedAt})
	c.Check(nodeInfos[1].NodeUpgradeCompletedAt.Valid, tc.IsTrue)
}

func (s *stateSuite) TestSetControllerDoneNotExists(c *tc.C) {
	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	err = s.st.SetControllerDone(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorMatches, `controller node "0" not ready`)
}

func (s *stateSuite) TestSetControllerDoneCompleteUpgrade(c *tc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1)")
	c.Assert(err, tc.ErrorIsNil)

	// Start the upgrade
	uuid := s.startUpgrade(c)

	// Set the nodes to ready.
	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "1")
	c.Assert(err, tc.ErrorIsNil)

	// Set the db upgrade completed.
	err = s.st.SetDBUpgradeCompleted(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that all the steps have been completed.
	err = s.st.SetControllerDone(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)

	// Check that the upgrade hasn't been completed for just one node.
	activeUpgrade, err := s.st.ActiveUpgrade(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(activeUpgrade, tc.Equals, uuid)

	// Set the last node.
	err = s.st.SetControllerDone(c.Context(), uuid, "1")
	c.Assert(err, tc.ErrorIsNil)

	// The active upgrade should be done.
	_, err = s.st.ActiveUpgrade(c.Context())
	c.Assert(err, tc.ErrorIs, upgradeerrors.NotFound)
}

func (s *stateSuite) TestSetControllerDoneCompleteUpgradeEmptyCompletedAt(c *tc.C) {
	db := s.DB()

	_, err := db.Exec("INSERT INTO controller_node (controller_id, dqlite_node_id) VALUES ('1', 1)")
	c.Assert(err, tc.ErrorIsNil)

	uuid := s.startUpgrade(c)

	err = s.st.SetControllerReady(c.Context(), uuid, "0")
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetControllerReady(c.Context(), uuid, "1")
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.Exec(`
UPDATE upgrade_info_controller_node 
SET    node_upgrade_completed_at = ''
WHERE  upgrade_info_uuid = ?
       AND controller_node_id = 0`, uuid)
	c.Assert(err, tc.ErrorIsNil)

	activeUpgrade, err := s.st.ActiveUpgrade(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activeUpgrade, tc.Equals, uuid)

	// Set the db upgrade completed.
	err = s.st.SetDBUpgradeCompleted(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)

	err = s.st.SetControllerDone(c.Context(), uuid, "1")
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.st.ActiveUpgrade(c.Context())
	c.Assert(err, tc.ErrorIs, upgradeerrors.NotFound)
}

func (s *stateSuite) TestActiveUpgradesNoUpgrades(c *tc.C) {
	_, err := s.st.ActiveUpgrade(c.Context())
	c.Assert(err, tc.ErrorIs, upgradeerrors.NotFound)
}

func (s *stateSuite) TestActiveUpgradesSingular(c *tc.C) {
	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	activeUpgrade, err := s.st.ActiveUpgrade(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(activeUpgrade, tc.Equals, uuid)
}

func (s *stateSuite) TestSetDBUpgradeCompleted(c *tc.C) {
	uuid := s.startUpgrade(c)

	err := s.st.SetDBUpgradeCompleted(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	err = s.st.SetDBUpgradeCompleted(c.Context(), uuid)
	c.Assert(err, tc.ErrorMatches, `expected to set upgrade state to db complete.*`)

	s.ensureUpgradeInfoState(c, uuid, upgrade.DBCompleted)
}

func (s *stateSuite) TestUpgradeInfo(c *tc.C) {
	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	upgradeInfo, err := s.st.UpgradeInfo(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(upgradeInfo, tc.Equals, upgrade.Info{
		UUID:            uuid.String(),
		PreviousVersion: "3.0.0",
		TargetVersion:   "3.0.1",
		State:           upgrade.Created,
	})
}

func (s *stateSuite) ensureUpgradeInfoState(c *tc.C, uuid domainupgrade.UUID, state upgrade.State) {
	upgradeInfo, err := s.st.UpgradeInfo(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(upgradeInfo.State, tc.Equals, state)
}

func (s *stateSuite) startUpgrade(c *tc.C) domainupgrade.UUID {
	_, err := s.st.ActiveUpgrade(c.Context())
	c.Assert(err, tc.ErrorIs, upgradeerrors.NotFound)

	uuid, err := s.st.CreateUpgrade(c.Context(), semversion.MustParse("3.0.0"), semversion.MustParse("3.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	err = s.st.StartUpgrade(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)

	return uuid
}

func (s *stateSuite) getUpgrade(c *tc.C, st *State, upgradeUUID domainupgrade.UUID) []ControllerNodeInfo {
	db, err := s.st.DB()
	c.Assert(err, tc.ErrorIsNil)

	nodeInfosQ := `
SELECT (controller_node_id, node_upgrade_completed_at) AS (&ControllerNodeInfo.*) FROM upgrade_info_controller_node
WHERE upgrade_info_uuid = $M.info_uuid`
	nodeInfosS, err := sqlair.Prepare(nodeInfosQ, ControllerNodeInfo{}, sqlair.M{})
	c.Assert(err, tc.ErrorIsNil)

	var (
		nodeInfos []ControllerNodeInfo
	)
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, nodeInfosS, sqlair.M{"info_uuid": upgradeUUID}).GetAll(&nodeInfos)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return nodeInfos
}
