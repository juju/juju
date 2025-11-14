// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/life"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type unitSuite struct {
	baseSuite
}

func TestUnitSuite(t *testing.T) {
	tc.Run(t, &unitSuite{})
}

func (s *unitSuite) TestUnitExists(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.UnitExists(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)

	exists, err = st.UnitExists(c.Context(), "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *unitSuite) TestEnsureUnitNotAliveCascadeNormalSuccessLastUnit(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	unitMachineUUID := s.getUnitMachineUUID(c, unitUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	cascade, err := st.EnsureUnitNotAliveCascade(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(*cascade.MachineUUID, tc.Equals, unitMachineUUID.String())

	// Unit had life "alive" and should now be "dying".
	s.checkUnitLife(c, unitUUID.String(), life.Dying)

	// The last machine had life "alive" and should now be "dying".
	s.checkMachineLife(c, unitMachineUUID.String(), life.Dying)
}

func (s *unitSuite) TestEnsureUnitNotAliveCascadeStorageAttachmentsDying(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	ctx := c.Context()

	// Create a storage pool and a storage instance attached to the app's unit.
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(
			ctx, "INSERT INTO storage_pool (uuid, name, type) VALUES ('pool-uuid', 'pool', 'whatever')",
		); err != nil {
			return err
		}

		inst := `
INSERT INTO storage_instance (
    uuid, storage_id, storage_pool_uuid, storage_kind_id, requested_size_mib,
    charm_name, storage_name, life_id
)
VALUES ('instance-uuid', 'does-not-matter', 'pool-uuid', 1, 100, 'charm-name', 'storage-name', 0)`
		if _, err := tx.ExecContext(ctx, inst); err != nil {
			return err
		}

		attach := `
INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES ('storage-attachment-uuid', 'instance-uuid', ?, 0)`
		if _, err := tx.ExecContext(ctx, attach, unitUUID); err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	cascade, err := st.EnsureUnitNotAliveCascade(ctx, unitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// Unit had life "alive" and should now be "dying".
	s.checkUnitLife(c, unitUUID.String(), life.Dying)

	// Storage attachment should be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM storage_attachment WHERE uuid = 'storage-attachment-uuid'")
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)

	c.Check(cascade.StorageAttachmentUUIDs, tc.DeepEquals, []string{"storage-attachment-uuid"})
}

func (s *unitSuite) TestEnsureUnitNotAliveDestroyStorage(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	ctx := c.Context()

	// Create a storage pool and a storage instance attached to the app's unit.
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(
			ctx, "INSERT INTO storage_pool (uuid, name, type) VALUES ('pool-uuid', 'pool', 'whatever')",
		); err != nil {
			return err
		}

		inst := `
INSERT INTO storage_instance (
	uuid, storage_id, storage_pool_uuid, requested_size_mib, charm_name, storage_name, life_id, storage_kind_id
)
VALUES ('instance-uuid', 'does-not-matter', 'pool-uuid', 100, 'charm-name', 'storage-name', 0, 0)`
		if _, err := tx.ExecContext(ctx, inst); err != nil {
			return err
		}

		attach := `
INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES ('storage-attachment-uuid', 'instance-uuid', ?, 0)`
		if _, err := tx.ExecContext(ctx, attach, unitUUID); err != nil {
			return err
		}

		owned := "INSERT INTO storage_unit_owner (storage_instance_uuid, unit_uuid) VALUES ('instance-uuid', ?)"
		if _, err := tx.ExecContext(ctx, owned, unitUUID); err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	cascade, err := st.EnsureUnitNotAliveCascade(ctx, unitUUID.String(), true)
	c.Assert(err, tc.ErrorIsNil)

	// Unit had life "alive" and should now be "dying".
	s.checkUnitLife(c, unitUUID.String(), life.Dying)

	// Storage attachment should be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM storage_attachment WHERE uuid = 'storage-attachment-uuid'")
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)

	// Storage instance should be "dying".
	row = s.DB().QueryRow("SELECT life_id FROM storage_instance WHERE uuid = 'instance-uuid'")
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)

	c.Check(cascade.StorageAttachmentUUIDs, tc.DeepEquals, []string{"storage-attachment-uuid"})
	c.Check(cascade.StorageInstanceUUIDs, tc.DeepEquals, []string{"instance-uuid"})
}

func (s *unitSuite) TestEnsureUnitNotAliveCascadeNormalSuccessLastUnitParentMachine(c *tc.C) {
	svc := s.setupApplicationService(c)
	app1UUID := s.createIAASApplication(c, svc, "foo",
		applicationservice.AddIAASUnitArg{},
	)
	app2UUID := s.createIAASApplication(c, svc, "bar",
		applicationservice.AddIAASUnitArg{},
	)

	app1UnitUUIDs := s.getAllUnitUUIDs(c, app1UUID)
	c.Assert(len(app1UnitUUIDs), tc.Equals, 1)
	app1UnitUUID := app1UnitUUIDs[0]

	app2UnitUUIDs := s.getAllUnitUUIDs(c, app2UUID)
	c.Assert(len(app2UnitUUIDs), tc.Equals, 1)
	app2UnitUUID := app2UnitUUIDs[0]

	app1UnitMachineUUID := s.getUnitMachineUUID(c, app1UnitUUID)
	app2UnitMachineUUID := s.getUnitMachineUUID(c, app2UnitUUID)

	_, err := s.DB().Exec(`
INSERT INTO machine_parent (machine_uuid, parent_uuid) VALUES (?, ?)
	`, app2UnitMachineUUID.String(), app1UnitMachineUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	cascade, err := st.EnsureUnitNotAliveCascade(c.Context(), app1UnitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cascade.MachineUUID, tc.IsNil)

	// Unit had life "alive" and should now be "dying".
	s.checkUnitLife(c, app1UnitUUID.String(), life.Dying)

	// The last machine had life "alive" and should be still alive, because
	// it is a parent machine.
	s.checkMachineLife(c, app1UnitMachineUUID.String(), life.Alive)
}

// Test to ensure that we don't prevent a unit from being set to "dying"
// if the machine is already in the "dying" state. This shouldn't happen,
// but we want to ensure that the state machine is resilient to this
// situation.
func (s *unitSuite) TestEnsureUnitNotAliveCascadeNormalSuccessLastUnitMachineAlreadyDying(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app",
		applicationservice.AddIAASUnitArg{},
	)

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	unitMachineUUID := s.getUnitMachineUUID(c, unitUUID)
	// Set the machine to "dying" manually.
	_, err := s.DB().Exec("UPDATE machine SET life_id = 1 WHERE uuid = ?", unitMachineUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	cascade, err := st.EnsureUnitNotAliveCascade(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// The machine was already "dying", so we don't expect a machine UUID.
	c.Assert(cascade.MachineUUID, tc.IsNil)

	// Unit had life "alive" and should now be "dying".
	s.checkUnitLife(c, unitUUID.String(), life.Dying)
}

func (s *unitSuite) TestEnsureUnitNotAliveCascadeNormalSuccess(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app",
		applicationservice.AddIAASUnitArg{},
	)

	_, _, err := svc.AddIAASUnits(
		c.Context(),
		"some-app",
		applicationservice.AddIAASUnitArg{
			AddUnitArg: applicationservice.AddUnitArg{
				// Place this unit on the same machine as the first one.
				Placement: instance.MustParsePlacement("0"),
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 2)
	unitUUID := unitUUIDs[0]

	unitMachineUUID := s.getUnitMachineUUID(c, unitUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	cascade, err := st.EnsureUnitNotAliveCascade(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// This isn't the last unit on the machine, so we don't expect a machine
	// UUID.
	c.Assert(cascade.MachineUUID, tc.IsNil)

	// Unit had life "alive" and should now be "dying".
	s.checkUnitLife(c, unitUUID.String(), life.Dying)

	// Don't set the machine life to "dying" if there are other units on it.
	s.checkMachineLife(c, unitMachineUUID.String(), life.Alive)
}

func (s *unitSuite) TestEnsureUnitNotAliveCascadeDyingSuccess(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.EnsureUnitNotAliveCascade(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// Unit was already "dying" and should be unchanged.
	s.checkUnitLife(c, unitUUID.String(), life.Dying)
}

func (s *unitSuite) TestEnsureUnitNotAliveCascadeNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// We don't care if it's already gone.
	_, err := st.EnsureUnitNotAliveCascade(c.Context(), "some-unit-uuid", false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestUnitRemovalNormalSuccess(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.UnitScheduleRemoval(
		c.Context(), "removal-uuid", unitUUID.String(), false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	row := s.DB().QueryRow(
		"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal where uuid = ?",
		"removal-uuid",
	)
	var (
		removalTypeID int
		rUUID         string
		force         bool
		scheduledFor  time.Time
	)
	err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalTypeID, tc.Equals, 1)
	c.Check(rUUID, tc.Equals, unitUUID.String())
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *unitSuite) TestGetRelationUnitsForUnit(c *tc.C) {
	// Arrange:
	// - Add a charm and application with three endpoints.
	// - Create a relation for each endpoint and put one unit from the
	//   application in the scope of all three.

	ctx := c.Context()

	charm := "some-charm"
	_, err := s.DB().ExecContext(
		ctx, "INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, ?)", charm, charm, 0)
	c.Assert(err, tc.ErrorIsNil)

	app := "some-app"
	_, err = s.DB().ExecContext(
		ctx, "INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)",
		app, app, 0, charm, network.AlphaSpaceId,
	)
	c.Assert(err, tc.ErrorIsNil)

	crs := []string{"some-charm-rel1", "some-charm-rel2", "some-charm-rel3"}
	for _, cr := range crs {
		_, err = s.DB().ExecContext(ctx, `
INSERT INTO charm_relation (uuid, charm_uuid, name, interface, capacity, role_id,  scope_id)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			cr, charm, cr, "interface", 0, 0, 0,
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	aes := []string{"some-app-ep1", "some-app-ep2", "some-app-ep3"}
	for i, ae := range aes {
		_, err = s.DB().ExecContext(ctx, `
INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) 
VALUES (?, ?, ?, ?)`,
			ae, app, network.AlphaSpaceId, crs[i],
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	rs := []string{"some-rel1", "some-rel2", "some-rel3"}
	for i, r := range rs {
		_, err = s.DB().ExecContext(
			ctx, "INSERT INTO relation (uuid, life_id, relation_id, scope_id) VALUES (?, ?, ?, ?)", r, 0, i, 0)
		c.Assert(err, tc.ErrorIsNil)
	}

	res := []string{"some-rel-ep1", "some-rel-ep2", "some-rel-ep3"}
	for i, re := range res {
		_, err = s.DB().ExecContext(ctx,
			"INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid) VALUES (?, ?, ?)", re, rs[i], aes[i],
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	node := "some-net-node"
	_, err = s.DB().Exec("INSERT INTO net_node (uuid) VALUES (?)", node)
	c.Assert(err, tc.ErrorIsNil)

	unit := "some-unit"
	_, err = s.DB().Exec(
		"INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)",
		unit, unit, 0, app, charm, node)
	c.Assert(err, tc.ErrorIsNil)

	rus := []string{"some-rel-unit1", "some-rel-unit2", "some-rel-unit3"}
	for i, ru := range rus {
		_, err = s.DB().Exec("INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid) VALUES (?, ?, ?)",
			ru, res[i], unit,
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	// Act:
	// - Check for extant and non-extant scopes.
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	relUnits1, err1 := st.GetRelationUnitsForUnit(ctx, unit)
	relUnits2, err2 := st.GetRelationUnitsForUnit(ctx, "nah")

	// Assert
	c.Assert(err1, tc.ErrorIsNil)
	c.Assert(err2, tc.ErrorIsNil)

	c.Check(relUnits1, tc.SameContents, rus)
	c.Check(relUnits2, tc.HasLen, 0)
}

func (s *unitSuite) TestUnitRemovalNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.UnitScheduleRemoval(
		c.Context(), "removal-uuid", "some-unit-uuid", true, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	// It doesn't matter that the unit does not exist.
	// We rely on the worker to handle that fact.
	row := s.DB().QueryRow(`
SELECT t.name, r.entity_uuid, r.force, r.scheduled_for
FROM   removal r JOIN removal_type t ON r.removal_type_id = t.id
where  r.uuid = ?`, "removal-uuid",
	)

	var (
		removalType  string
		rUUID        string
		force        bool
		scheduledFor time.Time
	)
	err = row.Scan(&removalType, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalType, tc.Equals, "unit")
	c.Check(rUUID, tc.Equals, "some-unit-uuid")
	c.Check(force, tc.Equals, true)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *unitSuite) TestGetUnitLifeSuccess(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetUnitLife(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Alive)

	// Set the unit to "dying" manually.
	s.advanceUnitLife(c, unitUUID, life.Dying)

	l, err = st.GetUnitLife(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Dying)
}

func (s *unitSuite) TestGetUnitLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetUnitLife(c.Context(), "some-unit-uuid")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitSuite) TestMarkUnitAsDead(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.MarkUnitAsDead(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)

	_, err = s.DB().Exec("UPDATE unit SET life_id = 1 WHERE uuid = ?", unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	err = st.MarkUnitAsDead(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The unit should now be dead.
	s.checkUnitLife(c, unitUUID.String(), life.Dead)
}

func (s *unitSuite) TestMarkUnitAsDeadNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.MarkUnitAsDead(c.Context(), "abc")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitSuite) TestDeleteUnitNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteUnit(c.Context(), "blah", false)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitSuite) TestDeleteIAASUnitConsumingSecret(c *tc.C) {
	// Arrange
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	ctx := c.Context()

	// Add a secret that the unit is consuming.
	// The consumer reference should be deleted.
	// We don't need an assertion, because the
	// deletion would fail due to FK violation.
	sID := "some-secret"
	_, err := s.DB().ExecContext(ctx, "INSERT INTO secret VALUES (?)", sID)
	c.Assert(err, tc.ErrorIsNil)

	q := `
INSERT INTO secret_unit_consumer(secret_id, source_model_uuid, unit_uuid, current_revision) 
VALUES (?, 'some-model', ?, 0)`
	_, err = s.DB().ExecContext(ctx, q, sID, unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// We only check the unit life for "alive" in the state layer.
	// The service layer is responsible for calling DeleteUnit according
	// to its current life value and whether a forced removal is being actioned.
	s.advanceUnitLife(c, unitUUID, life.Dying)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Act
	err = st.DeleteUnit(ctx, unitUUID.String(), false)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	// The unit should be gone.
	exists, err := st.UnitExists(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	// The charm isn't removed because the application still references it.
	s.checkCharmsCount(c, 1)
}

func (s *unitSuite) TestDeleteSubordinateUnit(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID1 := s.createIAASApplication(c, svc, "foo", applicationservice.AddIAASUnitArg{})
	appUUID2 := s.createIAASSubordinateApplication(c, svc, "baz", applicationservice.AddIAASUnitArg{})

	// Force the second application to be a subordinate of the first.

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID1)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	subUnitUUIDs := s.getAllUnitUUIDs(c, appUUID2)
	c.Assert(len(subUnitUUIDs), tc.Equals, 1)
	subUnitUUID := subUnitUUIDs[0]

	s.advanceUnitLife(c, subUnitUUID, life.Dead)

	_, err := s.DB().Exec(`INSERT INTO unit_principal (unit_uuid, principal_uuid) VALUES (?, ?)`,
		subUnitUUID.String(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.DeleteUnit(c.Context(), subUnitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitSuite) TestDeleteIAASUnitWithSubordinates(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID1 := s.createIAASApplication(c, svc, "foo", applicationservice.AddIAASUnitArg{})
	appUUID2 := s.createIAASSubordinateApplication(c, svc, "baz", applicationservice.AddIAASUnitArg{})

	// Force the second application to be a subordinate of the first.

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID1)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	subUnitUUIDs := s.getAllUnitUUIDs(c, appUUID2)
	c.Assert(len(subUnitUUIDs), tc.Equals, 1)
	subUnitUUID := subUnitUUIDs[0]

	s.advanceUnitLife(c, unitUUID, life.Dead)

	_, err := s.DB().Exec(`INSERT INTO unit_principal (unit_uuid, principal_uuid) VALUES (?, ?)`,
		subUnitUUID.String(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.DeleteUnit(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)

	_, err = s.DB().Exec(`DELETE FROM unit_principal`)
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteUnit(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// The unit should be gone.
	exists, err := st.UnitExists(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	// The charm isn't removed because the application still references it.
	s.checkCharmsCount(c, 2)
}

func (s *unitSuite) TestDeleteIAASUnitWithSubordinatesNotDying(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID1 := s.createIAASApplication(c, svc, "foo", applicationservice.AddIAASUnitArg{})
	appUUID2 := s.createIAASSubordinateApplication(c, svc, "baz", applicationservice.AddIAASUnitArg{})

	// Force the second application to be a subordinate of the first.

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID1)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	subUnitUUIDs := s.getAllUnitUUIDs(c, appUUID2)
	c.Assert(len(subUnitUUIDs), tc.Equals, 1)
	subUnitUUID := subUnitUUIDs[0]

	_, err := s.DB().Exec(`INSERT INTO unit_principal (unit_uuid, principal_uuid) VALUES (?, ?)`,
		subUnitUUID.String(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err = st.DeleteUnit(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorMatches, `.*still alive.*`)
}

func (s *unitSuite) TestDeleteIAASUnitWithOperation(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	s.addOperation(c) // control operation
	opUUID := s.addOperationAction(c, s.addCharm(c), "op")
	s.addOperationUnitTask(c, opUUID, unitUUID.String())

	s.advanceUnitLife(c, unitUUID, life.Dead)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteUnit(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// The unit should be gone.
	exists, err := st.UnitExists(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	// The operation should be gone, since it is only linked to the associated unit.
	c.Check(s.getRowCount(c, "operation"), tc.Equals, 1)
}

func (s *unitSuite) TestDeleteIAASUnitWithOperationExec(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	s.addOperation(c) // control operation
	// Exec operation without params (no entry in operation_action nor operation_parameters)
	opUUID := s.addOperation(c)
	s.addOperationUnitTask(c, opUUID, unitUUID.String())

	s.advanceUnitLife(c, unitUUID, life.Dead)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteUnit(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// The unit should be gone.
	exists, err := st.UnitExists(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	// The operation should be gone, since it is only linked to the associated unit.
	c.Check(s.getRowCount(c, "operation"), tc.Equals, 1)
}

func (s *unitSuite) TestDeleteIAASUnitWithOperationSpannedToSeveralUnit(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	opUUID := s.addOperationAction(c, s.addCharm(c), "op")
	s.addOperationUnitTask(c, opUUID, unitUUID.String())
	s.addOperationUnitTask(c, opUUID, s.addUnit(c, s.addCharm(c))) // Spans to another unit.

	s.advanceUnitLife(c, unitUUID, life.Dead)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteUnit(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// The unit should be gone.
	exists, err := st.UnitExists(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	// The operation should not be gone, since it is linked to another unit.
	c.Check(s.getRowCount(c, "operation"), tc.Equals, 1)
	c.Check(s.getRowCount(c, "operation_task"), tc.Equals, 1)
}

func (s *unitSuite) TestDeleteCAASUnit(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createCAASApplication(c, svc, "some-app", applicationservice.AddUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	s.advanceUnitLife(c, unitUUID, life.Dead)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.expectK8sPodCount(c, unitUUID, 1)

	err := st.DeleteUnit(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// The unit should be gone.
	exists, err := st.UnitExists(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	s.expectK8sPodCount(c, unitUUID, 0)

	// The charm isn't removed because the application still references it.
	s.checkCharmsCount(c, 1)
}

// TestDeleteUnitWithDanglingCharmReference ensures that unit removal and
// charm's cleanup are in two different transactions. So even if the latter fails, the unit
// is still removed.
func (s *unitSuite) TestDeleteUnitWithDanglingCharmReference(c *tc.C) {
	// Arrange: Create an application with a unit, update the application to reference a new charm, and
	// add a dangling charm_external_reference.
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	s.advanceUnitLife(c, unitUUID, life.Dead)

	charmUUID := s.addCharm(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS charm_external_reference (
			uuid TEXT PRIMARY KEY,
			charm_uuid TEXT,
			created_at DATETIME,
			FOREIGN KEY(charm_uuid) REFERENCES charm(uuid)
		)`)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
		INSERT INTO charm_external_reference (uuid, charm_uuid, created_at)
		VALUES (?, ?, ?)`, "dangling-charm-ref-1", charmUUID, time.Now())

		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE application SET charm_uuid = ? WHERE uuid = ?`, charmUUID, appUUID.String())
		return err

	})
	c.Assert(err, tc.ErrorIsNil)

	// Act: Delete the unit
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteUnit(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// Assert: The unit is deleted
	exists, err := st.UnitExists(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *unitSuite) TestGetCharmForUnit(c *tc.C) {
	// Arrange: One application with a unit and a charm.
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	expectedCharmUUID := s.getCharmUUIDForUnit(c, unitUUID.String())

	// Act: Get the charm for the unit.
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	charmUUID, err := st.GetCharmForUnit(c.Context(), unitUUID.String())

	// Assert: The charm UUID is correct.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(charmUUID, tc.Equals, expectedCharmUUID)
}

func (s *unitSuite) TestGetCharmForUnitNotFound(c *tc.C) {
	// Act: Get the charm for a non-existent unit.
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	charmUUID, err := st.GetCharmForUnit(c.Context(), "some-unit-uuid")

	// Assert: The unit is not found. Error is nil and charmUUID is empty.
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(charmUUID, tc.Equals, "")
}

func (s *unitSuite) TestDeleteCharmIfUnusedAfterUnitDeletion(c *tc.C) {
	// Arrange: Create an application with a unit.
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	s.advanceUnitLife(c, unitUUID, life.Dead)
	s.advanceApplicationLife(c, appUUID, life.Dead)

	charmUUID := s.getCharmUUIDForUnit(c, unitUUID.String())

	// Act: Delete the application, the unit and the charm.
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := st.DeleteUnit(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)
	err = st.DeleteApplication(c.Context(), appUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)
	err = st.DeleteCharmIfUnused(c.Context(), charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Assert: The unit is deleted and the charm is also deleted as it is unused.
	exists, err := st.UnitExists(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	// Check that the charm is deleted.
	s.checkNoCharmsExist(c)
}

func (s *unitSuite) TestDeleteCharmIfUnusedBeforeUnitDeletion(c *tc.C) {
	// Arrange: Create an application with a unit.
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	s.advanceUnitLife(c, unitUUID, life.Dead)
	s.advanceApplicationLife(c, appUUID, life.Dead)

	charmUUID := s.getCharmUUIDForUnit(c, unitUUID.String())

	// Act: Delete the charm, the unit and the application.
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := st.DeleteCharmIfUnused(c.Context(), charmUUID)

	// Assert: The charm deletion shouldn't fail as it's still referenced.
	// And the charm shouldn't be deleted.
	c.Assert(err, tc.ErrorIsNil)
	s.checkCharmsCount(c, 1)
}

// TestDeleteCAASUnitNotAffectingOtherUnits is a regression test where deleting a CAAS unit
// would remove the k8s pod info for other units.
func (s *unitSuite) TestDeleteCAASUnitNotAffectingOtherUnits(c *tc.C) {
	// Create two CAAS applications, each with one unit.
	// Update the k8s pod info for each unit to have different addresses.
	svc := s.setupApplicationService(c)
	app1 := "some-app"
	appUUID1 := s.createCAASApplication(c, svc, app1, applicationservice.AddUnitArg{})

	err := svc.UpdateCAASUnit(c.Context(), unit.Name(fmt.Sprintf("%s/0", app1)), applicationservice.UpdateCAASUnitParams{
		ProviderID: ptr("provider-id"),
		Address:    ptr("10.0.0.1"),
	})
	c.Assert(err, tc.ErrorIsNil)

	k8sPodInfoApp1, err := svc.GetUnitK8sPodInfo(c.Context(), unit.Name(fmt.Sprintf("%s/0", app1)))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(k8sPodInfoApp1.Address, tc.Equals, "10.0.0.1")

	app2 := "some-otherapp"
	s.createCAASApplication(c, svc, app2, applicationservice.AddUnitArg{})
	err = svc.UpdateCAASUnit(c.Context(), unit.Name(fmt.Sprintf("%s/0", app2)), applicationservice.UpdateCAASUnitParams{
		ProviderID: ptr("provider-id-2"),
		Address:    ptr("10.0.0.2"),
	})
	c.Assert(err, tc.ErrorIsNil)

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID1)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	// delete the first unit
	s.advanceUnitLife(c, unitUUID, life.Dead)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteUnit(c.Context(), unitUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// The unit should be gone.
	exists, err := st.UnitExists(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	// Fetching the k8s pod info for the second unit should still work.
	k8sPodInfoApp2, err := svc.GetUnitK8sPodInfo(c.Context(), "some-otherapp/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(k8sPodInfoApp2.Address, tc.Equals, "10.0.0.2")
}

func (s *unitSuite) TestGetApplicationNameAndUnitNameByUnitUUID(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(len(unitUUIDs), tc.Equals, 1)
	unitUUID := unitUUIDs[0]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	appName, unitName, err := st.GetApplicationNameAndUnitNameByUnitUUID(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(appName, tc.Equals, "some-app")
	c.Check(unitName, tc.Equals, "some-app/0")
}

func (s *unitSuite) TestGetApplicationNameAndUnitNameByUnitUUIDNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, _, err := st.GetApplicationNameAndUnitNameByUnitUUID(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *unitSuite) expectK8sPodCount(c *tc.C, unitUUID unit.UUID, expected int) {
	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM k8s_pod WHERE unit_uuid = ?`, unitUUID.String())
		if err := row.Scan(&count); err != nil {
			return err
		}
		return row.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, expected)
}

func (s *unitSuite) getCharmUUIDForUnit(c *tc.C, unitUUID string) string {
	row := s.DB().QueryRow("SELECT charm_uuid FROM unit WHERE uuid = ?", unitUUID)
	var charmUUID string
	err := row.Scan(&charmUUID)
	c.Assert(err, tc.ErrorIsNil)
	return charmUUID
}
