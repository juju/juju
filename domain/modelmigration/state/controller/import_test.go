// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	"github.com/juju/juju/internal/uuid"
)

// TestBeginImport verifies that a fresh claim is inserted with phase
// "importing" and the recorded source migration UUID, and that the inserted
// claim is returned.
func (s *stateSuite) TestBeginImport(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	sourceMigrationUUID := uuid.MustNewUUID().String()
	claimUUID := uuid.MustNewUUID().String()
	claim, err := st.BeginImport(c.Context(), s.modelUUID.String(), claimUUID, sourceMigrationUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.SourceMigrationUUID, tc.Equals, sourceMigrationUUID)
	c.Check(claim.Phase, tc.Equals, modelmigration.ImportPhaseImporting)

	persisted, err := st.GetImportClaim(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(persisted.SourceMigrationUUID, tc.Equals, sourceMigrationUUID)
	c.Check(persisted.Phase, tc.Equals, modelmigration.ImportPhaseImporting)
}

// TestBeginImportEmptySourceMigrationUUID verifies that an empty source
// migration UUID is rejected before any row is written.
func (s *stateSuite) TestBeginImportEmptySourceMigrationUUID(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	_, err := st.BeginImport(c.Context(), s.modelUUID.String(), uuid.MustNewUUID().String(), "")
	c.Assert(err, tc.ErrorMatches, ".*empty source migration uuid.*")

	_, err = st.GetImportClaim(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)
}

// TestBeginImportDuplicate verifies that a second claim for the same model
// returns ErrImportClaimExists together with the existing claim, and does not
// disturb it.
func (s *stateSuite) TestBeginImportDuplicate(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	firstSourceUUID := uuid.MustNewUUID().String()
	_, err := st.BeginImport(c.Context(), s.modelUUID.String(), uuid.MustNewUUID().String(), firstSourceUUID)
	c.Assert(err, tc.ErrorIsNil)

	existing, err := st.BeginImport(
		c.Context(), s.modelUUID.String(), uuid.MustNewUUID().String(), uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrImportClaimExists)
	c.Check(existing.SourceMigrationUUID, tc.Equals, firstSourceUUID)
	c.Check(existing.Phase, tc.Equals, modelmigration.ImportPhaseImporting)

	claim, err := st.GetImportClaim(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.SourceMigrationUUID, tc.Equals, firstSourceUUID)
}

// TestAssertImporting verifies that the assertion passes only while the
// claim's phase is "importing", and reports the correct sentinel otherwise.
func (s *stateSuite) TestAssertImporting(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	err := st.AssertImporting(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)

	claimUUID := uuid.MustNewUUID().String()
	_, err = st.BeginImport(c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)

	err = st.AssertImporting(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	for _, phaseID := range []int{1, 2} { // activating, aborting
		_, err = s.DB().ExecContext(c.Context(),
			"UPDATE model_migration_import SET phase_type_id = ? WHERE uuid = ?",
			phaseID, claimUUID)
		c.Assert(err, tc.ErrorIsNil)

		err = st.AssertImporting(c.Context(), s.modelUUID.String())
		c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotImporting)
	}
}

// TestImportOfferPermissions verifies that offer UUIDs are recorded against
// the claim, and that the write is rejected once the claim has left the
// importing phase.
func (s *stateSuite) TestImportOfferPermissions(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	claimUUID := uuid.MustNewUUID().String()
	_, err := st.BeginImport(c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)

	offerUUID1 := uuid.MustNewUUID().String()
	offerUUID2 := uuid.MustNewUUID().String()
	err = st.ImportOfferPermissions(c.Context(), s.modelUUID.String(), claimUUID, []string{offerUUID1, offerUUID2})
	c.Assert(err, tc.ErrorIsNil)

	var count int
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import_offer WHERE migration_uuid = ?",
		claimUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 2)

	// Calling with no offers is a no-op, not an error.
	err = st.ImportOfferPermissions(c.Context(), s.modelUUID.String(), claimUUID, nil)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(c.Context(),
		"UPDATE model_migration_import SET phase_type_id = 2 WHERE uuid = ?", claimUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = st.ImportOfferPermissions(c.Context(), s.modelUUID.String(), claimUUID, []string{uuid.MustNewUUID().String()})
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotImporting)
}

// TestEnsureExternalControllerExists verifies insert-if-absent,
// no-op-if-identical and fail-on-mismatch semantics.
func (s *stateSuite) TestEnsureExternalControllerExists(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	ctrlUUID := uuid.MustNewUUID().String()
	ref := externalController(ctrlUUID, "other", "ca-cert", []string{"10.0.0.1:17070", "10.0.0.2:17070"}, nil)

	err := st.EnsureExternalControllerExists(c.Context(), ref)
	c.Assert(err, tc.ErrorIsNil)

	var count int
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM external_controller WHERE uuid = ?", ctrlUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM external_controller_address WHERE controller_uuid = ?", ctrlUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 2)

	// Identical details: no-op, no error.
	err = st.EnsureExternalControllerExists(c.Context(), ref)
	c.Assert(err, tc.ErrorIsNil)

	// Different addresses: fail, do not overwrite.
	mismatched := externalController(ctrlUUID, "other", "ca-cert", []string{"10.0.0.9:17070"}, nil)
	err = st.EnsureExternalControllerExists(c.Context(), mismatched)
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrExternalControllerMismatch)

	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM external_controller_address WHERE controller_uuid = ? AND address = ?",
		ctrlUUID, "10.0.0.9:17070").Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// externalController builds a state-layer external controller reference with
// freshly generated address row UUIDs, mirroring the service-side
// translation.
func externalController(
	ctrlUUID, alias, caCert string, addrs, consumed []string,
) modelmigrationinternal.ExternalController {
	a := make([]modelmigrationinternal.ExternalControllerAddress, len(addrs))
	for i, addr := range addrs {
		a[i] = modelmigrationinternal.ExternalControllerAddress{
			UUID:    uuid.MustNewUUID().String(),
			Address: addr,
		}
	}
	return modelmigrationinternal.ExternalController{
		UUID:           ctrlUUID,
		Alias:          alias,
		CACert:         caCert,
		Addresses:      a,
		ConsumedModels: consumed,
	}
}

// TestImportExternalControllers verifies that third-party controllers and
// their consumed models are compared-or-inserted, and that the durable
// migration_uuid handoff rows are recorded.
func (s *stateSuite) TestImportExternalControllers(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	claimUUID := uuid.MustNewUUID().String()
	_, err := st.BeginImport(c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)

	offererModelUUID := uuid.MustNewUUID().String()
	ref := externalController(
		uuid.MustNewUUID().String(), "third-party", "ca-cert",
		[]string{"10.0.0.5:17070"}, []string{offererModelUUID})

	err = st.ImportExternalControllers(c.Context(), s.modelUUID.String(), claimUUID, []modelmigrationinternal.ExternalController{ref})
	c.Assert(err, tc.ErrorIsNil)

	var count int
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM external_controller WHERE uuid = ?", ref.UUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	var controllerUUID string
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT controller_uuid FROM external_model WHERE uuid = ?", offererModelUUID).Scan(&controllerUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerUUID, tc.Equals, ref.UUID)

	err = s.DB().QueryRowContext(c.Context(),
		`SELECT COUNT(*) FROM model_migration_import_external_controller_model
		 WHERE migration_uuid = ? AND offerer_model_uuid = ? AND controller_uuid = ?`,
		claimUUID, offererModelUUID, ref.UUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	// A second consumed model on a different controller for the same
	// offerer model UUID must fail rather than silently re-pointing it.
	otherRef := externalController(
		uuid.MustNewUUID().String(), "", "other-ca-cert", nil, []string{offererModelUUID})
	err = st.ImportExternalControllers(
		c.Context(), s.modelUUID.String(), claimUUID, []modelmigrationinternal.ExternalController{otherRef})
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrExternalControllerMismatch)
}

// phaseOf returns the current phase name of the model's import claim.
func (s *stateSuite) phaseOf(c *tc.C, modelUUID string) string {
	var phase string
	err := s.DB().QueryRowContext(c.Context(), `
SELECT mmipt.type
FROM   model_migration_import AS mmi
JOIN   model_migration_import_phase_type AS mmipt ON mmipt.id = mmi.phase_type_id
WHERE  mmi.model_uuid = ?`, modelUUID).Scan(&phase)
	c.Assert(err, tc.ErrorIsNil)
	return phase
}

// TestSetImportPhaseActivating verifies the importing→activating CAS
// transition, its idempotency once activating, and its phase guards.
func (s *stateSuite) TestSetImportPhaseActivating(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	// No claim: ErrImportNotFound.
	err := st.SetImportPhaseActivating(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)

	claimUUID := uuid.MustNewUUID().String()
	_, err = st.BeginImport(c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)

	// importing → activating.
	err = st.SetImportPhaseActivating(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.phaseOf(c, s.modelUUID.String()), tc.Equals, "activating")

	// Idempotent: already activating is a no-op success.
	err = st.SetImportPhaseActivating(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(s.phaseOf(c, s.modelUUID.String()), tc.Equals, "activating")

	// Aborting: refuse to activate.
	_, err = s.DB().ExecContext(c.Context(),
		"UPDATE model_migration_import SET phase_type_id = 2 WHERE uuid = ?", claimUUID)
	c.Assert(err, tc.ErrorIsNil)
	err = st.SetImportPhaseActivating(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrActivationAborting)
}

// TestDeleteActivatedImport verifies the claim and its FK-dependent companion
// rows are removed once activating, that a wrong phase is rejected, and that a
// missing claim is an idempotent success.
func (s *stateSuite) TestDeleteActivatedImport(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	// No claim: idempotent success.
	err := st.DeleteActivatedImport(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	claimUUID := uuid.MustNewUUID().String()
	_, err = st.BeginImport(c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)

	// Record companion rows while still importing.
	err = st.ImportOfferPermissions(
		c.Context(), s.modelUUID.String(), claimUUID, []string{uuid.MustNewUUID().String()})
	c.Assert(err, tc.ErrorIsNil)
	offererModelUUID := uuid.MustNewUUID().String()
	ref := externalController(
		uuid.MustNewUUID().String(), "third-party", "ca-cert",
		[]string{"10.0.0.5:17070"}, []string{offererModelUUID})
	err = st.ImportExternalControllers(
		c.Context(), s.modelUUID.String(), claimUUID, []modelmigrationinternal.ExternalController{ref})
	c.Assert(err, tc.ErrorIsNil)

	// Deleting while still importing is rejected without touching any rows.
	err = st.DeleteActivatedImport(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrPhaseTransitionInvalid)

	err = st.SetImportPhaseActivating(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteActivatedImport(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	var claims int
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE uuid = ?", claimUUID).Scan(&claims)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claims, tc.Equals, 0)
	var offers int
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import_offer WHERE migration_uuid = ?", claimUUID).Scan(&offers)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(offers, tc.Equals, 0)
	var ecm int
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import_external_controller_model WHERE migration_uuid = ?",
		claimUUID).Scan(&ecm)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ecm, tc.Equals, 0)

	// Second delete with no claim is an idempotent success.
	err = st.DeleteActivatedImport(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
}

// TestEnsureSourceControllerExists verifies the source controller, its
// addresses and its consumed (source-hosted) external_model rows are inserted,
// that a re-run is a no-op, and that a length mismatch is rejected.
func (s *stateSuite) TestEnsureSourceControllerExists(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	ctrlUUID := uuid.MustNewUUID().String()
	addrs := []string{"10.0.0.1:17070", "10.0.0.2:17070"}
	addrUUIDs := []string{uuid.MustNewUUID().String(), uuid.MustNewUUID().String()}
	consumed := []string{uuid.MustNewUUID().String(), uuid.MustNewUUID().String()}

	err := st.EnsureSourceControllerExists(
		c.Context(), ctrlUUID, "source", "ca-cert", addrs, addrUUIDs, consumed)
	c.Assert(err, tc.ErrorIsNil)

	var count int
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM external_controller WHERE uuid = ?", ctrlUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM external_controller_address WHERE controller_uuid = ?", ctrlUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 2)
	// Each consumed model is mapped to the source controller in external_model.
	for _, m := range consumed {
		var got string
		err = s.DB().QueryRowContext(c.Context(),
			"SELECT controller_uuid FROM external_model WHERE uuid = ?", m).Scan(&got)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(got, tc.Equals, ctrlUUID)
	}

	// Idempotent re-run with identical details.
	err = st.EnsureSourceControllerExists(
		c.Context(), ctrlUUID, "source", "ca-cert", addrs, addrUUIDs, consumed)
	c.Assert(err, tc.ErrorIsNil)

	// Length mismatch between addresses and address UUIDs is rejected.
	err = st.EnsureSourceControllerExists(
		c.Context(), ctrlUUID, "source", "ca-cert", addrs, []string{uuid.MustNewUUID().String()}, nil)
	c.Assert(err, tc.ErrorMatches, ".*length mismatch.*")
}

// TestExternalControllerModelsForImport verifies the third-party offerer-model
// mappings recorded at import are read back, and that a model without a claim
// returns an empty slice.
func (s *stateSuite) TestExternalControllerModelsForImport(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	// No claim: empty, not an error.
	got, err := st.ExternalControllerModelsForImport(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.HasLen, 0)

	claimUUID := uuid.MustNewUUID().String()
	_, err = st.BeginImport(c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)

	offererModelUUID := uuid.MustNewUUID().String()
	ctrlUUID := uuid.MustNewUUID().String()
	ref := externalController(ctrlUUID, "third-party", "ca-cert", []string{"10.0.0.5:17070"}, []string{offererModelUUID})
	err = st.ImportExternalControllers(
		c.Context(), s.modelUUID.String(), claimUUID, []modelmigrationinternal.ExternalController{ref})
	c.Assert(err, tc.ErrorIsNil)

	got, err = st.ExternalControllerModelsForImport(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 1)
	c.Check(got[0].OffererModelUUID, tc.Equals, offererModelUUID)
	c.Check(got[0].ControllerUUID, tc.Equals, ctrlUUID)
}
