// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	corecloud "github.com/juju/juju/core/cloud"
	cloudtesting "github.com/juju/juju/core/cloud/testing"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	accessstate "github.com/juju/juju/domain/access/state"
	dbcloud "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/model/state/controller"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/secretbackend/bootstrap"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerModelSuite

	modelState *controller.State

	controllerModelUUID coremodel.UUID

	modelUUID coremodel.UUID
	userUUID  user.UUID
	userName  user.Name

	cloudUUID      corecloud.UUID
	credentialUUID corecredential.UUID
}

func TestMigrationSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ControllerModelSuite.SetUpTest(c)

	s.modelState = controller.NewState(s.TxnRunnerFactory())

	s.controllerModelUUID = tc.Must(c, coremodel.NewUUID)

	// We need to generate a user in the database so that we can set the model
	// owner.
	s.modelUUID = tc.Must(c, coremodel.NewUUID)
	s.userName = usertesting.GenNewName(c, "test-user")
	accessState := accessstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	s.userUUID = usertesting.GenUserUUID(c)
	err := accessState.AddUser(
		c.Context(),
		s.userUUID,
		s.userName,
		s.userName.Name(),
		false,
		s.userUUID,
	)
	c.Check(err, tc.ErrorIsNil)

	// We need to generate a cloud in the database so that we can set the model
	// cloud.
	cloudSt := dbcloud.NewState(s.TxnRunnerFactory())
	s.cloudUUID = cloudtesting.GenCloudUUID(c)
	err = cloudSt.CreateCloud(c.Context(), s.userName, s.cloudUUID.String(),
		cloud.Cloud{
			Name:      "my-cloud",
			Type:      "ec2",
			AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
			Regions: []cloud.Region{
				{
					Name: "my-region",
				},
			},
		})
	c.Assert(err, tc.ErrorIsNil)
	err = cloudSt.CreateCloud(c.Context(), s.userName, uuid.MustNewUUID().String(),
		cloud.Cloud{
			Name:      "other-cloud",
			Type:      "ec2",
			AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
			Regions: []cloud.Region{
				{
					Name: "other-region",
				},
			},
		})
	c.Assert(err, tc.ErrorIsNil)

	// We need to generate a cloud credential in the database so that we can set
	// the models cloud credential.
	cred := credential.CloudCredentialInfo{
		Label:    "foobar",
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}

	credSt := credentialstate.NewState(s.TxnRunnerFactory())
	err = credSt.UpsertCloudCredential(
		c.Context(), corecredential.Key{
			Cloud: "my-cloud",
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, tc.ErrorIsNil)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT uuid FROM cloud_credential WHERE owner_uuid = ? AND name = ? AND cloud_uuid = ?", s.userUUID, "foobar", s.cloudUUID).
			Scan(&s.credentialUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	err = credSt.UpsertCloudCredential(
		c.Context(), corecredential.Key{
			Cloud: "other-cloud",
			Owner: usertesting.GenNewName(c, "test-user"),
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, tc.ErrorIsNil)

	err = bootstrap.CreateDefaultBackends(coremodel.IAAS)(c.Context(), s.ControllerTxnRunner(), s.TxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	s.createControllerModel(c, s.controllerModelUUID, s.userUUID)
	s.createModel(c, s.modelUUID, s.userUUID)
}

// TestDeleteModelImportingStatusSuccess tests that clearing an existing
// model_migration_import entry succeeds and actually removes the entry from the
// database.
func (s *stateSuite) TestDeleteModelImportingStatusSuccess(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory())

	// Insert a model_migration_import entry.
	migratingUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid) VALUES (?, ?)",
		migratingUUID, s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Verify the entry exists.
	var count int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	// Clear the importing status.
	err = st.DeleteModelImportingStatus(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Verify the entry has been deleted.
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestDeleteModelImportingStatusNoEntry tests that clearing a non-existent
// model_migration_import entry succeeds without error (idempotent behavior).
func (s *stateSuite) TestDeleteModelImportingStatusNoEntry(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory())

	// Verify no entry exists.
	var count int
	err := db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)

	// Clear should succeed even when there's nothing to delete.
	err = st.DeleteModelImportingStatus(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Verify still no entries.
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestDeleteModelImportingStatusVerifyCorrectEntry tests that clearing
// deletes the correct entry and verifies by UUID.
func (s *stateSuite) TestDeleteModelImportingStatusVerifyCorrectEntry(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory())

	// Insert a model_migration_import entry with a specific UUID.
	migratingUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid) VALUES (?, ?)",
		migratingUUID, s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Verify we can query the specific entry by its UUID.
	var retrievedModelUUID string
	err = db.QueryRowContext(c.Context(),
		"SELECT model_uuid FROM model_migration_import WHERE uuid = ?",
		migratingUUID).Scan(&retrievedModelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedModelUUID, tc.Equals, s.modelUUID.String())

	// Clear the importing status.
	err = st.DeleteModelImportingStatus(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Verify the entry no longer exists.
	var count int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE uuid = ?",
		migratingUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestDeleteModelImportingStatusWrongModelUUID tests that clearing with a
// non-existent model UUID succeeds without error and doesn't affect other
// entries.
func (s *stateSuite) TestDeleteModelImportingStatusWrongModelUUID(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory())

	// Insert a model_migration_import entry.
	migratingUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid) VALUES (?, ?)",
		migratingUUID, s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Try to clear with a different (non-existent) model UUID.
	differentModelUUID := uuid.MustNewUUID().String()
	err = st.DeleteModelImportingStatus(c.Context(), differentModelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Verify the original entry still exists.
	var count int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}

// TestDeleteModelImportingStatusIdempotent tests that calling
// DeleteModelImportingStatus multiple times is safe and idempotent.
func (s *stateSuite) TestDeleteModelImportingStatusIdempotent(c *tc.C) {
	db := s.DB()
	st := New(s.TxnRunnerFactory())

	// Insert a model_migration_import entry.
	migratingUUID := uuid.MustNewUUID().String()
	_, err := db.ExecContext(c.Context(),
		"INSERT INTO model_migration_import (uuid, model_uuid) VALUES (?, ?)",
		migratingUUID, s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Clear the importing status multiple times.
	err = st.DeleteModelImportingStatus(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteModelImportingStatus(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	err = st.DeleteModelImportingStatus(c.Context(), s.modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	// Verify no entries exist.
	var count int
	err = db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM model_migration_import WHERE model_uuid = ?",
		s.modelUUID).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestSetAndGetControllerVersion tests that the controller version can be
// retrieved with no errors and can also be set (upgraded) with no errors.
func (s *stateSuite) TestSetAndGetControllerVersion(c *tc.C) {
	st := New(s.TxnRunnerFactory())

	// Check initial version is reported correctly.
	ver, err := st.GetControllerTargetVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.Equals, jujuversion.Current)
}

// createControllerModel creates a the database for use in tests.
func (s *stateSuite) createControllerModel(c *tc.C, controllerModelUUID coremodel.UUID, userUUID user.UUID) uuid.UUID {
	// Before we can create the model, we need to create a controller model.
	// This ensures that we
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err := controller.Create(c.Context(), preparer{}, tx, controllerModelUUID, coremodel.IAAS, model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Name:          "controller",
			Qualifier:     "prod",
			AdminUsers:    []user.UUID{userUUID},
			SecretBackend: juju.BackendName,
		})
		if err != nil {
			return err
		}

		activator := controller.GetActivator()
		return activator(ctx, preparer{}, tx, controllerModelUUID)
	})
	c.Assert(err, tc.ErrorIsNil)

	u, err := uuid.UUIDFromString(s.SeedControllerTable(c, controllerModelUUID))
	c.Assert(err, tc.ErrorIsNil)
	return u
}

// createModel creates a model in the database for use in tests.
func (s *stateSuite) createModel(c *tc.C, modelUUID coremodel.UUID, userUUID user.UUID) {
	s.createModelWithoutActivation(c, "my-test-model", modelUUID, userUUID)

	err := s.modelState.Activate(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *stateSuite) createModelWithoutActivation(
	c *tc.C, name string, modelUUID coremodel.UUID, creatorUUID user.UUID,
) {
	err := s.modelState.Create(
		c.Context(),
		modelUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Name:          name,
			Qualifier:     "prod",
			AdminUsers:    []user.UUID{creatorUUID},
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, tc.ErrorIsNil)
}

type preparer struct{}

func (p preparer) Prepare(query string, args ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, args...)
}
