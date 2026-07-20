// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/modelmigration/service"
	migrationstatecontroller "github.com/juju/juju/domain/modelmigration/state/controller"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

// exportWatcherSuite exercises the source-side export watchers end-to-end
// against the controller database. The export rows are inserted directly with
// SQL so this watcher suite does not depend on the (separate) source-side state
// write methods.
type exportWatcherSuite struct {
	changestreamtesting.ControllerSuite

	modelUUID string
}

func TestExportWatcherSuite(t *testing.T) {
	tc.Run(t, &exportWatcherSuite{})
}

func (s *exportWatcherSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.modelUUID = uuid.MustNewUUID().String()
}

// TestWatchForMigration asserts the existence watcher fires when an export
// migration starts and ends, but NOT on intermediate phase transitions.
func (s *exportWatcherSuite) TestWatchForMigration(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model_migration_export")
	svc := s.setupService(c, factory)

	s.AssertChangeStreamIdle(c, "before watcher start")
	w, err := svc.WatchForMigration(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))

	var migrationUUID string

	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Recording the export (migration start) fires the watcher.
	harness.AddTest(c, func(c *tc.C) {
		migrationUUID = s.insertExport(c)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// An intermediate phase change must NOT fire the existence watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.setExportPhase(c, migrationUUID, 2)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Ending the export by entering a terminal phase fires the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.endExport(c, migrationUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

// TestWatchMigrationPhase asserts the phase watcher fires on each phase row
// recorded for this model.
func (s *exportWatcherSuite) TestWatchMigrationPhase(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model_migration_export_phase")
	svc := s.setupService(c, factory)

	migrationUUID := s.insertExport(c)

	s.AssertChangeStreamIdle(c, "before watcher start")
	w, err := svc.WatchMigrationPhase(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))

	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Recording a phase fires the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.insertPhase(c, migrationUUID, 1)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// A subsequent phase fires the watcher again.
	harness.AddTest(c, func(c *tc.C) {
		s.insertPhase(c, migrationUUID, 2)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// A change on the export row itself (namespace 10019) must NOT fire the
	// phase watcher (namespace 10020): the two surfaces are deliberately
	// isolated, mirroring the converse assertion in TestWatchForMigration.
	harness.AddTest(c, func(c *tc.C) {
		s.endExport(c, migrationUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

// TestWatchMinionReports asserts the minion watcher fires when a minion report
// is recorded for the model's active migration.
func (s *exportWatcherSuite) TestWatchMinionReports(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model_migration_export_minion_sync")
	svc := s.setupService(c, factory)

	// The minion watcher resolves the active migration UUID, so the export must
	// already exist.
	migrationUUID := s.insertExport(c)

	s.AssertChangeStreamIdle(c, "before watcher start")
	w, err := svc.WatchMinionReports(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))

	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		s.insertMinionReport(c, migrationUUID, 1, "machine-0", true)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

// TestWatchImportClaims asserts that the target-side import claim watcher
// returns model UUIDs for its initial collection and all claim mutations.
func (s *exportWatcherSuite) TestWatchImportClaims(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, coredatabase.ControllerNS)
	svc := service.NewWatchableImportService(
		migrationstatecontroller.New(s.controllerDBFactory(), clock.WallClock),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		loggertesting.WrapCheckLog(c),
	)

	// A claim present before the watcher starts must be returned in its initial
	// collection.
	s.insertImportClaim(c, s.modelUUID)
	s.AssertChangeStreamIdle(c, "before watcher start")
	w, err := svc.WatchImportClaims(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))
	otherModelUUID := uuid.MustNewUUID().String()
	harness.AddTest(c, func(c *tc.C) {
		s.insertImportClaim(c, otherModelUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(otherModelUUID))
	})
	harness.AddTest(c, func(c *tc.C) {
		s.setImportClaimPhase(c, otherModelUUID, 1)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(otherModelUUID))
	})
	harness.AddTest(c, func(c *tc.C) {
		s.deleteImportClaim(c, otherModelUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(otherModelUUID))
	})
	harness.Run(c, []string{s.modelUUID})
}

// insertMinionReport records a minion sync row directly.
func (s *exportWatcherSuite) insertMinionReport(c *tc.C, migrationUUID string, phaseID int, entityKey string, success bool) {
	err := s.ControllerTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_export_minion_sync (migration_uuid, phase_id, entity_key, success, reported_at)
VALUES (?, ?, ?, ?, DATETIME('now', 'utc'))`,
			migrationUUID, phaseID, entityKey, success)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// insertExport inserts an active export row (and its target external controller)
// directly, returning the migration UUID.
func (s *exportWatcherSuite) insertExport(c *tc.C) string {
	migrationUUID := uuid.MustNewUUID().String()
	ctrlUUID := uuid.MustNewUUID().String()
	err := s.ControllerTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO external_controller (uuid, ca_cert) VALUES (?, 'ca-cert')", ctrlUUID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_export
    (uuid, model_uuid, target_controller_uuid, current_phase_id, updated_at, start_time)
VALUES (?, ?, ?, 1, DATETIME('now', 'utc'), DATETIME('now', 'utc'))`,
			migrationUUID, s.modelUUID, ctrlUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return migrationUUID
}

// setExportPhase updates the export's denormalised current phase only.
func (s *exportWatcherSuite) setExportPhase(c *tc.C, migrationUUID string, phaseID int) {
	err := s.ControllerTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"UPDATE model_migration_export SET current_phase_id = ?, updated_at = DATETIME('now', 'utc') WHERE uuid = ?",
			phaseID, migrationUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// endExport marks the export as terminal.
func (s *exportWatcherSuite) endExport(c *tc.C, migrationUUID string) {
	s.setExportPhase(c, migrationUUID, 8)
}

// insertPhase records a phase-history row carrying the model-scoped key.
func (s *exportWatcherSuite) insertPhase(c *tc.C, migrationUUID string, phaseID int) {
	err := s.ControllerTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_export_phase (migration_uuid, model_uuid, phase_id, changed_at)
VALUES (?, ?, ?, DATETIME('now', 'utc'))`,
			migrationUUID, s.modelUUID, phaseID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *exportWatcherSuite) insertImportClaim(c *tc.C, modelUUID string) {
	err := s.ControllerTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid)
VALUES (?, ?, 'source-migration-uuid')`, uuid.MustNewUUID().String(), modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *exportWatcherSuite) setImportClaimPhase(c *tc.C, modelUUID string, phaseID int) {
	err := s.ControllerTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE model_migration_import
SET    phase_type_id = ?, updated_at = DATETIME('now', 'utc')
WHERE  model_uuid = ?`, phaseID, modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *exportWatcherSuite) deleteImportClaim(c *tc.C, modelUUID string) {
	err := s.ControllerTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM model_migration_import WHERE model_uuid = ?", modelUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *exportWatcherSuite) controllerDBFactory() coredatabase.TxnRunnerFactory {
	return func(ctx context.Context) (coredatabase.TxnRunner, error) {
		return s.ControllerTxnRunner(), nil
	}
}

func (s *exportWatcherSuite) setupService(c *tc.C, factory domain.WatchableDBFactory) *service.Service {
	noopInstanceGetter := func(context.Context) (service.InstanceProvider, error) {
		return nil, nil
	}
	noopResourceGetter := func(context.Context) (service.ResourceProvider, error) {
		return nil, nil
	}

	return service.NewService(
		migrationstatecontroller.New(s.controllerDBFactory(), clock.WallClock),
		nil,
		s.modelUUID,
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		providertracker.ProviderGetter[service.InstanceProvider](noopInstanceGetter),
		providertracker.ProviderGetter[service.ResourceProvider](noopResourceGetter),
		loggertesting.WrapCheckLog(c),
	)
}
