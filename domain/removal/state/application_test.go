// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/life"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type applicationSuite struct {
	baseSuite
}

func TestApplicationSuite(t *testing.T) {
	tc.Run(t, &applicationSuite{})
}

func (s *applicationSuite) TestApplicationExists(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddUnitArg{})

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.ApplicationExists(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)

	exists, err = st.ApplicationExists(c.Context(), "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *applicationSuite) TestEnsureApplicationNotAliveNormalSuccess(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	unitUUIDs, machineUUIDs, err := st.EnsureApplicationNotAlive(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// We don't have any units, so we expect an empty slice for both unit and
	// machine UUIDs.
	c.Check(unitUUIDs, tc.HasLen, 0)
	c.Check(machineUUIDs, tc.HasLen, 0)

	// Unit had life "alive" and should now be "dying".
	row := s.DB().QueryRow("SELECT life_id FROM application where uuid = ?", appUUID.String())
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *applicationSuite) TestEnsureApplicationNotAliveNormalSuccessWithAliveUnits(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app",
		applicationservice.AddUnitArg{},
		applicationservice.AddUnitArg{},
	)

	allUnitUUIDs, allMachineUUIDs := s.getAllUnitAndMachineUUIDs(c)
	c.Assert(allUnitUUIDs, tc.HasLen, 2)
	c.Assert(allMachineUUIDs, tc.HasLen, 2)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Perform the ensure operation.
	unitUUIDs, machineUUIDs, err := st.EnsureApplicationNotAlive(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	s.checkUnitContents(c, unitUUIDs, allUnitUUIDs)
	s.checkMachineContents(c, machineUUIDs, allMachineUUIDs)

	s.checkApplicationDyingState(c, appUUID)
	s.checkUnitDyingState(c, allUnitUUIDs)
	s.checkMachineDyingState(c, allMachineUUIDs)
}

func (s *applicationSuite) TestEnsureApplicationNotAliveNormalSuccessWithAliveAndDyingUnits(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app",
		applicationservice.AddUnitArg{},
		applicationservice.AddUnitArg{},
		applicationservice.AddUnitArg{},
	)

	allUnitUUIDs, allMachineUUIDs := s.getAllUnitAndMachineUUIDs(c)
	c.Assert(allUnitUUIDs, tc.HasLen, 3)
	c.Assert(allMachineUUIDs, tc.HasLen, 3)

	// Update one of the units and it's associated machine to be "dying". This
	// will simulate a scenario that someone did `juju remove-unit` on one of
	// the units and then `juju remove-application` was called.
	_, err := s.DB().Exec(`UPDATE unit SET life_id = 1 WHERE uuid = ?`, allUnitUUIDs[0].String())
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().Exec(`UPDATE machine SET life_id = 1 WHERE uuid = ?`, allMachineUUIDs[0].String())
	c.Assert(err, tc.ErrorIsNil)

	aliveUnitUUIDs := allUnitUUIDs[1:]
	aliveMachineUUIDs := allMachineUUIDs[1:]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	unitUUIDs, machineUUIDs, err := st.EnsureApplicationNotAlive(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	s.checkUnitContents(c, unitUUIDs, aliveUnitUUIDs)
	s.checkMachineContents(c, machineUUIDs, aliveMachineUUIDs)

	s.checkApplicationDyingState(c, appUUID)
	s.checkUnitDyingState(c, aliveUnitUUIDs)
	s.checkMachineDyingState(c, aliveMachineUUIDs)
}

func (s *applicationSuite) TestEnsureApplicationNotAliveNormalSuccessWithAliveAndDyingUnitsWithLastMachine(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app",
		applicationservice.AddUnitArg{},
		applicationservice.AddUnitArg{
			Placement: instance.MustParsePlacement("0"),
		},
		applicationservice.AddUnitArg{},
	)

	allUnitUUIDs, allMachineUUIDs := s.getAllUnitAndMachineUUIDs(c)
	c.Assert(allUnitUUIDs, tc.HasLen, 3)
	c.Assert(allMachineUUIDs, tc.HasLen, 3)

	// There should be a unit with the same placement as the machine.
	uniqueMachineUUIDs := removeDuplicates(allMachineUUIDs)
	c.Assert(uniqueMachineUUIDs, tc.HasLen, 2)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	unitUUIDs, machineUUIDs, err := st.EnsureApplicationNotAlive(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	s.checkUnitContents(c, unitUUIDs, allUnitUUIDs)
	s.checkMachineContents(c, machineUUIDs, uniqueMachineUUIDs)

	s.checkApplicationDyingState(c, appUUID)
	s.checkUnitDyingState(c, allUnitUUIDs)
	s.checkMachineDyingState(c, uniqueMachineUUIDs)
}

// Ensure that if another application is using the machine, it will not be
// set to dying.
func (s *applicationSuite) TestEnsureApplicationOnMultipleMachines(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID1 := s.createIAASApplication(c, svc, "foo",
		applicationservice.AddUnitArg{},
	)
	appUUID2 := s.createIAASApplication(c, svc, "bar",
		applicationservice.AddUnitArg{
			Placement: instance.MustParsePlacement("0"),
		},
	)

	allUnitUUIDs, allMachineUUIDs := s.getAllUnitAndMachineUUIDs(c)
	c.Assert(allUnitUUIDs, tc.HasLen, 2)
	c.Assert(allMachineUUIDs, tc.HasLen, 2)

	// There should be a unit with the same placement as the machine.
	uniqueMachineUUIDs := removeDuplicates(allMachineUUIDs)
	c.Assert(uniqueMachineUUIDs, tc.HasLen, 1)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	unitUUIDs, machineUUIDs, err := st.EnsureApplicationNotAlive(c.Context(), appUUID1.String())
	c.Assert(err, tc.ErrorIsNil)

	app1UnitUUIDs := s.getAllUnitUUIDs(c, appUUID1)
	app2UnitUUIDs := s.getAllUnitUUIDs(c, appUUID2)

	s.checkUnitContents(c, unitUUIDs, app1UnitUUIDs)
	s.checkMachineContents(c, machineUUIDs, []machine.UUID{})

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

func (s *applicationSuite) TestEnsureApplicationNotAliveDyingSuccess(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	unitUUIDs, machineUUIDs, err := st.EnsureApplicationNotAlive(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// We don't have any units, so we expect an empty slice for both unit and
	// machine UUIDs.
	c.Check(unitUUIDs, tc.HasLen, 0)
	c.Check(machineUUIDs, tc.HasLen, 0)

	// Unit was already "dying" and should be unchanged.
	row := s.DB().QueryRow("SELECT life_id FROM application where uuid = ?", appUUID.String())
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *applicationSuite) TestEnsureApplicationNotAliveNotExistsSuccess(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// We don't care if it's already gone.
	_, _, err := st.EnsureApplicationNotAlive(c.Context(), "some-application-uuid")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationSuite) TestApplicationRemovalNormalSuccess(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddUnitArg{})

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
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app", applicationservice.AddUnitArg{})

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
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createIAASApplication(c, svc, "some-app")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteApplication(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The application should be gone.
	exists, err := st.ApplicationExists(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *applicationSuite) TestDeleteCAASApplication(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	svc := s.setupService(c, factory)
	appUUID := s.createCAASApplication(c, svc, "some-app")

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteApplication(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// The application should be gone.
	exists, err := st.ApplicationExists(c.Context(), appUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
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

func (s *applicationSuite) checkApplicationDyingState(c *tc.C, appUUID coreapplication.ID) {
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
