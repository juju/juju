// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/export"
	migrationdomain "github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	migrationclaimstate "github.com/juju/juju/domain/modelmigration/state/controller"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/uuid"
)

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

	err := migration.AbortModelImport(c.Context(), deps, modelUUID)
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

	err := migration.AbortModelImport(c.Context(), deps, modelUUID)
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
}

// TestAbortModelImportIdempotent verifies aborting twice is safe and leaves the
// claim in the aborting phase.
func (s *controllerImportSuite) TestAbortModelImportIdempotent(c *tc.C) {
	modelUUID, _, deps := s.importWithContent(c)

	c.Assert(migration.AbortModelImport(c.Context(), deps, modelUUID), tc.ErrorIsNil)
	c.Assert(migration.AbortModelImport(c.Context(), deps, modelUUID), tc.ErrorIsNil)

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

	err := migration.AbortModelImport(c.Context(), deps, modelUUID)
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrAbortActivating)

	// The claim stays activating and the model identity row is untouched.
	claim, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.Phase, tc.Equals, migrationdomain.ImportPhaseActivating)
	c.Check(s.rowCount(c, "SELECT COUNT(*) FROM model WHERE uuid = ?", modelUUID.String()), tc.Equals, 1)
}
