// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/uuid"
)

// setImportPhase forces the claim for modelUUID to the given phase id
// (0=importing, 1=activating, 2=aborting) directly, for arranging test state.
func (s *stateSuite) setImportPhase(c *tc.C, modelUUID string, phaseID int) {
	_, err := s.DB().ExecContext(c.Context(),
		"UPDATE model_migration_import SET phase_type_id = ? WHERE model_uuid = ?",
		phaseID, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// insertClaim inserts an importing claim for modelUUID directly and returns its
// claim UUID.
func (s *stateSuite) insertClaim(c *tc.C, modelUUID string) string {
	claimUUID := uuid.MustNewUUID().String()
	_, err := s.DB().ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES (?, ?, ?)",
		claimUUID, modelUUID, uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)
	return claimUUID
}

func (s *stateSuite) importPhase(c *tc.C, modelUUID string) modelmigration.ImportPhase {
	claim, err := New(s.TxnRunnerFactory(), clock.WallClock).GetImportClaim(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	return claim.Phase
}

// TestSetImportPhaseAborting verifies an importing claim transitions to
// aborting.
func (s *stateSuite) TestSetImportPhaseAborting(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	_, err := st.BeginImport(c.Context(), s.modelUUID.String(), uuid.MustNewUUID().String(), uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)

	err = st.SetImportPhaseAborting(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.importPhase(c, s.modelUUID.String()), tc.Equals, modelmigration.ImportPhaseAborting)
}

// TestSetImportPhaseAbortingIdempotent verifies a second transition on an
// already-aborting claim is a no-op success.
func (s *stateSuite) TestSetImportPhaseAbortingIdempotent(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	_, err := st.BeginImport(c.Context(), s.modelUUID.String(), uuid.MustNewUUID().String(), uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(st.SetImportPhaseAborting(c.Context(), s.modelUUID.String()), tc.ErrorIsNil)
	c.Assert(st.SetImportPhaseAborting(c.Context(), s.modelUUID.String()), tc.ErrorIsNil)
	c.Check(s.importPhase(c, s.modelUUID.String()), tc.Equals, modelmigration.ImportPhaseAborting)
}

// TestSetImportPhaseAbortingFromActivating verifies that aborting is refused
// once activation has crossed the point of no return.
func (s *stateSuite) TestSetImportPhaseAbortingFromActivating(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	_, err := st.BeginImport(c.Context(), s.modelUUID.String(), uuid.MustNewUUID().String(), uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)
	s.setImportPhase(c, s.modelUUID.String(), 1) // activating

	err = st.SetImportPhaseAborting(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrAbortActivating)
	c.Check(s.importPhase(c, s.modelUUID.String()), tc.Equals, modelmigration.ImportPhaseActivating)
}

// TestSetImportPhaseAbortingNoClaim verifies a missing claim reports not found.
func (s *stateSuite) TestSetImportPhaseAbortingNoClaim(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	err := st.SetImportPhaseAborting(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)
}

// TestGetAllImportClaims verifies the reconciler scan returns every outstanding
// claim with its current phase.
func (s *stateSuite) TestGetAllImportClaims(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	claims, err := st.GetAllImportClaims(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claims, tc.HasLen, 0)

	sourceUUID := uuid.MustNewUUID().String()
	_, err = st.BeginImport(c.Context(), s.modelUUID.String(), uuid.MustNewUUID().String(), sourceUUID)
	c.Assert(err, tc.ErrorIsNil)

	claims, err = st.GetAllImportClaims(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(claims, tc.HasLen, 1)
	c.Check(claims[0].ModelUUID, tc.Equals, s.modelUUID.String())
	c.Check(claims[0].SourceMigrationUUID, tc.Equals, sourceUUID)
	c.Check(claims[0].Phase, tc.Equals, modelmigration.ImportPhaseImporting)
	c.Check(claims[0].UpdatedAt.IsZero(), tc.IsFalse)

	err = st.SetImportPhaseAborting(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	claims, err = st.GetAllImportClaims(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(claims, tc.HasLen, 1)
	c.Check(claims[0].Phase, tc.Equals, modelmigration.ImportPhaseAborting)
}

// TestIsImportNamespaceRegistered verifies the namespace_list predicate.
func (s *stateSuite) TestIsImportNamespaceRegistered(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	// s.modelUUID was created as a full model, so its namespace is registered.
	registered, err := st.IsImportNamespaceRegistered(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(registered, tc.IsTrue)

	// A model UUID that was never created has no namespace registration.
	registered, err = st.IsImportNamespaceRegistered(c.Context(), uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(registered, tc.IsFalse)
}

// TestFinalizeAbortedImportModelStillPresent verifies finalization refuses to
// delete the claim while the controller model identity row still exists, and
// leaves the claim untouched.
func (s *stateSuite) TestFinalizeAbortedImportModelStillPresent(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	// s.modelUUID has a live model row.
	s.insertClaim(c, s.modelUUID.String())
	s.setImportPhase(c, s.modelUUID.String(), 2) // aborting

	err := st.FinalizeAbortedImport(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrAbortNotFinalizable)

	// The claim is preserved for a later retry.
	c.Check(s.importPhase(c, s.modelUUID.String()), tc.Equals, modelmigration.ImportPhaseAborting)
}

// TestFinalizeAbortedImportWrongPhase verifies finalization refuses a claim
// that is not aborting.
func (s *stateSuite) TestFinalizeAbortedImportWrongPhase(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	s.insertClaim(c, s.modelUUID.String()) // importing

	err := st.FinalizeAbortedImport(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrPhaseTransitionInvalid)
}

// TestFinalizeAbortedImportIdempotent verifies finalization is a no-op when no
// claim exists.
func (s *stateSuite) TestFinalizeAbortedImportIdempotent(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	err := st.FinalizeAbortedImport(c.Context(), uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)
}

// TestStageAbortedModelDatabaseDeletion verifies staging deletes the namespace
// registration and records a model_database_deletion row for the undertaker.
func (s *stateSuite) TestStageAbortedModelDatabaseDeletion(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	db := s.DB()

	modelUUID := uuid.MustNewUUID().String()
	s.insertClaim(c, modelUUID)
	s.setImportPhase(c, modelUUID, 2) // aborting
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO namespace_list (namespace) VALUES (?)", modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = st.StageAbortedModelDatabaseDeletion(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// The namespace registration is gone and a deletion is staged.
	var nsCount, delCount int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM namespace_list WHERE namespace = ?", modelUUID).Scan(&nsCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(nsCount, tc.Equals, 0)
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_database_deletion WHERE namespace = ?", modelUUID).Scan(&delCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(delCount, tc.Equals, 1)

	// Staging again is idempotent (the deletion row is upserted).
	err = st.StageAbortedModelDatabaseDeletion(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_database_deletion WHERE namespace = ?", modelUUID).Scan(&delCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(delCount, tc.Equals, 1)
}

// TestStageAbortedModelDatabaseDeletionWrongPhase verifies staging refuses a
// claim that is not aborting, so a live model's database can never be staged.
func (s *stateSuite) TestStageAbortedModelDatabaseDeletionWrongPhase(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	s.insertClaim(c, s.modelUUID.String()) // importing

	err := st.StageAbortedModelDatabaseDeletion(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrPhaseTransitionInvalid)
}

// TestFinalizeAbortedImportDeletionPending verifies finalization refuses to
// release the claim while the model database deletion is still staged (the
// undertaker has not yet dropped the database).
func (s *stateSuite) TestFinalizeAbortedImportDeletionPending(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	db := s.DB()

	// A model UUID with no model row (compensation done) but a still-staged
	// database deletion.
	modelUUID := uuid.MustNewUUID().String()
	s.insertClaim(c, modelUUID)
	s.setImportPhase(c, modelUUID, 2) // aborting
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_database_deletion (namespace, created_at) VALUES (?, DATETIME('now'))", modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = st.FinalizeAbortedImport(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrAbortNotFinalizable)

	// The claim is preserved for a later retry.
	c.Check(s.importPhase(c, modelUUID), tc.Equals, modelmigration.ImportPhaseAborting)
}

// TestFinalizeAbortedImport verifies that once the model identity and namespace
// rows are gone and the database deletion has been completed by the undertaker
// (no staged row), finalization deletes the companion rows and the claim.
func (s *stateSuite) TestFinalizeAbortedImport(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	db := s.DB()

	// Use a model UUID with no model row (simulating that compensation has
	// already deleted the identity and namespace mapping) and no staged database
	// deletion (the undertaker has already dropped the database).
	modelUUID := uuid.MustNewUUID().String()
	claimUUID := s.insertClaim(c, modelUUID)
	s.setImportPhase(c, modelUUID, 2) // aborting

	// Companion rows that must be removed with the claim.
	offerUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import_offer (migration_uuid, offer_uuid) VALUES (?, ?)",
		claimUUID, offerUUID)
	c.Assert(err, tc.ErrorIsNil)
	extCtrlUUID := uuid.MustNewUUID().String()
	_, err = db.ExecContext(c.Context(),
		"INSERT INTO external_controller (uuid, alias, ca_cert) VALUES (?, 'other-ctrl', 'CACERT')",
		extCtrlUUID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = db.ExecContext(c.Context(),
		`INSERT INTO model_migration_import_external_controller_model
		 (migration_uuid, offerer_model_uuid, controller_uuid) VALUES (?, ?, ?)`,
		claimUUID, uuid.MustNewUUID().String(), extCtrlUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = st.FinalizeAbortedImport(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Claim and companions are all gone.
	tableKeyColumns := map[string]string{
		"model_migration_import":                           "model_uuid",
		"model_migration_import_offer":                     "migration_uuid",
		"model_migration_import_external_controller_model": "migration_uuid",
	}
	for table, keyColumn := range tableKeyColumns {
		key := modelUUID
		if keyColumn == "migration_uuid" {
			key = claimUUID
		}
		var count int
		err = db.QueryRowContext(c.Context(),
			"SELECT COUNT(*) FROM "+table+" WHERE "+keyColumn+" = ?", key).Scan(&count)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(count, tc.Equals, 0, tc.Commentf("table %q still has rows", table))
	}
}
