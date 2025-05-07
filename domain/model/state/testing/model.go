// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/model"
	modelstate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/uuid"
)

// CreateInternalSecretBackend creates the internal secret backend on a controller.
// This should only ever be used from within other state packages.
// This avoids the need for introducing cyclic imports with tests.
func CreateInternalSecretBackend(c *tc.C, runner database.TxnRunner) {
	backendUUID, err := corecredential.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	err = runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`
			INSERT INTO secret_backend (uuid, name, backend_type_id)
			VALUES (?, ?, ?)
			ON CONFLICT (name) DO NOTHING
		`, backendUUID.String(), juju.BackendName, 0)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

// CreateKubernetesSecretBackend creates the kubernetes secret backend on a controller.
// This should only ever be used from within other state packages.
// This avoids the need for introducing cyclic imports with tests.
func CreateKubernetesSecretBackend(c *tc.C, runner database.TxnRunner) {
	backendUUID, err := corecredential.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	err = runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(),
			`
			INSERT INTO secret_backend (uuid, name, backend_type_id)
			VALUES (?, ?, ?)
			ON CONFLICT (name) DO NOTHING
		`, backendUUID.String(), kubernetes.BackendName, 1)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

// CreateTestModel is a testing utility function for creating a basic model for
// a test to rely on. The created model will have it's uuid returned.
//
// This should only ever be used from within other state packages to establish a
// reference model. This avoids the need for introducing cyclic imports with
// tests.
func CreateTestModel(
	c *tc.C,
	txnRunner database.TxnRunnerFactory,
	name string,
) coremodel.UUID {
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	cloudUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	regionName := name + "-region"
	cloudRegionUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	credId, err := corecredential.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	userName := usertesting.GenNewName(c, "test-user"+name)
	runner, err := txnRunner()
	c.Assert(err, jc.ErrorIsNil)

	CreateInternalSecretBackend(c, runner)

	err = runner.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user (uuid, name, display_name, external, removed, created_by_uuid, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, userUUID.String(), userName.Name(), userName.Name(), false, false, userUUID, time.Now())
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO user_authentication (user_uuid, disabled)
			VALUES (?, ?)
		`, userUUID.String(), false)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO cloud (uuid, name, cloud_type_id, endpoint, skip_tls_verify)
			VALUES (?, ?, ?, "", true)
		`, cloudUUID.String(), name, 5)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO cloud_region (uuid, name, cloud_uuid)
			VALUES (?, ?, ?)
		`, cloudRegionUUID.String(), regionName, cloudUUID.String())
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO cloud_auth_type (cloud_uuid, auth_type_id)
			VALUES (?, 0), (?, 2)
		`, cloudUUID.String(), cloudUUID.String())
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO cloud_credential (uuid, cloud_uuid, auth_type_id, owner_uuid, name, revoked, invalid)
			VALUES (?, ?, ?, ?, "foobar", false, false)
		`, credId, cloudUUID.String(), 0, userUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	modelUUID := modeltesting.GenModelUUID(c)
	modelSt := modelstate.NewState(txnRunner)
	err = modelSt.Create(
		context.Background(),
		modelUUID,
		coremodel.IAAS,
		model.GlobalModelCreationArgs{
			Cloud:       name,
			CloudRegion: regionName,
			Credential: corecredential.Key{
				Cloud: name,
				Owner: userName,
				Name:  "foobar",
			},
			Name:          name,
			Owner:         userUUID,
			SecretBackend: juju.BackendName,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = modelSt.Activate(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)

	return modelUUID
}

// DeleteTestModel is responsible for cleaning up a testing mode previously
// created with [CreateTestModel].
func DeleteTestModel(c *tc.C, ctx context.Context, txnRunner database.TxnRunnerFactory, modelUUID coremodel.UUID) {
	modelSt := modelstate.NewState(txnRunner)
	err := modelSt.Delete(ctx, modelUUID)
	c.Assert(err, jc.ErrorIsNil)
}
