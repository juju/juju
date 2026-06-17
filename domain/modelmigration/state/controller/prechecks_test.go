// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/clock"
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/uuid"
)

// TestCloudExists verifies the cloud existence check against a known cloud and
// an unknown one.
func (s *stateSuite) TestCloudExists(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	exists, err := st.CloudExists(c.Context(), "my-cloud")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)

	exists, err = st.CloudExists(c.Context(), "missing-cloud")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

// TestCloudRegionExists verifies the region existence check, including that a
// region of a different cloud and an unknown cloud both report false.
func (s *stateSuite) TestCloudRegionExists(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	exists, err := st.CloudRegionExists(c.Context(), "my-cloud", "my-region")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)

	// "other-region" belongs to "other-cloud", not "my-cloud".
	exists, err = st.CloudRegionExists(c.Context(), "my-cloud", "other-region")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)

	exists, err = st.CloudRegionExists(c.Context(), "missing-cloud", "my-region")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

// TestIsUserDisabled verifies the user lookup reports existence and that an
// active, enabled user is not disabled, while an unknown user does not exist.
func (s *stateSuite) TestIsUserDisabled(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	disabled, exists, err := st.IsUserDisabled(c.Context(), s.userName.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
	c.Check(disabled, tc.IsFalse)

	_, exists, err = st.IsUserDisabled(c.Context(), "nonexistent-user")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

// TestIsUserDisabledDisabled verifies a disabled user is reported as disabled.
func (s *stateSuite) TestIsUserDisabledDisabled(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	_, err := s.DB().ExecContext(c.Context(),
		`INSERT INTO user_authentication (user_uuid, disabled) VALUES (?, TRUE)
		 ON CONFLICT(user_uuid) DO UPDATE SET disabled = TRUE`, s.userUUID)
	c.Assert(err, tc.ErrorIsNil)

	disabled, exists, err := st.IsUserDisabled(c.Context(), s.userName.Name())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
	c.Check(disabled, tc.IsTrue)
}

// TestGetCredentialRevoked verifies the credential lookup reports existence and
// that a live credential is not revoked, while an unknown one does not exist.
func (s *stateSuite) TestGetCredentialRevoked(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	revoked, exists, err := st.GetCredentialRevoked(c.Context(), "my-cloud", s.userName.Name(), "foobar")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
	c.Check(revoked, tc.IsFalse)

	_, exists, err = st.GetCredentialRevoked(c.Context(), "my-cloud", s.userName.Name(), "missing")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

// TestGetCredentialRevokedRevoked verifies a revoked credential is reported as
// revoked.
func (s *stateSuite) TestGetCredentialRevokedRevoked(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	_, err := s.DB().ExecContext(c.Context(),
		"UPDATE cloud_credential SET revoked = TRUE WHERE uuid = ?", s.credentialUUID)
	c.Assert(err, tc.ErrorIsNil)

	revoked, exists, err := st.GetCredentialRevoked(c.Context(), "my-cloud", s.userName.Name(), "foobar")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
	c.Check(revoked, tc.IsTrue)
}

// TestSecretBackendExists verifies the secret backend existence check.
func (s *stateSuite) TestSecretBackendExists(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	exists, err := st.SecretBackendExists(c.Context(), juju.BackendName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)

	exists, err = st.SecretBackendExists(c.Context(), "missing-backend")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

// TestModelExists verifies the model existence check against the suite's model
// and an unknown UUID.
func (s *stateSuite) TestModelExists(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	exists, err := st.ModelExists(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)

	exists, err = st.ModelExists(c.Context(), tc.Must(c, coremodel.NewUUID).String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

// TestModelNameInUse verifies the model name/qualifier collision check.
func (s *stateSuite) TestModelNameInUse(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	inUse, err := st.ModelNameInUse(c.Context(), "my-test-model", "prod")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(inUse, tc.IsTrue)

	// Same name, different qualifier.
	inUse, err = st.ModelNameInUse(c.Context(), "my-test-model", "staging")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(inUse, tc.IsFalse)

	// Different name, same qualifier.
	inUse, err = st.ModelNameInUse(c.Context(), "other-model", "prod")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(inUse, tc.IsFalse)
}

// TestGetImportClaim verifies the claim projection for each import phase.
func (s *stateSuite) TestGetImportClaim(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	migratingUUID := uuid.MustNewUUID().String()
	sourceMigrationUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES (?, ?, ?)",
		migratingUUID, s.modelUUID, sourceMigrationUUID)
	c.Assert(err, tc.ErrorIsNil)

	claim, err := st.GetImportClaim(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.SourceMigrationUUID, tc.Equals, sourceMigrationUUID)
	c.Check(claim.Phase, tc.Equals, modelmigration.ImportPhaseImporting)
	c.Check(claim.UpdatedAt.IsZero(), tc.IsFalse)

	for phaseID, phase := range map[int]modelmigration.ImportPhase{
		1: modelmigration.ImportPhaseActivating,
		2: modelmigration.ImportPhaseAborting,
	} {
		_, err = db.ExecContext(c.Context(),
			"UPDATE model_migration_import SET phase_type_id = ? WHERE uuid = ?",
			phaseID, migratingUUID)
		c.Assert(err, tc.ErrorIsNil)

		claim, err = st.GetImportClaim(c.Context(), s.modelUUID.String())
		c.Assert(err, tc.ErrorIsNil)
		c.Check(claim.Phase, tc.Equals, phase)
	}
}

// TestGetImportClaimNotFound verifies that a model without an import claim
// returns ErrImportNotFound.
func (s *stateSuite) TestGetImportClaimNotFound(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	_, err := st.GetImportClaim(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrImportNotFound)
}

// TestModelNamespaceExists verifies the namespace existence check for a model
// with a registered namespace and for an unknown model UUID.
func (s *stateSuite) TestModelNamespaceExists(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	// The suite's model is created through the model state and has a
	// registered namespace.
	exists, err := st.ModelNamespaceExists(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)

	exists, err = st.ModelNamespaceExists(c.Context(), tc.Must(c, coremodel.NewUUID).String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}
