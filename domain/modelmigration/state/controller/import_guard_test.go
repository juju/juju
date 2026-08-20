// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	"github.com/juju/juju/internal/uuid"
)

func (s *stateSuite) TestImportTxnRunnerFactory(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	claimUUID := uuid.MustNewUUID().String()
	_, err := st.BeginImport(
		c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	factory := NewImportTxnRunnerFactory(
		s.TxnRunnerFactory(), s.modelUUID.String(), claimUUID,
	)
	runner, err := factory(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	called := false
	err = runner.Txn(c.Context(), func(context.Context, *sqlair.TX) error {
		called = true
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(called, tc.IsTrue)
	c.Check(runner.Dying(), tc.NotNil)
}

func (s *stateSuite) TestImportTxnRunnerFactoryRejectsWrongAttempt(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	claimUUID := uuid.MustNewUUID().String()
	_, err := st.BeginImport(
		c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	for _, test := range []struct {
		name      string
		modelUUID string
		claimUUID string
	}{
		{name: "wrong model", modelUUID: uuid.MustNewUUID().String(), claimUUID: claimUUID},
		{name: "wrong claim", modelUUID: s.modelUUID.String(), claimUUID: uuid.MustNewUUID().String()},
	} {
		c.Logf("case %q", test.name)
		factory := NewImportTxnRunnerFactory(
			s.TxnRunnerFactory(), test.modelUUID, test.claimUUID,
		)
		runner, err := factory(c.Context())
		c.Assert(err, tc.ErrorIsNil)

		called := false
		err = runner.Txn(c.Context(), func(context.Context, *sqlair.TX) error {
			called = true
			return nil
		})
		c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)
		c.Check(called, tc.IsFalse)
	}
}

func (s *stateSuite) TestImportTxnRunnerFactoryRejectsAborting(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	claimUUID := uuid.MustNewUUID().String()
	_, err := st.BeginImport(
		c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	factory := NewImportTxnRunnerFactory(
		s.TxnRunnerFactory(), s.modelUUID.String(), claimUUID,
	)
	runner, err := factory(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(c.Context(), `
UPDATE model_migration_import SET phase_type_id = 2 WHERE uuid = ?`, claimUUID)
	c.Assert(err, tc.ErrorIsNil)

	called := false
	err = runner.Txn(c.Context(), func(context.Context, *sqlair.TX) error {
		called = true
		return nil
	})
	c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotImporting)
	c.Check(called, tc.IsFalse)
}

// TestGuardedCompanionWritesRejectAborting drives ImportOfferPermissions
// and ImportExternalControllers through the import txn guard after the
// claim has left importing. The guard must reject both writes before
// any companion row is committed.
func (s *stateSuite) TestGuardedCompanionWritesRejectAborting(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	claimUUID := uuid.MustNewUUID().String()
	_, err := st.BeginImport(
		c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(c.Context(), `
UPDATE model_migration_import SET phase_type_id = 2 WHERE uuid = ?`, claimUUID)
	c.Assert(err, tc.ErrorIsNil)

	guarded := New(
		NewImportTxnRunnerFactory(
			s.TxnRunnerFactory(), s.modelUUID.String(), claimUUID,
		),
		clock.WallClock,
	)

	offerUUID := uuid.MustNewUUID().String()
	err = guarded.ImportOfferPermissions(
		c.Context(), s.modelUUID.String(), claimUUID, []string{offerUUID},
	)
	c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotImporting)

	var offerCount int
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import_offer WHERE offer_uuid = ?",
		offerUUID).Scan(&offerCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(offerCount, tc.Equals, 0)

	ref := externalController(
		uuid.MustNewUUID().String(), "third-party", "ca-cert",
		[]string{"10.0.0.5:17070"}, []string{uuid.MustNewUUID().String()},
	)
	err = guarded.ImportExternalControllers(
		c.Context(), s.modelUUID.String(), claimUUID,
		[]modelmigrationinternal.ExternalController{ref},
	)
	c.Check(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotImporting)

	var controllerCount int
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM external_controller WHERE uuid = ?",
		ref.UUID).Scan(&controllerCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerCount, tc.Equals, 0)
}

// TestGuardedCompanionWritesSucceedWhileImporting drives ImportOfferPermissions
// and ImportExternalControllers through the import txn guard while the claim
// is still importing. Both writes must commit, so the guard does not reject a
// legitimate in-phase write.
func (s *stateSuite) TestGuardedCompanionWritesSucceedWhileImporting(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)
	claimUUID := uuid.MustNewUUID().String()
	_, err := st.BeginImport(
		c.Context(), s.modelUUID.String(), claimUUID, uuid.MustNewUUID().String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	guarded := New(
		NewImportTxnRunnerFactory(
			s.TxnRunnerFactory(), s.modelUUID.String(), claimUUID,
		),
		clock.WallClock,
	)

	offerUUID := uuid.MustNewUUID().String()
	err = guarded.ImportOfferPermissions(
		c.Context(), s.modelUUID.String(), claimUUID, []string{offerUUID},
	)
	c.Assert(err, tc.ErrorIsNil)

	var offerCount int
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import_offer WHERE offer_uuid = ?",
		offerUUID).Scan(&offerCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(offerCount, tc.Equals, 1)

	ref := externalController(
		uuid.MustNewUUID().String(), "third-party", "ca-cert",
		[]string{"10.0.0.5:17070"}, []string{uuid.MustNewUUID().String()},
	)
	err = guarded.ImportExternalControllers(
		c.Context(), s.modelUUID.String(), claimUUID,
		[]modelmigrationinternal.ExternalController{ref},
	)
	c.Assert(err, tc.ErrorIsNil)

	var controllerCount int
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM external_controller WHERE uuid = ?",
		ref.UUID).Scan(&controllerCount)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerCount, tc.Equals, 1)
}
