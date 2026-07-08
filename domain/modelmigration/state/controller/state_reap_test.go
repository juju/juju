// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/migration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	"github.com/juju/juju/internal/uuid"
)

// reapMigration inserts an export for the suite model and walks its phases up
// to REAP, returning the migration spec.
func (s *stateSuite) reapMigration(c *tc.C, st *State) modelmigrationinternal.MigrationSpec {
	spec := s.newMigrationSpec()
	c.Assert(st.InsertExport(c.Context(), spec), tc.ErrorIsNil)
	for _, phase := range []migration.Phase{
		migration.IMPORT, migration.VALIDATION, migration.SUCCESS,
		migration.LOGTRANSFER, migration.REAP,
	} {
		c.Assert(st.SetPhase(c.Context(), spec.MigrationUUID, phase), tc.ErrorIsNil)
	}
	return spec
}

// seedOfferPermission inserts an offer-scoped permission row for the suite
// user and returns the offer UUID it grants on.
func (s *stateSuite) seedOfferPermission(c *tc.C) string {
	offerUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO permission (uuid, access_type_id, object_type_id, grant_on, grant_to)
VALUES (?,
        (SELECT id FROM permission_access_type WHERE type = 'admin'),
        (SELECT id FROM permission_object_type WHERE type = 'offer'),
        ?, ?)`,
			uuid.MustNewUUID().String(), offerUUID, s.userUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return offerUUID
}

// TestGetModelUsersForRedirect asserts the projection returns the model's
// permission holders with their user identity and access level.
func (s *stateSuite) TestGetModelUsersForRedirect(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	users, err := st.GetModelUsersForRedirect(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(users, tc.HasLen, 1)
	c.Check(users[0].UserUUID, tc.Equals, s.userUUID.String())
	c.Check(users[0].UserName, tc.Equals, s.userName.Name())
	c.Check(users[0].Access, tc.Equals, "admin")
}

// TestReapFullPath drives the whole source REAP state sequence: capture
// offers (twice, for replay), stage the redirect (twice, for replay), then the
// final purge transaction, verifying the purged rows, the completed redirect,
// the DONE export, and the scrubbed target auth.
func (s *stateSuite) TestReapFullPath(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	ctx := c.Context()

	spec := s.reapMigration(c, st)
	offerUUID := s.seedOfferPermission(c)

	// Capture offers twice: idempotent replay.
	c.Assert(st.CaptureExportOffers(ctx, spec.MigrationUUID, []string{offerUUID}), tc.ErrorIsNil)
	c.Assert(st.CaptureExportOffers(ctx, spec.MigrationUUID, []string{offerUUID}), tc.ErrorIsNil)

	users, err := st.GetModelUsersForRedirect(ctx, s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(users, tc.HasLen, 1)

	target := modelmigrationinternal.RedirectionTarget{
		ControllerUUID:  spec.TargetControllerUUID,
		ControllerAlias: "target-controller",
		Addresses:       []string{"10.0.0.1:17070", "10.0.0.2:17070"},
		CACert:          "ca-cert-data",
	}
	// Stage twice: replay must tolerate the existing redirect row and its
	// child user rows.
	c.Assert(st.StageModelRedirect(ctx, spec.MigrationUUID, s.modelUUID.String(), target, users), tc.ErrorIsNil)
	c.Assert(st.StageModelRedirect(ctx, spec.MigrationUUID, s.modelUUID.String(), target, users), tc.ErrorIsNil)

	// Staged-but-incomplete redirect is not active.
	_, err = st.GetRedirectForModel(ctx, s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrModelNotRedirected)

	// Final purge transaction.
	c.Assert(st.CompleteModelRedirectAndPurge(ctx, spec.MigrationUUID, s.modelUUID.String()), tc.ErrorIsNil)

	// Model row is gone.
	var count int
	c.Assert(db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM model WHERE uuid = ?", s.modelUUID).Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)

	// Offer and model permissions are gone.
	c.Assert(db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM permission WHERE grant_on IN (?, ?)",
		offerUUID, s.modelUUID).Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)

	// Export ended in DONE with a phase-history entry.
	var phaseID int
	c.Assert(db.QueryRowContext(ctx,
		"SELECT current_phase_id FROM model_migration_export WHERE uuid = ?",
		spec.MigrationUUID).Scan(&phaseID), tc.ErrorIsNil)
	doneID, err := migration.PhasePersistedID(migration.DONE)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(phaseID, tc.Equals, doneID)
	c.Assert(db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM model_migration_export_phase WHERE migration_uuid = ? AND phase_id = ?",
		spec.MigrationUUID, doneID).Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	// Target auth secrets are scrubbed; the row itself is retained.
	var user, token string
	c.Assert(db.QueryRowContext(ctx,
		"SELECT target_user, IFNULL(target_token, '') FROM model_migration_export_target_auth WHERE migration_uuid = ?",
		spec.MigrationUUID).Scan(&user, &token), tc.ErrorIsNil)
	c.Check(user, tc.Equals, "")
	c.Check(token, tc.Equals, "")

	// The redirect is now active and round-trips.
	got, err := st.GetRedirectForModel(ctx, s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.ControllerUUID, tc.Equals, spec.TargetControllerUUID)
	c.Check(got.ControllerAlias, tc.Equals, "target-controller")
	c.Check(got.Addresses, tc.DeepEquals, []string{"10.0.0.1:17070", "10.0.0.2:17070"})
	c.Check(got.CACert, tc.Equals, "ca-cert-data")

	// The captured redirect users survive the purge.
	gotUsers, err := st.GetRedirectUsers(ctx, s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotUsers, tc.HasLen, 1)
	c.Check(gotUsers[0].UserName, tc.Equals, s.userName.Name())
	c.Check(gotUsers[0].Access, tc.Equals, "admin")

	// No active export remains.
	_, err = st.GetActiveExport(ctx, s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrMigrationNotFound)
}

// TestCompleteModelRedirectAndPurgeWrongPhase asserts the purge transaction
// refuses to run unless the export is in REAP, leaving all rows untouched.
func (s *stateSuite) TestCompleteModelRedirectAndPurgeWrongPhase(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	ctx := c.Context()

	// Export freshly inserted: phase QUIESCE.
	spec := s.newMigrationSpec()
	c.Assert(st.InsertExport(ctx, spec), tc.ErrorIsNil)

	err := st.CompleteModelRedirectAndPurge(ctx, spec.MigrationUUID, s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrPhaseTransitionInvalid)

	// Nothing was purged.
	var count int
	c.Assert(db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM model WHERE uuid = ?", s.modelUUID).Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
	c.Assert(db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM permission WHERE grant_on = ?", s.modelUUID).Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}

// TestCompleteModelRedirectAndPurgeImportClaims asserts stale import claims
// are removed by the purge unless target-side abort cleanup owns them.
func (s *stateSuite) TestCompleteModelRedirectAndPurgeImportClaims(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	ctx := c.Context()

	spec := s.reapMigration(c, st)

	// Seed a stale import claim in the 'importing' phase.
	claimUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid, phase_type_id)
VALUES (?, ?, ?, (SELECT id FROM model_migration_import_phase_type WHERE type = 'importing'))`,
			claimUUID, s.modelUUID, uuid.MustNewUUID().String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	users, err := st.GetModelUsersForRedirect(ctx, s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	target := modelmigrationinternal.RedirectionTarget{
		ControllerUUID: spec.TargetControllerUUID,
		Addresses:      []string{"10.0.0.1:17070"},
		CACert:         "ca-cert-data",
	}
	c.Assert(st.StageModelRedirect(ctx, spec.MigrationUUID, s.modelUUID.String(), target, users), tc.ErrorIsNil)
	c.Assert(st.CompleteModelRedirectAndPurge(ctx, spec.MigrationUUID, s.modelUUID.String()), tc.ErrorIsNil)

	var count int
	c.Assert(db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?",
		s.modelUUID).Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestCompleteModelRedirectAndPurgeKeepsAbortingClaim asserts an import claim
// owned by target-side abort cleanup survives the purge.
func (s *stateSuite) TestCompleteModelRedirectAndPurgeKeepsAbortingClaim(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	ctx := c.Context()

	spec := s.reapMigration(c, st)

	claimUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid, phase_type_id)
VALUES (?, ?, ?, (SELECT id FROM model_migration_import_phase_type WHERE type = 'aborting'))`,
			claimUUID, s.modelUUID, uuid.MustNewUUID().String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	users, err := st.GetModelUsersForRedirect(ctx, s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	target := modelmigrationinternal.RedirectionTarget{
		ControllerUUID: spec.TargetControllerUUID,
		Addresses:      []string{"10.0.0.1:17070"},
		CACert:         "ca-cert-data",
	}
	c.Assert(st.StageModelRedirect(ctx, spec.MigrationUUID, s.modelUUID.String(), target, users), tc.ErrorIsNil)
	c.Assert(st.CompleteModelRedirectAndPurge(ctx, spec.MigrationUUID, s.modelUUID.String()), tc.ErrorIsNil)

	var count int
	c.Assert(db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM model_migration_import WHERE uuid = ?",
		claimUUID).Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}

// TestStageModelRedirectNoAddresses asserts a redirect target without
// addresses is rejected at staging time.
func (s *stateSuite) TestStageModelRedirectNoAddresses(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	err := st.StageModelRedirect(
		c.Context(), uuid.MustNewUUID().String(), s.modelUUID.String(),
		modelmigrationinternal.RedirectionTarget{
			ControllerUUID: uuid.MustNewUUID().String(),
			CACert:         "ca-cert-data",
		}, nil,
	)
	c.Assert(err, tc.ErrorMatches, `redirect target for model .* has no addresses`)
}
