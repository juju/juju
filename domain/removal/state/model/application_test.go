// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/relation"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	charmresource "github.com/juju/juju/internal/charm/resource"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type applicationSuite struct {
	baseSuite
}

func TestApplicationSuite(t *testing.T) {
	tc.Run(t, &applicationSuite{})
}

func (s *applicationSuite) TestApplicationExists(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.ApplicationExists(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)

	exists, err = st.ApplicationExists(c.Context(), "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *applicationSuite) TestEnsureApplicationNotAliveCascadeNormalSuccess(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	artifacts, err := st.EnsureApplicationNotAliveCascade(c.Context(), appUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// We don't have any units, so we expect an empty slice for both unit and
	// machine UUIDs.
	c.Check(artifacts.Empty(), tc.Equals, true)

	// Unit had life "alive" and should now be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM application where uuid = ?", appUUID.String())
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *applicationSuite) TestEnsureApplicationNotAliveCascadeNormalSuccessWithAliveUnitsCascadedStorage(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app",
		applicationservice.AddIAASUnitArg{},
		applicationservice.AddIAASUnitArg{},
	)

	allUnitUUIDs, allMachineUUIDs := s.getAllUnitAndMachineUUIDs(c)
	c.Assert(allUnitUUIDs, tc.HasLen, 2)
	c.Assert(allMachineUUIDs, tc.HasLen, 2)

	ctx := c.Context()
	db := s.DB()

	// Add storage to one of the units.
	_, err := db.ExecContext(
		ctx, "INSERT INTO storage_pool (uuid, name, type) VALUES ('pool-uuid', 'pool', 'whatever')")
	c.Assert(err, tc.ErrorIsNil)

	inst := `
INSERT INTO storage_instance (
	uuid, storage_id, storage_pool_uuid, requested_size_mib, charm_name, storage_name, life_id, storage_kind_id
)
VALUES ('instance-uuid', 'does-not-matter', 'pool-uuid', 100, 'charm-name', 'storage-name', 0, 0)`
	_, err = db.ExecContext(ctx, inst)
	c.Assert(err, tc.ErrorIsNil)

	attach := `
INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES ('storage-attachment-uuid', 'instance-uuid', ?, 0)`
	_, err = db.ExecContext(ctx, attach, allUnitUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)

	owned := "INSERT INTO storage_unit_owner (storage_instance_uuid, unit_uuid) VALUES ('instance-uuid', ?)"
	_, err = db.ExecContext(ctx, owned, allUnitUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Perform the ensure operation with destroyStorage.
	artifacts, err := st.EnsureApplicationNotAliveCascade(ctx, appUUID.String(), true)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(artifacts.RelationUUIDs, tc.HasLen, 0)
	s.checkUnitContents(c, artifacts.UnitUUIDs, allUnitUUIDs)
	s.checkMachineContents(c, artifacts.MachineUUIDs, allMachineUUIDs)

	s.checkApplicationDyingState(c, appUUID)
	s.checkUnitDyingState(c, allUnitUUIDs)
	s.checkMachineDyingState(c, allMachineUUIDs)

	// Storage attachment should be "dying".
	row := db.QueryRowContext(ctx, "SELECT life_id FROM storage_attachment WHERE uuid = 'storage-attachment-uuid'")
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)

	// Storage instance should be "dying".
	row = db.QueryRowContext(ctx, "SELECT life_id FROM storage_instance WHERE uuid = 'instance-uuid'")
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *applicationSuite) TestEnsureApplicationNotAliveCascadeNormalSuccessWithAliveAndDyingUnits(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app",
		applicationservice.AddIAASUnitArg{},
		applicationservice.AddIAASUnitArg{},
		applicationservice.AddIAASUnitArg{},
	)

	allUnitUUIDs, allMachineUUIDs := s.getAllUnitAndMachineUUIDs(c)
	c.Assert(allUnitUUIDs, tc.HasLen, 3)
	c.Assert(allMachineUUIDs, tc.HasLen, 3)

	// Update one of the units and its associated machine to be "dying". This
	// will simulate a scenario that someone did `juju remove-unit` on one of
	// the units and then `juju remove-application` was called.
	_, err := s.DB().Exec(`UPDATE unit SET life_id = 1 WHERE uuid = ?`, allUnitUUIDs[0].String())
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().Exec(`UPDATE machine SET life_id = 1 WHERE uuid = ?`, allMachineUUIDs[0].String())
	c.Assert(err, tc.ErrorIsNil)

	aliveUnitUUIDs := allUnitUUIDs[1:]
	aliveMachineUUIDs := allMachineUUIDs[1:]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	artifacts, err := st.EnsureApplicationNotAliveCascade(c.Context(), appUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(artifacts.RelationUUIDs, tc.HasLen, 0)
	s.checkUnitContents(c, artifacts.UnitUUIDs, aliveUnitUUIDs)
	s.checkMachineContents(c, artifacts.MachineUUIDs, aliveMachineUUIDs)

	s.checkApplicationDyingState(c, appUUID)
	s.checkUnitDyingState(c, aliveUnitUUIDs)
	s.checkMachineDyingState(c, aliveMachineUUIDs)
}

func (s *applicationSuite) TestEnsureApplicationNotAliveCascadeNormalSuccessWithAliveAndDyingUnitsWithLastMachine(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app",
		applicationservice.AddIAASUnitArg{},
		applicationservice.AddIAASUnitArg{},
	)
	_, _, err := svc.AddIAASUnits(c.Context(), "some-app", applicationservice.AddIAASUnitArg{
		AddUnitArg: applicationservice.AddUnitArg{
			Placement: instance.MustParsePlacement("0"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	allUnitUUIDs, allMachineUUIDs := s.getAllUnitAndMachineUUIDs(c)
	c.Assert(allUnitUUIDs, tc.HasLen, 3)
	c.Assert(allMachineUUIDs, tc.HasLen, 3)

	// There should be a unit with the same placement as the machine.
	uniqueMachineUUIDs := removeDuplicates(allMachineUUIDs)
	c.Assert(uniqueMachineUUIDs, tc.HasLen, 2)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	artifacts, err := st.EnsureApplicationNotAliveCascade(c.Context(), appUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(artifacts.RelationUUIDs, tc.HasLen, 0)
	s.checkUnitContents(c, artifacts.UnitUUIDs, allUnitUUIDs)
	s.checkMachineContents(c, artifacts.MachineUUIDs, uniqueMachineUUIDs)

	s.checkApplicationDyingState(c, appUUID)
	s.checkUnitDyingState(c, allUnitUUIDs)
	s.checkMachineDyingState(c, uniqueMachineUUIDs)
}

// Ensure that if another application is using the machine, it will not be
// set to dying.
func (s *applicationSuite) TestEnsureApplicationOnMultipleMachines(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID1 := s.createIAASApplication(c, svc, "foo",
		applicationservice.AddIAASUnitArg{},
	)
	appUUID2 := s.createIAASApplication(c, svc, "bar",
		applicationservice.AddIAASUnitArg{
			AddUnitArg: applicationservice.AddUnitArg{
				Placement: instance.MustParsePlacement("0"),
			},
		},
	)

	allUnitUUIDs, allMachineUUIDs := s.getAllUnitAndMachineUUIDs(c)
	c.Assert(allUnitUUIDs, tc.HasLen, 2)
	c.Assert(allMachineUUIDs, tc.HasLen, 2)

	// There should be a unit with the same placement as the machine.
	uniqueMachineUUIDs := removeDuplicates(allMachineUUIDs)
	c.Assert(uniqueMachineUUIDs, tc.HasLen, 1)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	artifacts, err := st.EnsureApplicationNotAliveCascade(c.Context(), appUUID1.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	app1UnitUUIDs := s.getAllUnitUUIDs(c, appUUID1)
	app2UnitUUIDs := s.getAllUnitUUIDs(c, appUUID2)

	c.Check(artifacts.RelationUUIDs, tc.HasLen, 0)
	s.checkUnitContents(c, artifacts.UnitUUIDs, app1UnitUUIDs)
	c.Check(artifacts.MachineUUIDs, tc.HasLen, 0)

	s.checkApplicationDyingState(c, appUUID1)

	var (
		count int
		uuid  string
	)

	// There should be one unit that is alive (from app2), and one that is dying
	// (from app1).
	row := s.DB().QueryRow("SELECT COUNT(*), uuid FROM unit WHERE life_id = 0")
	err = row.Scan(&count, &uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
	c.Check(uuid, tc.Equals, app2UnitUUIDs[0].String())

	row = s.DB().QueryRow("SELECT COUNT(*), uuid FROM unit WHERE life_id != 0")
	err = row.Scan(&count, &uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
	c.Check(uuid, tc.Equals, app1UnitUUIDs[0].String())

	// The machine is being used by another application, so it should be
	// still alive.
	row = s.DB().QueryRow("SELECT COUNT(*) FROM machine WHERE life_id = 0")
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	// Ensure that there are no other machines that are not alive.
	row = s.DB().QueryRow("SELECT COUNT(*) FROM machine WHERE life_id != 0")
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *applicationSuite) TestEnsureApplicationNotAliveCascadeNormalSuccessWithRelations(c *tc.C) {
	appSvc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, appSvc, "app1")
	s.createIAASApplication(c, appSvc, "app2")

	relSvc := s.setupRelationService(c)
	_, _, err := relSvc.AddRelation(c.Context(), "app1:foo", "app2:bar")
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	artifacts, err := st.EnsureApplicationNotAliveCascade(c.Context(), appUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(artifacts.RelationUUIDs, tc.HasLen, 1)
	c.Check(artifacts.UnitUUIDs, tc.HasLen, 0)
	c.Check(artifacts.MachineUUIDs, tc.HasLen, 0)

	// Check the relation is no longer alive.
	var count int
	row := s.DB().QueryRow("SELECT COUNT(*) FROM relation WHERE life_id = 0")
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *applicationSuite) TestEnsureApplicationNotAliveCascadeDyingSuccess(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	artifacts, err := st.EnsureApplicationNotAliveCascade(c.Context(), appUUID.String(), false)
	c.Assert(err, tc.ErrorIsNil)

	// We don't have any units, so we expect an empty slice for both unit and
	// machine UUIDs.
	c.Check(artifacts.Empty(), tc.Equals, true)

	// Unit was already "dying" and should be unchanged.
	row := s.DB().QueryRow("SELECT life_id FROM application where uuid = ?", appUUID.String())
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *applicationSuite) TestEnsureApplicationNotAliveCascadeNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// We don't care if it's already gone.
	_, err := st.EnsureApplicationNotAliveCascade(c.Context(), "some-application-uuid", false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationSuite) TestApplicationRemovalNormalSuccess(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.ApplicationScheduleRemoval(
		c.Context(), "removal-uuid", appUUID.String(), false, when,
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

	c.Check(removalTypeID, tc.Equals, 2)
	c.Check(rUUID, tc.Equals, appUUID.String())
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *applicationSuite) TestApplicationRemovalNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.ApplicationScheduleRemoval(
		c.Context(), "removal-uuid", "some-application-uuid", true, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	// It doesn't matter that the application does not exist.
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

	c.Check(removalType, tc.Equals, "application")
	c.Check(rUUID, tc.Equals, "some-application-uuid")
	c.Check(force, tc.Equals, true)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *applicationSuite) TestGetApplicationLifeSuccess(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddIAASUnitArg{})

	// Set the application to "dying" manually.
	_, err := s.DB().Exec("UPDATE application SET life_id = 1 WHERE uuid = ?", appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetApplicationLife(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Dying)
}

func (s *applicationSuite) TestGetApplicationLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetApplicationLife(c.Context(), "some-application-uuid")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *applicationSuite) TestDeleteIAASApplication(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app")

	s.advanceApplicationLife(c, appUUID, life.Dead)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteApplication(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The application should be gone.
	exists, err := st.ApplicationExists(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *applicationSuite) TestDeleteIAASApplicationWithUnits(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, svc, "some-app",
		applicationservice.AddIAASUnitArg{},
	)

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(unitUUIDs, tc.HasLen, 1)

	s.advanceUnitLife(c, unitUUIDs[0], life.Dead)
	s.advanceApplicationLife(c, appUUID, life.Dead)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// This should fail because the application has units.
	err := st.DeleteApplication(c.Context(), appUUID.String())
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)
	c.Check(err, tc.ErrorIs, applicationerrors.ApplicationHasUnits)

	// Delete any units associated with the application.
	err = st.DeleteUnit(c.Context(), unitUUIDs[0].String())
	c.Assert(err, tc.ErrorIsNil)

	// Now we can delete the application.
	err = st.DeleteApplication(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The application should be gone.
	exists, err := st.ApplicationExists(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	s.checkNoCharmsExist(c)
}

func (s *applicationSuite) TestDeleteIAASApplicationWithRelations(c *tc.C) {
	appSvc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, appSvc, "app1")
	s.createIAASApplication(c, appSvc, "app2")

	relSvc := s.setupRelationService(c)
	ep1, ep2, err := relSvc.AddRelation(c.Context(), "app1:foo", "app2:bar")
	c.Assert(err, tc.ErrorIsNil)
	relUUID, err := relSvc.GetRelationUUIDForRemoval(c.Context(), relation.GetRelationUUIDForRemovalArgs{
		Endpoints: []string{ep1.String(), ep2.String()},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.advanceApplicationLife(c, appUUID, life.Dead)
	s.advanceRelationLife(c, relUUID, life.Dead)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// This should fail because the application has units.
	err = st.DeleteApplication(c.Context(), appUUID.String())
	c.Check(err, tc.ErrorIs, removalerrors.RemovalJobIncomplete)
	c.Check(err, tc.ErrorIs, applicationerrors.ApplicationHasRelations)

	// Delete any relations associated with the application.
	err = st.DeleteRelation(c.Context(), relUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Now we can delete the application.
	err = st.DeleteApplication(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The application should be gone.
	exists, err := st.ApplicationExists(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *applicationSuite) TestDeleteIAASApplicationMultipleRemovesCharm(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID1 := s.createIAASApplication(c, svc, "foo",
		applicationservice.AddIAASUnitArg{},
	)
	appUUID2 := s.createIAASApplication(c, svc, "bar")

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID1)
	c.Assert(unitUUIDs, tc.HasLen, 1)

	s.advanceUnitLife(c, unitUUIDs[0], life.Dead)
	s.advanceApplicationLife(c, appUUID1, life.Dead)
	s.advanceApplicationLife(c, appUUID2, life.Dead)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Delete any units associated with the application.
	err := st.DeleteUnit(c.Context(), unitUUIDs[0].String())
	c.Assert(err, tc.ErrorIsNil)

	// Now we can delete the application.
	err = st.DeleteApplication(c.Context(), appUUID1.String())
	c.Assert(err, tc.ErrorIsNil)

	s.checkCharmsCount(c, 1)

	// Now we can delete the application and the charm should be removed as
	// well.
	err = st.DeleteApplication(c.Context(), appUUID2.String())
	c.Assert(err, tc.ErrorIsNil)

	s.checkNoCharmsExist(c)
}

func (s *applicationSuite) TestDeleteCAASApplication(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createCAASApplication(c, svc, "some-app", applicationservice.AddUnitArg{})

	unitUUIDs := s.getAllUnitUUIDs(c, appUUID)
	c.Assert(unitUUIDs, tc.HasLen, 1)

	s.advanceUnitLife(c, unitUUIDs[0], life.Dead)
	s.advanceApplicationLife(c, appUUID, life.Dead)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Delete any units associated with the application.
	err := st.DeleteUnit(c.Context(), unitUUIDs[0].String())
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteApplication(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The application should be gone.
	exists, err := st.ApplicationExists(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	s.checkNoCharmsExist(c)
}

func (s *applicationSuite) TestDeleteCAASApplicationWithUnit(c *tc.C) {
	svc := s.setupApplicationService(c)
	appUUID := s.createCAASApplication(c, svc, "some-app", applicationservice.AddUnitArg{})

	s.advanceApplicationLife(c, appUUID, life.Dead)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteApplication(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorMatches, `.*still has 1 unit.*`)
}

// TestDeleteApplicationNotWipingDevices is a regression test for where deleting an application
// would wipe the device_constraint_attribute table.
func (s *applicationSuite) TestDeleteApplicationNotWipingDeviceConstraints(c *tc.C) {
	svc := s.setupApplicationService(c)
	app1ID, err := svc.CreateIAASApplication(c.Context(), "app1", &stubCharm{name: "test-charm"}, corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, applicationservice.AddApplicationArgs{
		ReferenceName: "app1",
		DownloadInfo: &charm.DownloadInfo{
			Provenance:  charm.ProvenanceDownload,
			DownloadURL: "http://example.com",
		},
		ResolvedResources: applicationservice.ResolvedResources{{
			Name:     "buzz",
			Revision: ptr(42),
			Origin:   charmresource.OriginStore,
		}},
		Devices: map[string]devices.Constraints{
			"bitcoinminer": {
				Type:  "nvidia.com/gpu",
				Count: 10,
				Attributes: map[string]string{
					"model": "tesla",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	_, err = svc.CreateIAASApplication(c.Context(), "app2", &stubCharm{name: "test-charm"}, corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, applicationservice.AddApplicationArgs{
		ReferenceName: "app2",
		DownloadInfo: &charm.DownloadInfo{
			Provenance:  charm.ProvenanceDownload,
			DownloadURL: "http://example.com",
		},
		ResolvedResources: applicationservice.ResolvedResources{{
			Name:     "buzz",
			Revision: ptr(42),
			Origin:   charmresource.OriginStore,
		}},
		Devices: map[string]devices.Constraints{
			"bitcoinminer": {
				Type:  "nvidia.com/gpu",
				Count: 20,
				Attributes: map[string]string{
					"model": "tesla",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.advanceApplicationLife(c, app1ID, life.Dead)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteApplication(c.Context(), app1ID.String())
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.GetApplicationLife(c.Context(), "app1")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)

	devices, err := svc.GetDeviceConstraints(c.Context(), "app2")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(devices, tc.HasLen, 1)
	c.Assert(devices["bitcoinminer"].Count, tc.Equals, 20)
}

func (s *applicationSuite) TestDeleteApplicationWithObjectstoreResource(c *tc.C) {
	// Arrange: Two apps that share a resource object
	appSvc := s.setupApplicationService(c)
	appUUID := s.createIAASApplication(c, appSvc, "some-app")
	s.advanceApplicationLife(c, appUUID, life.Dead)

	resourceUUID := s.getAppResourceUUID(c, appUUID)

	// Arrange: Manually create a resource object store entry
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)
	_, err := s.DB().Exec("INSERT INTO object_store_metadata (uuid, sha_384, sha_256, size) VALUES (?, ?, ?, ?)",
		objectStoreUUID.String(), "sha384", "sha256", 42)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec("INSERT INTO object_store_metadata_path (metadata_uuid, path) VALUES (?, ?)",
		objectStoreUUID.String(), "/path/to/resource")
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec("INSERT INTO resource_file_store (resource_uuid, store_uuid, size, sha384) VALUES (?, ?, ?, ?)",
		resourceUUID, objectStoreUUID.String(), 42, "sha_384")
	c.Assert(err, tc.ErrorIsNil)

	// Act: Delete the application
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteApplication(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Assert: The application is deleted
	exists, err := st.ApplicationExists(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)

	// Assert: The resource object store entry is deleted
	row := s.DB().QueryRow("SELECT COUNT(*) FROM object_store_metadata WHERE uuid = ?", objectStoreUUID.String())
	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

func (s *applicationSuite) TestDeleteApplicationWithSharedObjectstoreResource(c *tc.C) {
	// Arrange: Two apps that share a resource object store entry
	appSvc := s.setupApplicationService(c)

	appUUID1 := s.createIAASApplication(c, appSvc, "some-app")
	s.advanceApplicationLife(c, appUUID1, life.Dead)
	appUUID2 := s.createIAASApplication(c, appSvc, "other-app")
	s.advanceApplicationLife(c, appUUID2, life.Dead)

	app1ResourceUUID := s.getAppResourceUUID(c, appUUID1)
	app2ResourceUUID := s.getAppResourceUUID(c, appUUID2)

	// Arrange: Manually create a resource object store entry
	// NOTE: We only add one, since resources can share the same object store entry
	objectStoreUUID := objectstoretesting.GenObjectStoreUUID(c)
	_, err := s.DB().Exec("INSERT INTO object_store_metadata (uuid, sha_384, sha_256, size) VALUES (?, ?, ?, ?)",
		objectStoreUUID.String(), "sha384", "sha256", 42)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec("INSERT INTO object_store_metadata_path (metadata_uuid, path) VALUES (?, ?)",
		objectStoreUUID.String(), "/path/to/resource")
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec("INSERT INTO object_store_metadata_path (metadata_uuid, path) VALUES (?, ?)",
		objectStoreUUID.String(), "/path/to/resource2")
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec("INSERT INTO resource_file_store (resource_uuid, store_uuid, size, sha384) VALUES (?, ?, ?, ?)",
		app1ResourceUUID, objectStoreUUID.String(), 42, "sha_384")
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().Exec("INSERT INTO resource_file_store (resource_uuid, store_uuid, size, sha384) VALUES (?, ?, ?, ?)",
		app2ResourceUUID, objectStoreUUID.String(), 42, "sha_384")
	c.Assert(err, tc.ErrorIsNil)

	// Act: Delete an application
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = st.DeleteApplication(c.Context(), appUUID1.String())
	c.Assert(err, tc.ErrorIsNil)

	// Assert: The application is deleted
	exists, err := st.ApplicationExists(c.Context(), appUUID1.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *applicationSuite) getAppResourceUUID(c *tc.C, appUUID coreapplication.UUID) string {
	row := s.DB().QueryRow(`
SELECT uuid
FROM application_resource ar
JOIN resource r ON ar.resource_uuid = r.uuid
WHERE ar.application_uuid = ?`, appUUID.String())
	var resourceUUID string
	err := row.Scan(&resourceUUID)
	c.Assert(err, tc.ErrorIsNil)
	return resourceUUID
}

func (s *applicationSuite) checkUnitContents(c *tc.C, actual []string, expected []unit.UUID) {
	c.Check(actual, tc.SameContents, transform.Slice(expected, func(u unit.UUID) string {
		return u.String()
	}))
}

func (s *applicationSuite) checkMachineContents(c *tc.C, actual []string, expected []machine.UUID) {
	c.Check(actual, tc.SameContents, transform.Slice(expected, func(m machine.UUID) string {
		return m.String()
	}))
}

func (s *applicationSuite) checkApplicationDyingState(c *tc.C, appUUID coreapplication.UUID) {
	row := s.DB().QueryRow("SELECT life_id FROM application where uuid = ?", appUUID.String())
	var lifeID int
	err := row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *applicationSuite) checkUnitDyingState(c *tc.C, unitUUIDs []unit.UUID) {
	// Ensure that there are no units left with life "alive".
	row := s.DB().QueryRow("SELECT COUNT(*) FROM unit WHERE  life_id = 0")
	var count int
	err := row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)

	// Ensure that all units are now "dying".
	placeholders := strings.Repeat("?,", len(unitUUIDs)-1) + "?"
	uuids := transform.Slice(unitUUIDs, func(u unit.UUID) any {
		return u.String()
	})

	row = s.DB().QueryRow(fmt.Sprintf(`
SELECT COUNT(*) FROM unit WHERE life_id = 1 AND uuid IN (%s)
`, placeholders), uuids...)
	var dyingCount int
	err = row.Scan(&dyingCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dyingCount, tc.Equals, len(unitUUIDs))
}

func (s *applicationSuite) checkMachineDyingState(c *tc.C, machineUUIDs []machine.UUID) {
	// Ensure that there are no machines left with life "alive".
	row := s.DB().QueryRow("SELECT COUNT(*) FROM machine WHERE life_id = 0")
	var count int
	err := row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)

	// Ensure that all machines are now "dying".
	placeholders := strings.Repeat("?,", len(machineUUIDs)-1) + "?"
	uuids := transform.Slice(machineUUIDs, func(u machine.UUID) any {
		return u.String()
	})

	row = s.DB().QueryRow(fmt.Sprintf(`
SELECT COUNT(*) FROM machine WHERE life_id = 1 AND uuid IN (%s)
`, placeholders), uuids...)
	var dyingCount int
	err = row.Scan(&dyingCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(dyingCount, tc.Equals, len(machineUUIDs))
}

func removeDuplicates[T comparable](uuids []T) []T {
	unique := make(map[T]struct{}, len(uuids))
	for _, uuid := range uuids {
		unique[uuid] = struct{}{}
	}
	result := make([]T, 0, len(unique))
	for uuid := range unique {
		result = append(result, uuid)
	}
	return result
}
