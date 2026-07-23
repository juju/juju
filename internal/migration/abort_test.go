// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/export"
	migrationdomain "github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	migrationclaimservice "github.com/juju/juju/domain/modelmigration/service"
	migrationclaimstate "github.com/juju/juju/domain/modelmigration/state/controller"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/uuid"
)

// claimService builds the modelmigration import service the abort tests inject
// into the abort driver, standing in for the apiserver-injected service.
func (s *controllerImportSuite) claimService(c *tc.C) *migrationclaimservice.Service {
	return migrationclaimservice.NewImportService(
		migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock),
		loggertesting.WrapCheckLog(c),
	)
}

// waitAbortClaim wraps the real import Service with a controllable
// model-database deletion watcher, so the finalize-wait tests can drive
// WaitAbortFinalized without a changestream. The watcher never fires; the wait's
// fallback re-check drives the finalize attempts.
type waitAbortClaim struct {
	*migrationclaimservice.Service
	changes chan struct{}
}

func (w waitAbortClaim) WatchModelDatabaseDeletion(
	context.Context, coremodel.UUID,
) (watcher.NotifyWatcher, error) {
	return watchertest.NewMockNotifyWatcher(w.changes), nil
}

// waitClaim builds the abortFinalizer the WaitAbortFinalized tests inject: the
// real import Service plus a stubbed deletion watcher.
func (s *controllerImportSuite) waitClaim(c *tc.C) waitAbortClaim {
	return waitAbortClaim{Service: s.claimService(c), changes: make(chan struct{})}
}

// importWithContent runs a full v8 controller-data import for a fresh model,
// including an offer permission, and returns the model UUID, the offer UUID it
// granted, and the deps used, for the abort tests to tear down.
func (s *controllerImportSuite) importWithContent(c *tc.C) (coremodel.UUID, string, migration.Deps) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	deps, _, _ := s.deps(c, modelUUID)

	offerUUID := uuid.MustNewUUID().String()
	info := s.baseControllerModelInfo(modelUUID)
	info.ModelCredential = &coremodelmigration.ModelCloudCredential{
		Cloud:      s.cloudName,
		Owner:      coreuser.AdminUserName.Name(),
		Name:       s.credentialName,
		AuthType:   "access-key",
		Attributes: map[string]string{"access-key": "val"},
	}
	info.Users = []coremodelmigration.ModelUser{
		{Name: coreuser.AdminUserName.Name()},
		{Name: "bob@external", DisplayName: "Bob", External: true},
	}
	info.Permissions = []coremodelmigration.ModelPermission{
		{ObjectType: "model", GrantOn: modelUUID.String(), SubjectName: "bob@external", Access: "read"},
		{ObjectType: "offer", GrantOn: offerUUID, SubjectName: "bob@external", Access: "consume"},
	}
	info.Leaders = []coremodelmigration.ApplicationLeadership{
		{Application: "myapp", Leader: "myapp/0"},
	}

	view := export.ProjectionView{AgentTargetVersion: jujuversion.Current}
	err := migration.ImportControllerModelInfo(c.Context(), deps, uuid.MustNewUUID().String(), info, view)
	c.Assert(err, tc.ErrorIsNil)
	return modelUUID, offerUUID, deps
}

func (s *controllerImportSuite) rowCount(c *tc.C, query string, args ...any) int {
	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, query, args...).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	return count
}

// TestAbortModelImportNoClaim verifies aborting a model with no import claim is
// a no-op success.
func (s *controllerImportSuite) TestAbortModelImportNoClaim(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	deps, _, _ := s.deps(c, modelUUID)

	err := migration.AbortModelImport(c.Context(), deps, s.claimService(c), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// TestAbortModelImportRemovesPartialImport verifies that aborting an importing
// model flips the claim to aborting, removes the controller-DB model identity
// and its model and offer permissions, but preserves the durable claim (which
// the reconciler finalizes later).
func (s *controllerImportSuite) TestAbortModelImportRemovesPartialImport(c *tc.C) {
	modelUUID, offerUUID, deps := s.importWithContent(c)

	// Sanity: the import wrote the model row and at least one offer-scoped
	// permission row, and the offer was recorded in the ledger.
	c.Assert(s.rowCount(c, "SELECT COUNT(*) FROM model WHERE uuid = ?", modelUUID.String()), tc.Equals, 1)
	c.Assert(s.rowCount(c, "SELECT COUNT(*) FROM permission WHERE grant_on = ?", offerUUID) > 0, tc.IsTrue)
	c.Assert(s.rowCount(c,
		"SELECT COUNT(*) FROM model_migration_import_offer WHERE offer_uuid = ?", offerUUID), tc.Equals, 1)

	err := migration.AbortModelImport(c.Context(), deps, s.claimService(c), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// The claim survives, now in the aborting phase.
	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	claim, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.Phase, tc.Equals, migrationdomain.ImportPhaseAborting)

	// The model identity row is gone.
	c.Check(s.rowCount(c, "SELECT COUNT(*) FROM model WHERE uuid = ?", modelUUID.String()), tc.Equals, 0)
	// The model-scoped and offer-scoped permission rows are gone. The offer row
	// is the one that regressed before the ledger-before-write fence fix: its
	// grant-on UUID is only discoverable from the offer ledger.
	c.Check(s.rowCount(c, "SELECT COUNT(*) FROM permission WHERE grant_on = ?", modelUUID.String()), tc.Equals, 0)
	c.Check(s.rowCount(c, "SELECT COUNT(*) FROM permission WHERE grant_on = ?", offerUUID), tc.Equals, 0)

	// The model database has been handed off to the undertaker: the namespace
	// registration is gone and a deletion is staged.
	c.Check(s.rowCount(c,
		"SELECT COUNT(*) FROM namespace_list WHERE namespace = ?", modelUUID.String()), tc.Equals, 0)
	c.Check(s.rowCount(c,
		"SELECT COUNT(*) FROM model_database_deletion WHERE namespace = ?", modelUUID.String()), tc.Equals, 1)
}

// TestAbortModelImportIdempotent verifies aborting twice is safe and leaves the
// claim in the aborting phase.
func (s *controllerImportSuite) TestAbortModelImportIdempotent(c *tc.C) {
	modelUUID, _, deps := s.importWithContent(c)

	c.Assert(migration.AbortModelImport(c.Context(), deps, s.claimService(c), modelUUID), tc.ErrorIsNil)
	c.Assert(migration.AbortModelImport(c.Context(), deps, s.claimService(c), modelUUID), tc.ErrorIsNil)

	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	claim, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.Phase, tc.Equals, migrationdomain.ImportPhaseAborting)
}

// TestAbortModelImportActivatingRefused verifies aborting a claim that has
// crossed the activation point of no return is a non-retryable conflict and
// leaves the model untouched.
func (s *controllerImportSuite) TestAbortModelImportActivatingRefused(c *tc.C) {
	modelUUID, _, deps := s.importWithContent(c)

	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	c.Assert(claimSt.SetImportPhaseActivating(c.Context(), modelUUID.String()), tc.ErrorIsNil)

	err := migration.AbortModelImport(c.Context(), deps, s.claimService(c), modelUUID)
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrAbortActivating)

	// The claim stays activating and the model identity row is untouched.
	claim, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.Phase, tc.Equals, migrationdomain.ImportPhaseActivating)
	c.Check(s.rowCount(c, "SELECT COUNT(*) FROM model WHERE uuid = ?", modelUUID.String()), tc.Equals, 1)
}

// TestAbortModelImportStandsAsideForLegacyRemoval verifies that when a
// v7/legacy abort has already marked the model dead and taken the claim's abort
// lock, the v8 abort driver does nothing: it must not re-drive compensation or
// stage a model-database deletion over the generic undertaker's teardown. This
// covers the abort reconciler picking up such an aborting claim.
func (s *controllerImportSuite) TestAbortModelImportStandsAsideForLegacyRemoval(c *tc.C) {
	modelUUID, _, deps := s.importWithContent(c)

	// Simulate domain/removal MarkMigratingModelAsDead: claim -> aborting and
	// the model marked dead, in the same spirit as the v7 Abort facade path.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"UPDATE model_migration_import SET phase_type_id = 2 WHERE model_uuid = ?", modelUUID.String()); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			"UPDATE model SET life_id = 2 WHERE uuid = ?", modelUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = migration.AbortModelImport(c.Context(), deps, s.claimService(c), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Nothing was touched: the model row, claim and namespace registration all
	// remain, and no model-database deletion was staged. The undertaker owns it.
	c.Check(s.rowCount(c, "SELECT COUNT(*) FROM model WHERE uuid = ?", modelUUID.String()), tc.Equals, 1)
	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	claim, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.Phase, tc.Equals, migrationdomain.ImportPhaseAborting)
	c.Check(s.rowCount(c,
		"SELECT COUNT(*) FROM namespace_list WHERE namespace = ?", modelUUID.String()), tc.Equals, 1)
	c.Check(s.rowCount(c,
		"SELECT COUNT(*) FROM model_database_deletion WHERE namespace = ?", modelUUID.String()), tc.Equals, 0)
}

// shortWait is a tiny finalize-wait budget for the synchronous-finalize tests,
// polling on the real wall clock so no background clock-advancing goroutine is
// needed against the live dqlite transactions.
var shortWait = migration.AbortFinalizeWait{Delay: time.Millisecond, MaxDuration: 50 * time.Millisecond}

// TestWaitAbortFinalized verifies that once the model database has been dropped
// (staged deletion cleared, standing in for the undertaker), the synchronous
// finalize deletes the claim so the model UUID is free.
func (s *controllerImportSuite) TestWaitAbortFinalized(c *tc.C) {
	modelUUID, _, deps := s.importWithContent(c)

	err := migration.AbortModelImport(c.Context(), deps, s.claimService(c), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Stand in for the undertaker's model-database deleter: clear the staged
	// deletion row so finalization's predicates pass.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"DELETE FROM model_database_deletion WHERE namespace = ?", modelUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = migration.WaitAbortFinalized(c.Context(), deps, s.waitClaim(c), modelUUID, shortWait)
	c.Assert(err, tc.ErrorIsNil)

	// The claim is gone, so the model UUID can be claimed by a fresh import.
	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	_, err = claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)
}

// TestWaitAbortFinalizedNoClaim verifies finalizing a model with no claim (a
// prior abort already finalized) is a no-op success.
func (s *controllerImportSuite) TestWaitAbortFinalizedNoClaim(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	deps, _, _ := s.deps(c, modelUUID)

	err := migration.WaitAbortFinalized(c.Context(), deps, s.waitClaim(c), modelUUID, shortWait)
	c.Assert(err, tc.ErrorIsNil)
}

// TestWaitAbortFinalizedPendingDropReturnsNil verifies that when the model
// database has not yet been dropped (staged deletion still present), the
// bounded wait exhausts its budget and returns nil, leaving the claim in the
// aborting phase for the reconciler to finalize later.
func (s *controllerImportSuite) TestWaitAbortFinalizedPendingDropReturnsNil(c *tc.C) {
	modelUUID, _, deps := s.importWithContent(c)

	err := migration.AbortModelImport(c.Context(), deps, s.claimService(c), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// The staged deletion row is left in place: the undertaker has not dropped
	// the database yet, so finalization cannot prove cleanup complete.
	err = migration.WaitAbortFinalized(c.Context(), deps, s.waitClaim(c), modelUUID, shortWait)
	c.Assert(err, tc.ErrorIsNil)

	// The claim survives in the aborting phase for the reconciler to complete.
	claimSt := migrationclaimstate.New(s.TxnRunnerFactory(), clock.WallClock)
	claim, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.Phase, tc.Equals, migrationdomain.ImportPhaseAborting)
}
