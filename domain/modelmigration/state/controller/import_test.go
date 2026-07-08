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
