// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	dbcloud "github.com/juju/juju/domain/cloud/state"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/model"
	statecontroller "github.com/juju/juju/domain/model/state/controller"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/uuid"
)

type secretSuite struct {
	baseSuite
}

func TestSecretSuite(t *testing.T) {
	tc.Run(t, &secretSuite{})
}

func (s *secretSuite) TestRemoveSecretBackendReference(c *tc.C) {
	backendID := s.insertSecretBackend(c)

	modelUUID := s.uuid.String()
	secretRevisionID1 := uuid.MustNewUUID().String()
	secretRevisionID2 := uuid.MustNewUUID().String()

	s.insertSecretBackendReference(c, backendID, modelUUID, secretRevisionID1)
	s.insertSecretBackendReference(c, backendID, modelUUID, secretRevisionID2)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	assertSecretBackendReferenceCount(c, s.DB(), backendID, 2)

	err := st.RemoveSecretBackendReference(c.Context(), secretRevisionID1)
	c.Assert(err, tc.ErrorIsNil)
	assertSecretBackendReferenceCount(c, s.DB(), backendID, 1)

	err = st.RemoveSecretBackendReference(c.Context(), secretRevisionID2)
	c.Assert(err, tc.ErrorIsNil)
	assertSecretBackendReferenceCount(c, s.DB(), backendID, 0)
}

func (s *secretSuite) TestGetActiveModelSecretBackendK8sBuiltIn(c *tc.C) {
	controllerUUID := s.SeedControllerUUID(c)
	modelUUID := s.createK8sModel(c)

	ccState := controllerconfigstate.NewState(s.TxnRunnerFactory())
	err := ccState.UpdateControllerConfig(c.Context(), map[string]string{
		"controller-name": "test",
	}, nil)
	c.Assert(err, tc.ErrorIsNil)

	var k8sBackendUUID string
	err = s.DB().QueryRowContext(c.Context(),
		"SELECT uuid FROM secret_backend WHERE name = ?", kubernetes.BackendName,
	).Scan(&k8sBackendUUID)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	backendID, cfg, err := st.GetActiveModelSecretBackend(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(backendID, tc.Equals, k8sBackendUUID)
	c.Check(cfg, tc.DeepEquals, &provider.ModelBackendConfig{
		ControllerUUID: controllerUUID,
		ModelUUID:      modelUUID.String(),
		ModelName:      "my-k8s-model",
		BackendConfig: provider.BackendConfig{
			BackendType: kubernetes.BackendType,
			Config: map[string]any{
				"endpoint":  "https://k8s.example.com",
				"ca-certs":  []string{"k8s-ca-cert", "k8s-ca-cert-2"},
				"namespace": "my-k8s-model",
				"username":  "k8s-user",
				"password":  "k8s-password",
			},
		},
	})
}

func (s *secretSuite) createK8sModel(c *tc.C) coremodel.UUID {
	userName := usertesting.GenNewName(c, "test-user")

	cloudSt := dbcloud.NewState(s.TxnRunnerFactory())
	err := cloudSt.CreateCloud(c.Context(), userName, uuid.MustNewUUID().String(),
		cloud.Cloud{
			Name:           "k8s-cloud",
			Type:           "kubernetes",
			AuthTypes:      cloud.AuthTypes{cloud.UserPassAuthType, cloud.OAuth2AuthType},
			Endpoint:       "https://k8s.example.com",
			CACertificates: []string{"k8s-ca-cert", "k8s-ca-cert-2"},
			Regions:        []cloud.Region{{Name: "default"}},
		})
	c.Assert(err, tc.ErrorIsNil)

	credKey := corecredential.Key{
		Cloud: "k8s-cloud",
		Owner: userName,
		Name:  "k8s-cred",
	}
	credSt := credentialstate.NewState(s.TxnRunnerFactory())
	err = credSt.UpsertCloudCredential(c.Context(), credKey, credential.CloudCredentialInfo{
		Label:    "k8s-cred",
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"username": "k8s-user",
			"password": "k8s-password",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	modelUUID := tc.Must0(c, coremodel.NewUUID)
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err := statecontroller.Create(
			ctx, preparer{}, tx, modelUUID, coremodel.CAAS,
			model.GlobalModelCreationArgs{
				Cloud:         "k8s-cloud",
				CloudRegion:   "default",
				Credential:    credKey,
				Name:          "my-k8s-model",
				Qualifier:     "prod",
				AdminUsers:    []user.UUID{s.userUUID},
				SecretBackend: kubernetes.BackendName,
			},
		)
		if err != nil {
			return err
		}
		return statecontroller.GetActivator()(ctx, preparer{}, tx, modelUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	return modelUUID
}

// insertSecretBackend inserts a vault-type secret backend into the DB and
// returns its UUID.
func (s *secretSuite) insertSecretBackend(c *tc.C) string {
	backendUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO secret_backend (uuid, name, backend_type_id)
VALUES (?, ?, (SELECT id FROM secret_backend_type WHERE type = 'vault'))`,
			backendUUID, "test-vault-backend")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return backendUUID
}

// insertSecretBackendReference inserts a single row into
// secret_backend_reference for the given backendID, modelUUID and
// secretRevisionID.
func (s *secretSuite) insertSecretBackendReference(c *tc.C, backendID, modelUUID, secretRevisionID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO secret_backend_reference (secret_backend_uuid, model_uuid, secret_revision_uuid, secret_id)
VALUES (?, ?, ?, ?)`,
			backendID, modelUUID, secretRevisionID, secretRevisionID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// assertSecretBackendReferenceCount asserts that the number of references to the
// given secret backend in the DB matches expected.
func assertSecretBackendReferenceCount(c *tc.C, db *sql.DB, backendID string, expected int) {
	row := db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_reference
WHERE secret_backend_uuid = ?`, backendID)
	var refCount int
	err := row.Scan(&refCount)
	c.Assert(err, tc.IsNil)
	c.Assert(refCount, tc.Equals, expected)
}
