// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
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
	"github.com/juju/juju/internal/database"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/version"
)

type stateSuite struct {
	schematesting.ControllerSuite
	state *State

	internalBackendID   string
	kubernetesBackendID string
	vaultBackendID      string
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
}

func (s *stateSuite) setupController(c *gc.C) string {
	controllerUUID := uuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO controller_config (key, value)
			VALUES (?, ?)
		`, "controller-uuid", controllerUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	return controllerUUID
}

func (s *stateSuite) createModel(c *gc.C, modelType coremodel.ModelType) coremodel.UUID {
	return s.createModelWithName(c, modelType, "my-model")
}

func (s *stateSuite) createModelWithName(c *gc.C, modelType coremodel.ModelType, name string) coremodel.UUID {
	// Create internal controller secret backend.
	s.internalBackendID = uuid.MustNewUUID().String()
	result, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
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
		_, err = s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
			BackendIdentifier: secretbackend.BackendIdentifier{
				ID:   s.kubernetesBackendID,
				Name: kubernetes.BackendName,
			},
			BackendType: kubernetes.BackendType,
		})
		c.Assert(err, gc.IsNil)
	}

	s.vaultBackendID = uuid.MustNewUUID().String()
	result, err = s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
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
			Name:  name,
			Owner: userUUID,
			UUID:  modelUUID,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = modelSt.Finalise(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetModelSecretBackend(context.Background(), modelUUID, "my-backend")
	c.Assert(err, jc.ErrorIsNil)
	return modelUUID
}

func (s *stateSuite) assertSecretBackend(
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

func (s *stateSuite) TestCreateSecretBackendFailed(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
		Config: map[string]string{
			"key1": "",
		},
	})
	c.Assert(err, jc.ErrorIs, backenderrors.NotValid)
	c.Assert(err, gc.ErrorMatches, `secret backend not valid: empty config value for "my-backend"`)

	_, err = s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
		Config: map[string]string{
			"": "value1",
		},
	})
	c.Assert(err, jc.ErrorIs, backenderrors.NotValid)
	c.Assert(err, gc.ErrorMatches, `secret backend not valid: empty config key for "my-backend"`)
}

func (s *stateSuite) TestCreateSecretBackend(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	result, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, backendID)

	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)
}

func (s *stateSuite) TestCreateSecretBackendWithNoRotateNoConfig(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	result, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
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
}

func (s *stateSuite) TestupsertSecretBackendInvalidArg(c *gc.C) {
	_, err := s.state.upsertSecretBackend(context.Background(), nil, upsertSecretBackendParams{})
	c.Check(err, gc.ErrorMatches, `secret backend not valid: ID is missing`)

	backendID := uuid.MustNewUUID().String()
	_, err = s.state.upsertSecretBackend(context.Background(), nil, upsertSecretBackendParams{
		ID: backendID,
	})
	c.Check(err, gc.ErrorMatches, `secret backend not valid: name is missing`)

	_, err = s.state.upsertSecretBackend(context.Background(), nil, upsertSecretBackendParams{
		ID:   backendID,
		Name: "my-backend",
	})
	c.Check(err, gc.ErrorMatches, `secret backend not valid: type is missing`)

	_, err = s.state.upsertSecretBackend(context.Background(), nil, upsertSecretBackendParams{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
		Config: map[string]string{
			"key1": "",
		},
	})
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: empty config value for "my-backend"`)

	_, err = s.state.upsertSecretBackend(context.Background(), nil, upsertSecretBackendParams{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
		Config: map[string]string{
			"": "value1",
		},
	})
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, `secret backend not valid: empty config key for "my-backend"`)
}

func (s *stateSuite) TestUpdateSecretBackend(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)

	// Update by ID.
	nameChange := "my-backend-updated"
	_, err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		NewName: &nameChange,
	})
	c.Assert(err, gc.IsNil)

	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend-updated",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)

	// Update by name.
	newRotateInternal := 48 * time.Hour
	newNextRotateTime := time.Now().Add(newRotateInternal)
	_, err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			Name: "my-backend-updated",
		},
		TokenRotateInterval: &newRotateInternal,
		NextRotateTime:      &newNextRotateTime,
		Config: map[string]string{
			"key1": "value1-updated",
			"key3": "value3",
		},
	})
	c.Assert(err, gc.IsNil)

	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend-updated",
		BackendType:         "vault",
		TokenRotateInterval: &newRotateInternal,
		Config: map[string]string{
			"key1": "value1-updated",
			"key3": "value3",
		},
	}, &newNextRotateTime)
}

func (s *stateSuite) TestUpdateSecretBackendWithNoRotateNoConfig(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)

	nameChange := "my-backend-updated"
	_, err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		NewName: &nameChange,
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend-updated",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)
}

func (s *stateSuite) TestUpdateSecretBackendFailed(c *gc.C) {
	backendID1 := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID1,
			Name: "my-backend1",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
	})
	c.Check(err, gc.IsNil)

	backendID2 := uuid.MustNewUUID().String()
	_, err = s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID2,
			Name: "my-backend2",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
	})
	c.Check(err, gc.IsNil)

	nameChange := "my-backend1"
	_, err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID2,
		},
		NewName: &nameChange,
	})
	c.Check(err, jc.ErrorIs, backenderrors.AlreadyExists)
	c.Check(err, gc.ErrorMatches, `secret backend already exists: name "my-backend1"`)

	_, err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID2,
		},
		Config: map[string]string{
			"key1": "",
		},
	})
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, fmt.Sprintf(`secret backend not valid: empty config value for %q`, backendID2))

	_, err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID2,
		},
		Config: map[string]string{
			"": "value1",
		},
	})
	c.Check(err, jc.ErrorIs, backenderrors.NotValid)
	c.Check(err, gc.ErrorMatches, fmt.Sprintf(`secret backend not valid: empty config key for %q`, backendID2))
}

func (s *stateSuite) TestUpdateSecretBackendFailedForInternalBackend(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "controller",
	})
	c.Assert(err, gc.IsNil)

	newName := "my-backend-new"
	_, err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		NewName: &newName,
	})
	c.Assert(err, jc.ErrorIs, backenderrors.Forbidden)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`secret backend operation forbidden: %q is immutable`, backendID))
}

func (s *stateSuite) TestUpdateSecretBackendFailedForKubernetesBackend(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "kubernetes",
	})
	c.Assert(err, gc.IsNil)

	newName := "my-backend-new"
	_, err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		NewName: &newName,
	})
	c.Assert(err, jc.ErrorIs, backenderrors.Forbidden)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`secret backend operation forbidden: %q is immutable`, backendID))
}

func (s *stateSuite) TestDeleteSecretBackend(c *gc.C) {
	db := s.DB()
	modelUUID := s.createModel(c, coremodel.IAAS)

	row := db.QueryRow(`
SELECT secret_backend_uuid
FROM model_secret_backend
WHERE model_uuid = ?`[1:], modelUUID)
	var configuredBackendUUID string
	err := row.Scan(&configuredBackendUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(configuredBackendUUID, gc.Equals, s.vaultBackendID)

	err = s.state.DeleteSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: s.vaultBackendID}, false)
	c.Assert(err, gc.IsNil)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend
WHERE uuid = ?`[1:], s.vaultBackendID)
	var count int
	err = row.Scan(&count)
	c.Assert(err, gc.IsNil)
	c.Assert(count, gc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_config
WHERE backend_uuid = ?`[1:], s.vaultBackendID)
	err = row.Scan(&count)
	c.Assert(err, gc.IsNil)
	c.Assert(count, gc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_rotation
WHERE backend_uuid = ?`[1:], s.vaultBackendID)
	err = row.Scan(&count)
	c.Assert(err, gc.IsNil)
	c.Assert(count, gc.Equals, 0)

	var configuredBackend string
	row = db.QueryRow(`
SELECT sb.name
FROM model_secret_backend msb
JOIN secret_backend sb ON sb.uuid = msb.secret_backend_uuid
WHERE model_uuid = ?`[1:], modelUUID)
	err = row.Scan(&configuredBackend)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(configuredBackend, gc.Equals, "internal")
}

func (s *stateSuite) TestDeleteSecretBackendWithNoConfigNoNextRotationTime(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	}, nil)

	err = s.state.DeleteSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: backendID}, false)
	c.Assert(err, gc.IsNil)

	db := s.DB()
	row := db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend
WHERE uuid = ?`[1:], backendID)
	var count int
	err = row.Scan(&count)
	c.Assert(err, gc.IsNil)
	c.Assert(count, gc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_config
WHERE backend_uuid = ?`[1:], backendID)
	err = row.Scan(&count)
	c.Assert(err, gc.IsNil)
	c.Assert(count, gc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_rotation
WHERE backend_uuid = ?`[1:], backendID)
	err = row.Scan(&count)
	c.Assert(err, gc.IsNil)
	c.Assert(count, gc.Equals, 0)
}

func (s *stateSuite) TestDeleteSecretBackendFailedForInternalBackend(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "controller",
	})
	c.Assert(err, gc.IsNil)

	err = s.state.DeleteSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: backendID}, false)
	c.Assert(err, jc.ErrorIs, backenderrors.Forbidden)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`secret backend operation forbidden: %q is immutable`, backendID))
}

func (s *stateSuite) TestDeleteSecretBackendFailedForKubernetesBackend(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "kubernetes",
	})
	c.Assert(err, gc.IsNil)

	err = s.state.DeleteSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: backendID}, false)
	c.Assert(err, jc.ErrorIs, backenderrors.Forbidden)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`secret backend operation forbidden: %q is immutable`, backendID))
}

func (s *stateSuite) TestDeleteSecretBackendInUseFail(c *gc.C) {
	c.Skip("TODO: wait for secret DqLite support")
}

func (s *stateSuite) TestDeleteSecretBackendInUseWithForce(c *gc.C) {
	c.Skip("TODO: wait for secret DqLite support")
}

func (s *stateSuite) TestListSecretBackends(c *gc.C) {
	backendID1 := uuid.MustNewUUID().String()
	rotateInternal1 := 24 * time.Hour
	nextRotateTime1 := time.Now().Add(rotateInternal1)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID1,
			Name: "my-backend1",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		NextRotateTime:      &nextRotateTime1,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID1,
		Name:                "my-backend1",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime1)

	backendID2 := uuid.MustNewUUID().String()
	rotateInternal2 := 48 * time.Hour
	nextRotateTime2 := time.Now().Add(rotateInternal2)
	_, err = s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID2,
			Name: "my-backend2",
		},
		BackendType:         "kubernetes",
		TokenRotateInterval: &rotateInternal2,
		NextRotateTime:      &nextRotateTime2,
		Config: map[string]string{
			"key3": "value3",
			"key4": "value4",
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID2,
		Name:                "my-backend2",
		BackendType:         "kubernetes",
		TokenRotateInterval: &rotateInternal2,
		Config: map[string]string{
			"key3": "value3",
			"key4": "value4",
		},
	}, &nextRotateTime2)

	backends, err := s.state.ListSecretBackends(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(backends, gc.HasLen, 2)
	c.Assert(backends, jc.DeepEquals, []*secretbackend.SecretBackend{
		{
			ID:                  backendID1,
			Name:                "my-backend1",
			BackendType:         "vault",
			TokenRotateInterval: &rotateInternal1,
			Config: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			ID:                  backendID2,
			Name:                "my-backend2",
			BackendType:         "kubernetes",
			TokenRotateInterval: &rotateInternal2,
			Config: map[string]string{
				"key3": "value3",
				"key4": "value4",
			},
		},
	})
}

func (s *stateSuite) TestListSecretBackendsForModelIAAS(c *gc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)

	backendID1 := uuid.MustNewUUID().String()
	rotateInternal1 := 24 * time.Hour
	nextRotateTime1 := time.Now().Add(rotateInternal1)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID1,
			Name: "my-backend1",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		NextRotateTime:      &nextRotateTime1,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID1,
		Name:                "my-backend1",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime1)

	backendID2 := uuid.MustNewUUID().String()
	rotateInternal2 := 48 * time.Hour
	nextRotateTime2 := time.Now().Add(rotateInternal2)
	_, err = s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID2,
			Name: "my-backend2",
		},
		BackendType:         "kubernetes",
		TokenRotateInterval: &rotateInternal2,
		NextRotateTime:      &nextRotateTime2,
		Config: map[string]string{
			"key3": "value3",
			"key4": "value4",
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID2,
		Name:                "my-backend2",
		BackendType:         "kubernetes",
		TokenRotateInterval: &rotateInternal2,
		Config: map[string]string{
			"key3": "value3",
			"key4": "value4",
		},
	}, &nextRotateTime2)

	backends, err := s.state.ListSecretBackendsForModel(context.Background(), modelUUID, true)
	c.Assert(err, gc.IsNil)
	c.Assert(backends, gc.HasLen, 4)
	c.Assert(backends, jc.DeepEquals, []*secretbackend.SecretBackend{
		{
			ID:          s.internalBackendID,
			Name:        "internal",
			BackendType: "controller",
		},
		{
			ID:          s.vaultBackendID,
			Name:        "my-backend",
			BackendType: "vault",
			Config: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			ID:                  backendID1,
			Name:                "my-backend1",
			BackendType:         "vault",
			TokenRotateInterval: &rotateInternal1,
			Config: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			ID:                  backendID2,
			Name:                "my-backend2",
			BackendType:         "kubernetes",
			TokenRotateInterval: &rotateInternal2,
			Config: map[string]string{
				"key3": "value3",
				"key4": "value4",
			},
		},
	})
}

func (s *stateSuite) TestListSecretBackendsForModelCAAS(c *gc.C) {
	modelUUID := s.createModelWithName(c, coremodel.CAAS, "controller")

	backendID1 := uuid.MustNewUUID().String()
	rotateInternal1 := 24 * time.Hour
	nextRotateTime1 := time.Now().Add(rotateInternal1)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID1,
			Name: "my-backend1",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		NextRotateTime:      &nextRotateTime1,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID1,
		Name:                "my-backend1",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime1)

	backendID2 := uuid.MustNewUUID().String()
	rotateInternal2 := 48 * time.Hour
	nextRotateTime2 := time.Now().Add(rotateInternal2)
	_, err = s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID2,
			Name: "my-backend2",
		},
		BackendType:         "kubernetes",
		TokenRotateInterval: &rotateInternal2,
		NextRotateTime:      &nextRotateTime2,
		Config: map[string]string{
			"key3": "value3",
			"key4": "value4",
		},
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID2,
		Name:                "my-backend2",
		BackendType:         "kubernetes",
		TokenRotateInterval: &rotateInternal2,
		Config: map[string]string{
			"key3": "value3",
			"key4": "value4",
		},
	}, &nextRotateTime2)

	backends, err := s.state.ListSecretBackendsForModel(context.Background(), modelUUID, true)
	c.Assert(err, gc.IsNil)
	c.Assert(backends, gc.HasLen, 4)
	c.Assert(backends, jc.DeepEquals, []*secretbackend.SecretBackend{
		{
			ID:          s.kubernetesBackendID,
			Name:        "kubernetes",
			BackendType: "kubernetes",
		},
		{
			ID:          s.vaultBackendID,
			Name:        "my-backend",
			BackendType: "vault",
			Config: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			ID:                  backendID1,
			Name:                "my-backend1",
			BackendType:         "vault",
			TokenRotateInterval: &rotateInternal1,
			Config: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			ID:                  backendID2,
			Name:                "my-backend2",
			BackendType:         "kubernetes",
			TokenRotateInterval: &rotateInternal2,
			Config: map[string]string{
				"key3": "value3",
				"key4": "value4",
			},
		},
	})
}

func (s *stateSuite) TestListSecretBackendsEmpty(c *gc.C) {
	backends, err := s.state.ListSecretBackends(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(backends, gc.IsNil)
}

func (s *stateSuite) TestGetSecretBackendByName(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "vault",
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	}, nil)

	backend, err := s.state.GetSecretBackend(context.Background(), secretbackend.BackendIdentifier{Name: "my-backend"})
	c.Assert(err, gc.IsNil)
	c.Assert(backend, jc.DeepEquals, &secretbackend.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	})

	_, err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		TokenRotateInterval: &rotateInternal,
	})
	c.Assert(err, gc.IsNil)
	backend, err = s.state.GetSecretBackend(context.Background(), secretbackend.BackendIdentifier{Name: "my-backend"})
	c.Assert(err, gc.IsNil)
	c.Assert(backend, jc.DeepEquals, &secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	})

	_, err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	backend, err = s.state.GetSecretBackend(context.Background(), secretbackend.BackendIdentifier{Name: "my-backend"})
	c.Assert(err, gc.IsNil)
	c.Assert(backend, jc.DeepEquals, &secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
}

func (s *stateSuite) TestGetSecretBackendByNameNotFound(c *gc.C) {
	backend, err := s.state.GetSecretBackend(context.Background(), secretbackend.BackendIdentifier{Name: "my-backend"})
	c.Assert(err, jc.ErrorIs, backenderrors.NotFound)
	c.Assert(err, gc.ErrorMatches, `secret backend not found: "my-backend"`)
	c.Assert(backend, gc.IsNil)
}

func (s *stateSuite) TestGetSecretBackend(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "vault",
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	}, nil)

	backend, err := s.state.GetSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: backendID})
	c.Assert(err, gc.IsNil)
	c.Assert(backend, jc.DeepEquals, &secretbackend.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	})

	_, err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		TokenRotateInterval: &rotateInternal,
	})
	c.Assert(err, gc.IsNil)
	backend, err = s.state.GetSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: backendID})

	c.Assert(err, gc.IsNil)
	c.Assert(backend, jc.DeepEquals, &secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	})

	_, err = s.state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	backend, err = s.state.GetSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: backendID})
	c.Assert(err, gc.IsNil)
	c.Assert(backend, jc.DeepEquals, &secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
}

func (s *stateSuite) TestGetSecretBackendNotFound(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	backend, err := s.state.GetSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: backendID})
	c.Assert(err, jc.ErrorIs, backenderrors.NotFound)
	c.Assert(err, gc.ErrorMatches, `secret backend not found: "`+backendID+`"`)
	c.Assert(backend, gc.IsNil)
}

func (s *stateSuite) TestSecretBackendRotated(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	}, &nextRotateTime)

	newNextRotateTime := time.Now().Add(2 * rotateInternal)
	err = s.state.SecretBackendRotated(context.Background(), backendID, newNextRotateTime)
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	},
		// No ops because the new next rotation time is after the current one.
		&nextRotateTime,
	)

	newNextRotateTime = time.Now().Add(rotateInternal / 2)
	err = s.state.SecretBackendRotated(context.Background(), backendID, newNextRotateTime)
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	},
		// The next rotation time is updated.
		&newNextRotateTime,
	)

	nonExistBackendID := uuid.MustNewUUID().String()
	newNextRotateTime = time.Now().Add(rotateInternal / 4)
	err = s.state.SecretBackendRotated(context.Background(), nonExistBackendID, newNextRotateTime)
	c.Assert(err, jc.ErrorIs, backenderrors.NotFound)
	c.Assert(err, gc.ErrorMatches, `secret backend not found: "`+nonExistBackendID+`"`)
}

func (s *stateSuite) TestSetModelSecretBackend(c *gc.C) {
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
	result, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
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

func (s *stateSuite) TestSetModelSecretBackendBackendNotFound(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	err := s.state.SetModelSecretBackend(context.Background(), modelUUID, "my-backend")
	c.Assert(err, jc.ErrorIs, backenderrors.NotFound)
	c.Assert(err, gc.ErrorMatches, `secret backend not found: "my-backend"`)
}

func (s *stateSuite) TestSetModelSecretBackendModelNotFound(c *gc.C) {
	backendID := uuid.MustNewUUID().String()
	result, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
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

func (s *stateSuite) TestGetModelSecretBackendDetails(c *gc.C) {
	controllerUUID := s.setupController(c)
	modelUUID := s.createModel(c, coremodel.IAAS)

	result, err := s.state.GetModelSecretBackendDetails(context.Background(), modelUUID)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, secretbackend.ModelSecretBackend{
		ControllerUUID:  controllerUUID,
		ID:              modelUUID,
		Name:            "my-model",
		Type:            "iaas",
		SecretBackendID: s.vaultBackendID,
	})
}

func (s *stateSuite) TestGetModelCloudAndCredential(c *gc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)
	cld, cred, err := s.state.GetModelCloudAndCredential(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cld, jc.DeepEquals, cloud.Cloud{
		Name:           "my-cloud",
		Type:           "ec2",
		AuthTypes:      cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		CACertificates: []string{"my-ca-cert"},
		Regions: []cloud.Region{
			{Name: "my-region"},
		},
	})
	expectedCred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	expectedCred.Label = "foobar"
	c.Check(cred, jc.DeepEquals, expectedCred)
}

func (s *stateSuite) TestGetControllerModelCloudAndCredential(c *gc.C) {
	s.createModelWithName(c, coremodel.IAAS, "controller")
	cld, cred, err := s.state.GetControllerModelCloudAndCredential(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cld, jc.DeepEquals, cloud.Cloud{
		Name:           "my-cloud",
		Type:           "ec2",
		AuthTypes:      cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		CACertificates: []string{"my-ca-cert"},
		Regions: []cloud.Region{
			{Name: "my-region"},
		},
	})
	expectedCred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
		"foo": "foo val",
		"bar": "bar val",
	})
	expectedCred.Label = "foobar"
	c.Check(cred, jc.DeepEquals, expectedCred)
}

func (s *stateSuite) TestGetModelCloudAndCredentialNotFound(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	_, _, err := s.state.GetModelCloudAndCredential(context.Background(), modelUUID)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) TestInitialWatchStatement(c *gc.C) {
	table, q := s.state.InitialWatchStatement()
	c.Assert(table, gc.Equals, "secret_backend_rotation")
	c.Assert(q, gc.Equals, `SELECT backend_uuid FROM secret_backend_rotation`)
}

func (s *stateSuite) TestGetSecretBackendRotateChanges(c *gc.C) {
	backendID1 := uuid.MustNewUUID().String()
	rotateInternal1 := 24 * time.Hour
	nextRotateTime1 := time.Now().Add(rotateInternal1)
	_, err := s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID1,
			Name: "my-backend1",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		NextRotateTime:      &nextRotateTime1,
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID1,
		Name:                "my-backend1",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
	}, &nextRotateTime1)

	backendID2 := uuid.MustNewUUID().String()
	rotateInternal2 := 48 * time.Hour
	nextRotateTime2 := time.Now().Add(rotateInternal2)
	_, err = s.state.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID2,
			Name: "my-backend2",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal2,
		NextRotateTime:      &nextRotateTime2,
	})
	c.Assert(err, gc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID2,
		Name:                "my-backend2",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal2,
	}, &nextRotateTime2)

	changes, err := s.state.GetSecretBackendRotateChanges(context.Background(), backendID1, backendID2)
	c.Assert(err, gc.IsNil)
	c.Assert(changes, gc.HasLen, 2)
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Name < changes[j].Name
	})
	c.Assert(changes[0].ID, gc.Equals, backendID1)
	c.Assert(changes[0].Name, gc.Equals, "my-backend1")
	c.Assert(changes[0].NextTriggerTime.Equal(nextRotateTime1), jc.IsTrue)
	c.Assert(changes[1].ID, gc.Equals, backendID2)
	c.Assert(changes[1].Name, gc.Equals, "my-backend2")
	c.Assert(changes[1].NextTriggerTime.Equal(nextRotateTime2), jc.IsTrue)
}
