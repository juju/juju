// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	userstate "github.com/juju/juju/domain/access/state"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelestate "github.com/juju/juju/domain/model/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/secretbackend"
	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	secretbackendstate "github.com/juju/juju/domain/secretbackend/state"
	"github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/version"
)

type controllerStateSuite struct {
	schematesting.ControllerSuite
	state              *ControllerState
	secretBackendState *secretbackendstate.State

	internalBackendID   string
	kubernetesBackendID string
	vaultBackendID      string
}

var _ = gc.Suite(&controllerStateSuite{})

func (s *controllerStateSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.state = NewControllerState(s.TxnRunnerFactory())
	s.secretBackendState = secretbackendstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
}

func (s *controllerStateSuite) setupController(c *gc.C) string {
	controllerUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO controller (uuid) VALUES (?)`, controllerUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	return controllerUUID
}

func (s *controllerStateSuite) createModel(c *gc.C, modelType coremodel.ModelType) coremodel.UUID {
	return s.createModelWithName(c, modelType, "my-model")
}

func (s *controllerStateSuite) createModelWithName(c *gc.C, modelType coremodel.ModelType, name string) coremodel.UUID {
	// Create internal controller secret backend.
	s.internalBackendID = uuid.MustNewUUID().String()
	result, err := s.secretBackendState.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   s.internalBackendID,
			Name: juju.BackendName,
		},
		BackendType: juju.BackendType,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, s.internalBackendID)

	if modelType == coremodel.CAAS {
		s.kubernetesBackendID = uuid.MustNewUUID().String()
		_, err = s.secretBackendState.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
			BackendIdentifier: secretbackend.BackendIdentifier{
				ID:   s.kubernetesBackendID,
				Name: kubernetes.BackendName,
			},
			BackendType: kubernetes.BackendType,
		})
		c.Assert(err, gc.IsNil)
	}

	s.vaultBackendID = uuid.MustNewUUID().String()
	result, err = s.secretBackendState.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   s.vaultBackendID,
			Name: "my-backend",
		},
		BackendType: "vault",
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, s.vaultBackendID)

	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:          s.vaultBackendID,
		Name:        "my-backend",
		BackendType: "vault",
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}, nil)

	// We need to generate a user in the database so that we can set the model
	// owner.
	userUUID, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	userName := "test-user"
	userState := userstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = userState.AddUser(
		context.Background(),
		userUUID,
		userName,
		userName,
		userUUID,
		// TODO (stickupkid): This should be AdminAccess, but we don't have
		// a model to set the user as the owner of.
		permission.ControllerForAccess(permission.SuperuserAccess),
	)
	c.Assert(err, jc.ErrorIsNil)

	cloudSt := cloudstate.NewState(s.TxnRunnerFactory())
	err = cloudSt.CreateCloud(context.Background(), userName, uuid.MustNewUUID().String(),
		cloud.Cloud{
			Name:           "my-cloud",
			Type:           "ec2",
			AuthTypes:      cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
			CACertificates: []string{"my-ca-cert"},
			Regions: []cloud.Region{
				{Name: "my-region"},
			},
		})
	c.Assert(err, jc.ErrorIsNil)

	cred := credential.CloudCredentialInfo{
		Label:    "foobar",
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}

	credSt := credentialstate.NewState(s.TxnRunnerFactory())
	_, err = credSt.UpsertCloudCredential(
		context.Background(), corecredential.Key{
			Cloud: "my-cloud",
			Owner: "test-user",
			Name:  "foobar",
		},
		cred,
	)
	c.Assert(err, jc.ErrorIsNil)

	modelUUID := modeltesting.GenModelUUID(c)
	modelSt := modelestate.NewState(s.TxnRunnerFactory())
	err = modelSt.Create(
		context.Background(),
		modelUUID,
		modelType,
		model.ModelCreationArgs{
			AgentVersion: version.Current,
			Cloud:        "my-cloud",
			CloudRegion:  "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: "test-user",
				Name:  "foobar",
			},
			Name:          name,
			Owner:         userUUID,
			SecretBackend: "my-backend",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = modelSt.Activate(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)

	return modelUUID
}

func (s *controllerStateSuite) assertSecretBackend(
	c *gc.C, expectedSecretBackend secretbackend.SecretBackend, expectedNextRotationTime *time.Time,
) {
	db := s.DB()
	row := db.QueryRow(`
SELECT uuid, name, bt.type, token_rotate_interval
FROM secret_backend sb
JOIN secret_backend_type bt ON sb.backend_type_id = bt.id
WHERE uuid = ?`[1:], expectedSecretBackend.ID)
	c.Assert(row.Err(), gc.IsNil)

	var (
		actual              secretbackend.SecretBackend
		tokenRotateInterval database.NullDuration
	)
	err := row.Scan(&actual.ID, &actual.Name, &actual.BackendType, &tokenRotateInterval)
	c.Assert(err, gc.IsNil)

	if tokenRotateInterval.Valid {
		actual.TokenRotateInterval = &tokenRotateInterval.Duration
	}
	if expectedNextRotationTime != nil {
		var actualNextRotationTime sql.NullTime
		row = db.QueryRow(`
SELECT next_rotation_time
FROM secret_backend_rotation
WHERE backend_uuid = ?`[1:], expectedSecretBackend.ID)
		c.Check(row.Err(), gc.IsNil)
		err = row.Scan(&actualNextRotationTime)
		c.Check(err, gc.IsNil)
		c.Check(actualNextRotationTime.Valid, jc.IsTrue)
		c.Check(actualNextRotationTime.Time.Equal(*expectedNextRotationTime), jc.IsTrue)
	} else {
		row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_rotation
WHERE backend_uuid = ?`[1:], expectedSecretBackend.ID)
		var count int
		err = row.Scan(&count)
		c.Check(err, gc.IsNil)
		c.Check(count, gc.Equals, 0)
	}

	if len(expectedSecretBackend.Config) > 0 {
		actual.Config = map[string]string{}
		rows, err := db.Query(`
SELECT name, content
FROM secret_backend_config
WHERE backend_uuid = ?`[1:], expectedSecretBackend.ID)
		c.Check(err, gc.IsNil)
		c.Check(rows.Err(), gc.IsNil)
		defer rows.Close()
		for rows.Next() {
			var k, v string
			err = rows.Scan(&k, &v)
			c.Check(err, gc.IsNil)
			actual.Config[k] = v
		}
	} else {
		var count int
		row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_config
WHERE backend_uuid = ?`[1:], expectedSecretBackend.ID)
		err = row.Scan(&count)
		c.Check(err, gc.IsNil)
		c.Check(count, gc.Equals, 0)
	}
	c.Assert(actual, jc.DeepEquals, expectedSecretBackend)
}

func (s *controllerStateSuite) TestSetModelSecretBackend(c *gc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)

	q := `
SELECT secret_backend_uuid
FROM model_secret_backend
WHERE model_uuid = ?`
	row := s.DB().QueryRow(q, modelUUID)
	var actualBackendID string
	err := row.Scan(&actualBackendID)
	c.Assert(err, gc.IsNil)
	c.Assert(actualBackendID, gc.Equals, s.vaultBackendID)

	anotherBackendID := uuid.MustNewUUID().String()
	result, err := s.secretBackendState.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   anotherBackendID,
			Name: "another-backend",
		},
		BackendType: "vault",
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, anotherBackendID)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:          anotherBackendID,
		Name:        "another-backend",
		BackendType: "vault",
	}, nil)

	err = s.state.SetModelSecretBackend(context.Background(), modelUUID, "another-backend")
	c.Assert(err, gc.IsNil)

	q = `
SELECT secret_backend_uuid
FROM model_secret_backend
WHERE model_uuid = ?`
	row = s.DB().QueryRow(q, modelUUID)
	err = row.Scan(&actualBackendID)
	c.Assert(err, gc.IsNil)
	c.Assert(actualBackendID, gc.Equals, anotherBackendID)
}

func (s *controllerStateSuite) TestSetModelSecretBackendBackendNotFound(c *gc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)
	err := s.state.SetModelSecretBackend(context.Background(), modelUUID, "non-existing-backen-id")
	c.Assert(err, jc.ErrorIs, backenderrors.NotFound)
	c.Assert(err, gc.ErrorMatches, `secret backend not found: "non-existing-backen-id"`)
}

func (s *controllerStateSuite) TestSetModelSecretBackendModelNotFound(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	result, err := s.secretBackendState.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "vault",
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, backendID)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	}, nil)

	modelUUID := modeltesting.GenModelUUID(c)
	err = s.state.SetModelSecretBackend(context.Background(), modelUUID, "my-backend")
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`model not found: model %q`, modelUUID))
}

func (s *controllerStateSuite) TestGetModelSecretBackendIaaSDefault(c *gc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)

	result, err := s.state.GetModelSecretBackend(context.Background(), modelUUID)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, `my-backend`)

	err = s.state.SetModelSecretBackend(context.Background(), modelUUID, "auto")
	c.Assert(err, gc.IsNil)

	detail, err := s.secretBackendState.GetModelSecretBackendDetails(context.Background(), modelUUID)
	c.Assert(err, gc.IsNil)
	c.Assert(detail.SecretBackendName, gc.Equals, provider.Internal)

	result, err = s.state.GetModelSecretBackend(context.Background(), modelUUID)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, provider.Auto)
}

func (s *controllerStateSuite) TestGetModelSecretBackendCaaSDefault(c *gc.C) {
	modelUUID := s.createModel(c, coremodel.CAAS)

	result, err := s.state.GetModelSecretBackend(context.Background(), modelUUID)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, `my-backend`)

	err = s.state.SetModelSecretBackend(context.Background(), modelUUID, "auto")
	c.Assert(err, gc.IsNil)

	detail, err := s.secretBackendState.GetModelSecretBackendDetails(context.Background(), modelUUID)
	c.Assert(err, gc.IsNil)
	c.Assert(detail.SecretBackendName, gc.Equals, kubernetes.BackendName)

	result, err = s.state.GetModelSecretBackend(context.Background(), modelUUID)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, provider.Auto)
}

func (s *controllerStateSuite) TestGetModelSecretBackendDetails(c *gc.C) {
	controllerUUID := s.setupController(c)
	modelUUID := s.createModel(c, coremodel.IAAS)

	result, err := s.secretBackendState.GetModelSecretBackendDetails(context.Background(), modelUUID)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, secretbackend.ModelSecretBackend{
		ControllerUUID:    controllerUUID,
		ID:                modelUUID,
		Name:              "my-model",
		Type:              "iaas",
		SecretBackendID:   s.vaultBackendID,
		SecretBackendName: "my-backend",
	})
}
