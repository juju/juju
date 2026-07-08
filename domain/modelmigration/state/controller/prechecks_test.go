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

// TestCheckCloudRegion verifies the combined cloud/region lookup reports both
// parts without requiring separate state calls.
func (s *stateSuite) TestCheckCloudRegion(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	cloudExists, regionExists, err := st.CheckCloudRegion(c.Context(), "my-cloud", "my-region")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cloudExists, tc.IsTrue)
	c.Check(regionExists, tc.IsTrue)

	cloudExists, regionExists, err = st.CheckCloudRegion(c.Context(), "my-cloud", "other-region")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cloudExists, tc.IsTrue)
	c.Check(regionExists, tc.IsFalse)

	cloudExists, regionExists, err = st.CheckCloudRegion(c.Context(), "missing-cloud", "my-region")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cloudExists, tc.IsFalse)
	c.Check(regionExists, tc.IsFalse)

	cloudExists, regionExists, err = st.CheckCloudRegion(c.Context(), "my-cloud", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cloudExists, tc.IsTrue)
	c.Check(regionExists, tc.IsTrue)
}

// TestGetDisabledUsers verifies the batched user lookup reports only active
// disabled users and ignores missing users.
func (s *stateSuite) TestGetDisabledUsers(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	disabled, err := st.GetDisabledUsers(c.Context(), []string{s.userName.Name(), "nonexistent-user"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(disabled, tc.DeepEquals, []string{})

	_, err = s.DB().ExecContext(c.Context(),
		`INSERT INTO user_authentication (user_uuid, disabled) VALUES (?, TRUE)
		 ON CONFLICT(user_uuid) DO UPDATE SET disabled = TRUE`, s.userUUID)
	c.Assert(err, tc.ErrorIsNil)

	disabled, err = st.GetDisabledUsers(c.Context(), []string{s.userName.Name(), "nonexistent-user"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(disabled, tc.DeepEquals, []string{s.userName.Name()})
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

// TestCheckImportModelCollision verifies the combined model collision lookup
// reports import, UUID, namespace and name/qualifier collisions.
func (s *stateSuite) TestCheckImportModelCollision(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	collision, err := st.CheckImportModelCollision(
		c.Context(), s.modelUUID.String(), "my-test-model", "prod",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(collision.Importing, tc.IsFalse)
	c.Check(collision.ModelExists, tc.IsTrue)
	c.Check(collision.ModelNamespaceExists, tc.IsTrue)
	c.Check(collision.ModelNameExists, tc.IsTrue)

	_, err = db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid) VALUES (?, ?, ?)",
		uuid.MustNewUUID().String(), s.modelUUID, uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)

	collision, err = st.CheckImportModelCollision(
		c.Context(), s.modelUUID.String(), "my-test-model", "prod",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(collision.Importing, tc.IsTrue)

	collision, err = st.CheckImportModelCollision(
		c.Context(), tc.Must(c, coremodel.NewUUID).String(), "other-model", "prod",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(collision, tc.DeepEquals, modelmigration.ImportModelCollision{})
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

// TestGetConflictingCloudImageMetadata verifies a custom row matching by
// natural key with a different image id is reported, while identical and absent
// rows are not.
func (s *stateSuite) TestGetConflictingCloudImageMetadata(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	// Seed a custom target row for amd64 (architecture_id 0) with image
	// ami-target.
	_, err := s.DB().ExecContext(c.Context(),
		`INSERT INTO cloud_image_metadata
		 (uuid, created_at, source, stream, region, version, architecture_id, virt_type, root_storage_type, priority, image_id)
		 VALUES (?, DATETIME('now'), 'custom', 'released', 'us-east-1', '24.04', 0, 'hvm', 'ebs', 10, 'ami-target')`,
		uuid.MustNewUUID().String())
	c.Assert(err, tc.ErrorIsNil)

	conflict := modelmigration.ImportPrecheckImageMetadata{
		Stream: "released", Region: "us-east-1", Version: "24.04", Arch: "amd64",
		VirtType: "hvm", RootStorageType: "ebs", Source: "custom", ImageID: "ami-source",
	}
	identical := conflict
	identical.ImageID = "ami-target" // same image as target -> no conflict
	absent := conflict
	absent.Region = "eu-west-1" // no target row -> no conflict

	conflicts, err := st.GetConflictingCloudImageMetadata(c.Context(),
		[]modelmigration.ImportPrecheckImageMetadata{conflict, identical, absent})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(conflicts, tc.HasLen, 1)
	c.Check(conflicts[0].ImageID, tc.Equals, "ami-source")
	c.Check(conflicts[0].ExistingImageID, tc.Equals, "ami-target")
	c.Check(conflicts[0].Region, tc.Equals, "us-east-1")
}

// TestGetConflictingCloudImageMetadataEmpty verifies no rows yields no conflicts.
func (s *stateSuite) TestGetConflictingCloudImageMetadataEmpty(c *tc.C) {
	st := New(s.TxnRunnerFactory(), clock.WallClock)

	conflicts, err := st.GetConflictingCloudImageMetadata(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conflicts, tc.HasLen, 0)
}
