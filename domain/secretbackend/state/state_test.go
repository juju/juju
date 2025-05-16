// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"fmt"
	"sort"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	userstate "github.com/juju/juju/domain/access/state"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
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
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	schematesting.ControllerSuite
	state *State

	controllerUUID string

	internalBackendID   string
	kubernetesBackendID string
	vaultBackendID      string
}

func TestStateSuite(t *stdtesting.T) { tc.Run(t, &stateSuite{}) }
func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.controllerUUID = s.SeedControllerUUID(c)
	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
}

func (s *stateSuite) createModel(c *tc.C, modelType coremodel.ModelType) coremodel.UUID {
	return s.createModelWithName(c, modelType, "my-model")
}

func (s *stateSuite) TestGetModelSecretBackendDetails(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)

	result, err := s.state.GetModelSecretBackendDetails(c.Context(), modelUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, secretbackend.ModelSecretBackend{
		ControllerUUID:    s.controllerUUID,
		ModelID:           modelUUID,
		ModelName:         "my-model",
		ModelType:         "iaas",
		SecretBackendID:   s.vaultBackendID,
		SecretBackendName: "my-backend",
	})
}

func (s *stateSuite) TestGetModelTypeIAAS(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)

	modelType, err := s.state.GetModelType(c.Context(), modelUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(modelType, tc.Equals, coremodel.IAAS)
}

func (s *stateSuite) TestGetModelTypeCAAS(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.CAAS)

	modelType, err := s.state.GetModelType(c.Context(), modelUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(modelType, tc.Equals, coremodel.CAAS)
}

func (s *stateSuite) TestGetInternalAndActiveBackendUUIDs(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)

	internalUUID, activeUUID, err := s.state.GetInternalAndActiveBackendUUIDs(c.Context(), modelUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(internalUUID, tc.Equals, s.internalBackendID)
	c.Assert(activeUUID, tc.Equals, s.vaultBackendID)
}

func (s *stateSuite) createModelWithName(c *tc.C, modelType coremodel.ModelType, name string) coremodel.UUID {
	// Create internal controller secret backend.
	s.internalBackendID = uuid.MustNewUUID().String()
	result, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   s.internalBackendID,
			Name: juju.BackendName,
		},
		BackendType: juju.BackendType,
	})
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, s.internalBackendID)

	if modelType == coremodel.CAAS {
		s.kubernetesBackendID = uuid.MustNewUUID().String()
		_, err = s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
			BackendIdentifier: secretbackend.BackendIdentifier{
				ID:   s.kubernetesBackendID,
				Name: kubernetes.BackendName,
			},
			BackendType: kubernetes.BackendType,
		})
		c.Assert(err, tc.IsNil)
	}

	s.vaultBackendID = uuid.MustNewUUID().String()
	result, err = s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
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
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, s.vaultBackendID)

	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:          s.vaultBackendID,
		Name:        "my-backend",
		BackendType: "vault",
		Config: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}, nil)

	// We need to generate a user in the database so that we can set the model
	// owner.
	userUUID, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	userName := usertesting.GenNewName(c, "test-user")
	userState := userstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err = userState.AddUserWithPermission(
		c.Context(),
		userUUID,
		userName,
		userName.Name(),
		false,
		userUUID,
		// TODO (stickupkid): This should be AdminAccess, but we don't have
		// a model to set the user as the owner of.
		permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.controllerUUID,
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	cloudSt := cloudstate.NewState(s.TxnRunnerFactory())
	err = cloudSt.CreateCloud(c.Context(), userName, uuid.MustNewUUID().String(),
		cloud.Cloud{
			Name:           "my-cloud",
			Type:           "ec2",
			AuthTypes:      cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
			Endpoint:       "https://my-cloud.com",
			CACertificates: []string{"my-ca-cert"},
			Regions: []cloud.Region{
				{Name: "my-region"},
			},
		})
	c.Assert(err, tc.ErrorIsNil)

	cred := credential.CloudCredentialInfo{
		Label:    "foobar",
		AuthType: string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{
			"Token": "token val",
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

	modelUUID := modeltesting.GenModelUUID(c)
	modelSt := modelestate.NewState(s.TxnRunnerFactory())
	err = modelSt.Create(
		c.Context(),
		modelUUID,
		modelType,
		model.GlobalModelCreationArgs{
			Cloud:       "my-cloud",
			CloudRegion: "my-region",
			Credential: corecredential.Key{
				Cloud: "my-cloud",
				Owner: usertesting.GenNewName(c, "test-user"),
				Name:  "foobar",
			},
			Name:          name,
			Owner:         userUUID,
			SecretBackend: "my-backend",
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	ccState := controllerconfigstate.NewState(s.TxnRunnerFactory())
	err = ccState.UpdateControllerConfig(c.Context(), map[string]string{
		"controller-name": "test",
	}, nil, func(map[string]string) error { return nil })
	c.Assert(err, tc.ErrorIsNil)

	err = modelSt.Activate(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	return modelUUID
}

func (s *stateSuite) assertSecretBackend(
	c *tc.C, expectedSecretBackend secretbackend.SecretBackend, expectedNextRotationTime *time.Time,
) {
	db := s.DB()
	row := db.QueryRow(`
SELECT uuid, name, bt.type, token_rotate_interval
FROM secret_backend sb
JOIN secret_backend_type bt ON sb.backend_type_id = bt.id
WHERE uuid = ?`[1:], expectedSecretBackend.ID)
	c.Assert(row.Err(), tc.IsNil)

	var (
		actual              secretbackend.SecretBackend
		tokenRotateInterval database.NullDuration
	)
	err := row.Scan(&actual.ID, &actual.Name, &actual.BackendType, &tokenRotateInterval)
	c.Assert(err, tc.IsNil)

	if tokenRotateInterval.Valid {
		actual.TokenRotateInterval = &tokenRotateInterval.Duration
	}
	if expectedNextRotationTime != nil {
		var actualNextRotationTime sql.NullTime
		row = db.QueryRow(`
SELECT next_rotation_time
FROM secret_backend_rotation
WHERE backend_uuid = ?`[1:], expectedSecretBackend.ID)
		c.Check(row.Err(), tc.IsNil)
		err = row.Scan(&actualNextRotationTime)
		c.Check(err, tc.IsNil)
		c.Check(actualNextRotationTime.Valid, tc.IsTrue)
		c.Check(actualNextRotationTime.Time.Equal(*expectedNextRotationTime), tc.IsTrue)
	} else {
		row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_rotation
WHERE backend_uuid = ?`[1:], expectedSecretBackend.ID)
		var count int
		err = row.Scan(&count)
		c.Check(err, tc.IsNil)
		c.Check(count, tc.Equals, 0)
	}

	if len(expectedSecretBackend.Config) > 0 {
		actual.Config = map[string]any{}
		rows, err := db.Query(`
SELECT name, content
FROM secret_backend_config
WHERE backend_uuid = ?`[1:], expectedSecretBackend.ID)
		c.Check(err, tc.IsNil)
		c.Check(rows.Err(), tc.IsNil)
		defer rows.Close()
		for rows.Next() {
			var k, v string
			err = rows.Scan(&k, &v)
			c.Check(err, tc.IsNil)
			actual.Config[k] = v
		}
	} else {
		var count int
		row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_config
WHERE backend_uuid = ?`[1:], expectedSecretBackend.ID)
		err = row.Scan(&count)
		c.Check(err, tc.IsNil)
		c.Check(count, tc.Equals, 0)
	}
	c.Assert(actual, tc.DeepEquals, expectedSecretBackend)
}

func (s *stateSuite) TestCreateSecretBackendFailed(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
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
	c.Assert(err, tc.ErrorIs, backenderrors.NotValid)
	c.Assert(err, tc.ErrorMatches, `secret backend not valid: empty config value for "my-backend"`)

	_, err = s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
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
	c.Assert(err, tc.ErrorIs, backenderrors.NotValid)
	c.Assert(err, tc.ErrorMatches, `secret backend not valid: empty config key for "my-backend"`)
}

func (s *stateSuite) TestCreateSecretBackend(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	result, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
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
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, backendID)

	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)
}

func (s *stateSuite) TestCreateSecretBackendWithNoRotateNoConfig(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	result, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "vault",
	})
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, backendID)

	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	}, nil)
}

func (s *stateSuite) TestUpsertSecretBackendInvalidArg(c *tc.C) {
	_, err := s.state.upsertSecretBackend(c.Context(), nil, upsertSecretBackendParams{})
	c.Check(err, tc.ErrorMatches, `secret backend not valid: ID is missing`)

	backendID := uuid.MustNewUUID().String()
	_, err = s.state.upsertSecretBackend(c.Context(), nil, upsertSecretBackendParams{
		ID: backendID,
	})
	c.Check(err, tc.ErrorMatches, `secret backend not valid: name is missing`)

	_, err = s.state.upsertSecretBackend(c.Context(), nil, upsertSecretBackendParams{
		ID:   backendID,
		Name: "my-backend",
	})
	c.Check(err, tc.ErrorMatches, `secret backend not valid: type is missing`)

	_, err = s.state.upsertSecretBackend(c.Context(), nil, upsertSecretBackendParams{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
		Config: map[string]string{
			"key1": "",
		},
	})
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: empty config value for "my-backend"`)

	_, err = s.state.upsertSecretBackend(c.Context(), nil, upsertSecretBackendParams{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
		Config: map[string]string{
			"": "value1",
		},
	})
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, `secret backend not valid: empty config key for "my-backend"`)
}

func (s *stateSuite) TestUpdateSecretBackend(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
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
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)

	// Update by ID.
	nameChange := "my-backend-updated"
	_, err = s.state.UpdateSecretBackend(c.Context(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		NewName: &nameChange,
	})
	c.Assert(err, tc.IsNil)

	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend-updated",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)

	// Update by name.
	newRotateInternal := 48 * time.Hour
	newNextRotateTime := time.Now().Add(newRotateInternal)
	_, err = s.state.UpdateSecretBackend(c.Context(), secretbackend.UpdateSecretBackendParams{
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
	c.Assert(err, tc.IsNil)

	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend-updated",
		BackendType:         "vault",
		TokenRotateInterval: &newRotateInternal,
		Config: map[string]any{
			"key1": "value1-updated",
			"key3": "value3",
		},
	}, &newNextRotateTime)
}

func (s *stateSuite) TestUpdateSecretBackendWithNoRotateNoConfig(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
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
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)

	nameChange := "my-backend-updated"
	_, err = s.state.UpdateSecretBackend(c.Context(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		NewName: &nameChange,
	})
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend-updated",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime)
}

func (s *stateSuite) TestUpdateSecretBackendFailed(c *tc.C) {
	backendID1 := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID1,
			Name: "my-backend1",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
	})
	c.Check(err, tc.IsNil)

	backendID2 := uuid.MustNewUUID().String()
	_, err = s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID2,
			Name: "my-backend2",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
	})
	c.Check(err, tc.IsNil)

	nameChange := "my-backend1"
	_, err = s.state.UpdateSecretBackend(c.Context(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID2,
		},
		NewName: &nameChange,
	})
	c.Check(err, tc.ErrorIs, backenderrors.AlreadyExists)
	c.Check(err, tc.ErrorMatches, `secret backend already exists: name "my-backend1"`)

	_, err = s.state.UpdateSecretBackend(c.Context(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID2,
		},
		Config: map[string]string{
			"key1": "",
		},
	})
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, fmt.Sprintf(`secret backend not valid: empty config value for %q`, backendID2))

	_, err = s.state.UpdateSecretBackend(c.Context(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID2,
		},
		Config: map[string]string{
			"": "value1",
		},
	})
	c.Check(err, tc.ErrorIs, backenderrors.NotValid)
	c.Check(err, tc.ErrorMatches, fmt.Sprintf(`secret backend not valid: empty config key for %q`, backendID2))
}

func (s *stateSuite) TestUpdateSecretBackendFailedForInternalBackend(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "controller",
	})
	c.Assert(err, tc.IsNil)

	newName := "my-backend-new"
	_, err = s.state.UpdateSecretBackend(c.Context(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		NewName: &newName,
	})
	c.Assert(err, tc.ErrorIs, backenderrors.Forbidden)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(`secret backend operation forbidden: %q is immutable`, backendID))
}

func (s *stateSuite) TestUpdateSecretBackendFailedForKubernetesBackend(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "kubernetes",
	})
	c.Assert(err, tc.IsNil)

	newName := "my-backend-new"
	_, err = s.state.UpdateSecretBackend(c.Context(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		NewName: &newName,
	})
	c.Assert(err, tc.ErrorIs, backenderrors.Forbidden)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(`secret backend operation forbidden: %q is immutable`, backendID))
}

func (s *stateSuite) TestDeleteSecretBackend(c *tc.C) {
	db := s.DB()
	modelUUID := s.createModel(c, coremodel.IAAS)

	row := db.QueryRow(`
SELECT secret_backend_uuid
FROM model_secret_backend
WHERE model_uuid = ?`[1:], modelUUID)
	var configuredBackendUUID string
	err := row.Scan(&configuredBackendUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(configuredBackendUUID, tc.Equals, s.vaultBackendID)

	err = s.state.DeleteSecretBackend(c.Context(), secretbackend.BackendIdentifier{ID: s.vaultBackendID}, false)
	c.Assert(err, tc.IsNil)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend
WHERE uuid = ?`[1:], s.vaultBackendID)
	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.IsNil)
	c.Assert(count, tc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_config
WHERE backend_uuid = ?`[1:], s.vaultBackendID)
	err = row.Scan(&count)
	c.Assert(err, tc.IsNil)
	c.Assert(count, tc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_rotation
WHERE backend_uuid = ?`[1:], s.vaultBackendID)
	err = row.Scan(&count)
	c.Assert(err, tc.IsNil)
	c.Assert(count, tc.Equals, 0)

	var configuredBackend string
	row = db.QueryRow(`
SELECT sb.name
FROM model_secret_backend msb
JOIN secret_backend sb ON sb.uuid = msb.secret_backend_uuid
WHERE model_uuid = ?`[1:], modelUUID)
	err = row.Scan(&configuredBackend)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(configuredBackend, tc.Equals, "internal")
}

func (s *stateSuite) TestDeleteSecretBackendWithNoConfigNoNextRotationTime(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	})
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	}, nil)

	err = s.state.DeleteSecretBackend(c.Context(), secretbackend.BackendIdentifier{ID: backendID}, false)
	c.Assert(err, tc.IsNil)

	db := s.DB()
	row := db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend
WHERE uuid = ?`[1:], backendID)
	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.IsNil)
	c.Assert(count, tc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_config
WHERE backend_uuid = ?`[1:], backendID)
	err = row.Scan(&count)
	c.Assert(err, tc.IsNil)
	c.Assert(count, tc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_rotation
WHERE backend_uuid = ?`[1:], backendID)
	err = row.Scan(&count)
	c.Assert(err, tc.IsNil)
	c.Assert(count, tc.Equals, 0)
}

func (s *stateSuite) TestDeleteSecretBackendFailedForInternalBackend(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "controller",
	})
	c.Assert(err, tc.IsNil)

	err = s.state.DeleteSecretBackend(c.Context(), secretbackend.BackendIdentifier{ID: backendID}, false)
	c.Assert(err, tc.ErrorIs, backenderrors.Forbidden)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(`secret backend operation forbidden: %q is immutable`, backendID))
}

func (s *stateSuite) TestDeleteSecretBackendFailedForKubernetesBackend(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "kubernetes",
	})
	c.Assert(err, tc.IsNil)

	err = s.state.DeleteSecretBackend(c.Context(), secretbackend.BackendIdentifier{ID: backendID}, false)
	c.Assert(err, tc.ErrorIs, backenderrors.Forbidden)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(`secret backend operation forbidden: %q is immutable`, backendID))
}

func (s *stateSuite) TestDeleteSecretBackendInUseFail(c *tc.C) {
	db := s.DB()
	modelUUID := s.createModel(c, coremodel.IAAS)

	row := db.QueryRow(`
SELECT secret_backend_uuid
FROM model_secret_backend
WHERE model_uuid = ?`[1:], modelUUID)
	var configuredBackendUUID string
	err := row.Scan(&configuredBackendUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(configuredBackendUUID, tc.Equals, s.vaultBackendID)

	secretRevisionID := uuid.MustNewUUID().String()
	_, err = s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.vaultBackendID}, modelUUID, secretRevisionID)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.DeleteSecretBackend(c.Context(), secretbackend.BackendIdentifier{ID: s.vaultBackendID}, false)
	c.Assert(err, tc.ErrorIs, backenderrors.Forbidden)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(`secret backend operation forbidden: %q is in use`, s.vaultBackendID))
}

func (s *stateSuite) TestDeleteSecretBackendInUseWithForce(c *tc.C) {
	db := s.DB()
	modelUUID := s.createModel(c, coremodel.IAAS)

	row := db.QueryRow(`
SELECT secret_backend_uuid
FROM model_secret_backend
WHERE model_uuid = ?`[1:], modelUUID)
	var configuredBackendUUID string
	err := row.Scan(&configuredBackendUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(configuredBackendUUID, tc.Equals, s.vaultBackendID)

	secretRevisionID := uuid.MustNewUUID().String()
	_, err = s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.vaultBackendID}, modelUUID, secretRevisionID)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.DeleteSecretBackend(c.Context(), secretbackend.BackendIdentifier{ID: s.vaultBackendID}, true)
	c.Assert(err, tc.ErrorIsNil)

	refCount, err := s.state.GetSecretBackendReferenceCount(c.Context(), s.vaultBackendID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(refCount, tc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend
WHERE uuid = ?`[1:], s.vaultBackendID)
	var count int
	err = row.Scan(&count)
	c.Assert(err, tc.IsNil)
	c.Assert(count, tc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_config
WHERE backend_uuid = ?`[1:], s.vaultBackendID)
	err = row.Scan(&count)
	c.Assert(err, tc.IsNil)
	c.Assert(count, tc.Equals, 0)

	row = db.QueryRow(`
SELECT COUNT(*)
FROM secret_backend_rotation
WHERE backend_uuid = ?`[1:], s.vaultBackendID)
	err = row.Scan(&count)
	c.Assert(err, tc.IsNil)
	c.Assert(count, tc.Equals, 0)

	var configuredBackend string
	row = db.QueryRow(`
SELECT sb.name
FROM model_secret_backend msb
JOIN secret_backend sb ON sb.uuid = msb.secret_backend_uuid
WHERE model_uuid = ?`[1:], modelUUID)
	err = row.Scan(&configuredBackend)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(configuredBackend, tc.Equals, "internal")
}

func (s *stateSuite) TestListSecretBackendsIAAS(c *tc.C) {
	backendID1 := uuid.MustNewUUID().String()
	rotateInternal1 := 24 * time.Hour
	nextRotateTime1 := time.Now().Add(rotateInternal1)
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID1,
			Name: "my-backend1",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		NextRotateTime:      &nextRotateTime1,
		Config: map[string]string{
			"key3": "value3",
			"key4": "value4",
		},
	})
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID1,
		Name:                "my-backend1",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		Config: map[string]any{
			"key3": "value3",
			"key4": "value4",
		},
	}, &nextRotateTime1)

	modelUUID := s.createModel(c, coremodel.IAAS)
	err = s.state.SetModelSecretBackend(c.Context(), modelUUID, "my-backend1")
	c.Assert(err, tc.IsNil)
	secrectRevisionID1 := uuid.MustNewUUID().String()
	_, err = s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: backendID1}, modelUUID, secrectRevisionID1)
	c.Assert(err, tc.IsNil)
	secrectRevisionID2 := uuid.MustNewUUID().String()
	_, err = s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: backendID1}, modelUUID, secrectRevisionID2)
	c.Assert(err, tc.IsNil)

	backends, err := s.state.ListSecretBackends(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(backends, tc.HasLen, 3)
	c.Assert(backends, tc.DeepEquals, []*secretbackend.SecretBackend{
		{
			ID:          s.internalBackendID,
			Name:        "internal",
			BackendType: "controller",
		},
		{
			ID:          s.vaultBackendID,
			Name:        "my-backend",
			BackendType: "vault",
			Config: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			ID:                  backendID1,
			Name:                "my-backend1",
			BackendType:         "vault",
			TokenRotateInterval: &rotateInternal1,
			Config: map[string]any{
				"key3": "value3",
				"key4": "value4",
			},
			NumSecrets: 2,
		},
	})
}

func (s *stateSuite) TestListSecretBackendsCAAS(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.CAAS)
	secrectRevisionID1 := uuid.MustNewUUID().String()
	_, err := s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.kubernetesBackendID}, modelUUID, secrectRevisionID1)
	c.Assert(err, tc.IsNil)
	secrectRevisionID2 := uuid.MustNewUUID().String()
	_, err = s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.kubernetesBackendID}, modelUUID, secrectRevisionID2)
	c.Assert(err, tc.IsNil)

	backendID2 := uuid.MustNewUUID().String()
	rotateInternal2 := 48 * time.Hour
	nextRotateTime2 := time.Now().Add(rotateInternal2)
	_, err = s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID2,
			Name: "my-backend2",
		},
		BackendType:         "kubernetes",
		TokenRotateInterval: &rotateInternal2,
		NextRotateTime:      &nextRotateTime2,
		Config: map[string]string{
			"key5": "value5",
			"key6": "value6",
		},
	})
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID2,
		Name:                "my-backend2",
		BackendType:         "kubernetes",
		TokenRotateInterval: &rotateInternal2,
		Config: map[string]any{
			"key5": "value5",
			"key6": "value6",
		},
	}, &nextRotateTime2)

	backends, err := s.state.ListSecretBackends(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(backends, tc.HasLen, 4)
	c.Assert(backends, tc.DeepEquals, []*secretbackend.SecretBackend{
		{
			ID:          s.kubernetesBackendID,
			Name:        "my-model-local",
			BackendType: kubernetes.BackendType,
			Config: map[string]any{
				"endpoint":  "https://my-cloud.com",
				"namespace": "my-model",
				"ca-certs":  []string{"my-ca-cert"},
				"token":     "token val",
			},
			NumSecrets: 2,
		},
		{
			ID:          s.internalBackendID,
			Name:        "internal",
			BackendType: "controller",
		},
		{
			ID:          s.vaultBackendID,
			Name:        "my-backend",
			BackendType: "vault",
			Config: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			ID:                  backendID2,
			Name:                "my-backend2",
			BackendType:         "kubernetes",
			TokenRotateInterval: &rotateInternal2,
			Config: map[string]any{
				"key5": "value5",
				"key6": "value6",
			},
		},
	})
}

func (s *stateSuite) TestListSecretBackendIDs(c *tc.C) {
	backendID1 := uuid.MustNewUUID().String()
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID1,
			Name: "my-backend1",
		},
		BackendType: "vault",
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, tc.IsNil)

	backendID2 := uuid.MustNewUUID().String()
	rotateInternal2 := 48 * time.Hour
	nextRotateTime2 := time.Now().Add(rotateInternal2)
	_, err = s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
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
	c.Assert(err, tc.IsNil)

	backends, err := s.state.ListSecretBackendIDs(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(backends, tc.HasLen, 2)
	c.Assert(backends, tc.SameContents, []string{backendID1, backendID2})
}

func (s *stateSuite) assertListSecretBackendsForModelIAAS(c *tc.C, includeEmpty bool) {
	modelUUID := s.createModel(c, coremodel.IAAS)
	err := s.state.SetModelSecretBackend(c.Context(), modelUUID, "my-backend")
	c.Assert(err, tc.IsNil)
	secrectRevisionID := uuid.MustNewUUID().String()
	_, err = s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.vaultBackendID}, modelUUID, secrectRevisionID)
	c.Assert(err, tc.IsNil)

	backendID1 := uuid.MustNewUUID().String()
	rotateInternal1 := 24 * time.Hour
	nextRotateTime1 := time.Now().Add(rotateInternal1)
	_, err = s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
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
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID1,
		Name:                "my-backend1",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		Config: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime1)

	backendID2 := uuid.MustNewUUID().String()
	rotateInternal2 := 48 * time.Hour
	nextRotateTime2 := time.Now().Add(rotateInternal2)
	_, err = s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
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
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID2,
		Name:                "my-backend2",
		BackendType:         "kubernetes",
		TokenRotateInterval: &rotateInternal2,
		Config: map[string]any{
			"key3": "value3",
			"key4": "value4",
		},
	}, &nextRotateTime2)

	backends, err := s.state.ListSecretBackendsForModel(c.Context(), modelUUID, includeEmpty)
	c.Assert(err, tc.IsNil)
	expected := []*secretbackend.SecretBackend{
		{
			ID:          s.internalBackendID,
			Name:        "internal",
			BackendType: "controller",
		},
		{
			ID:          s.vaultBackendID,
			Name:        "my-backend",
			BackendType: "vault",
			Config: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
		},
	}
	if includeEmpty {
		expected = append(expected,
			&secretbackend.SecretBackend{
				ID:                  backendID1,
				Name:                "my-backend1",
				BackendType:         "vault",
				TokenRotateInterval: &rotateInternal1,
				Config: map[string]any{
					"key1": "value1",
					"key2": "value2",
				},
			},
			&secretbackend.SecretBackend{
				ID:                  backendID2,
				Name:                "my-backend2",
				BackendType:         "kubernetes",
				TokenRotateInterval: &rotateInternal2,
				Config: map[string]any{
					"key3": "value3",
					"key4": "value4",
				},
			},
		)
	}
	c.Assert(backends, tc.DeepEquals, expected)
}

func (s *stateSuite) TestListSecretBackendsForModelIAASIncludeEmpty(c *tc.C) {
	s.assertListSecretBackendsForModelIAAS(c, true)
}

func (s *stateSuite) TestListSecretBackendsForModelIAASNotIncludeEmpty(c *tc.C) {
	s.assertListSecretBackendsForModelIAAS(c, false)
}

func (s *stateSuite) assertListSecretBackendsForModelCAAS(c *tc.C, includeEmpty bool) {
	modelUUID := s.createModelWithName(c, coremodel.CAAS, "controller")
	err := s.state.SetModelSecretBackend(c.Context(), modelUUID, "my-backend")
	c.Assert(err, tc.IsNil)
	secrectRevisionID := uuid.MustNewUUID().String()
	_, err = s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.vaultBackendID}, modelUUID, secrectRevisionID)
	c.Assert(err, tc.IsNil)

	backendID1 := uuid.MustNewUUID().String()
	rotateInternal1 := 24 * time.Hour
	nextRotateTime1 := time.Now().Add(rotateInternal1)
	_, err = s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
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
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID1,
		Name:                "my-backend1",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		Config: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}, &nextRotateTime1)

	backendID2 := uuid.MustNewUUID().String()
	rotateInternal2 := 48 * time.Hour
	nextRotateTime2 := time.Now().Add(rotateInternal2)
	_, err = s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
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
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID2,
		Name:                "my-backend2",
		BackendType:         "kubernetes",
		TokenRotateInterval: &rotateInternal2,
		Config: map[string]any{
			"key3": "value3",
			"key4": "value4",
		},
	}, &nextRotateTime2)

	backends, err := s.state.ListSecretBackendsForModel(c.Context(), modelUUID, includeEmpty)
	c.Assert(err, tc.IsNil)
	expected := []*secretbackend.SecretBackend{
		{
			ID:          s.kubernetesBackendID,
			Name:        "kubernetes",
			BackendType: "kubernetes",
			Config: map[string]any{
				"endpoint":  "https://my-cloud.com",
				"namespace": "controller-test",
				"ca-certs":  []string{"my-ca-cert"},
				"token":     "token val",
			},
		},
		{
			ID:          s.vaultBackendID,
			Name:        "my-backend",
			BackendType: "vault",
			Config: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
		},
	}
	if includeEmpty {
		expected = append(expected,
			&secretbackend.SecretBackend{
				ID:                  backendID1,
				Name:                "my-backend1",
				BackendType:         "vault",
				TokenRotateInterval: &rotateInternal1,
				Config: map[string]any{
					"key1": "value1",
					"key2": "value2",
				},
			},
			&secretbackend.SecretBackend{
				ID:                  backendID2,
				Name:                "my-backend2",
				BackendType:         "kubernetes",
				TokenRotateInterval: &rotateInternal2,
				Config: map[string]any{
					"key3": "value3",
					"key4": "value4",
				},
			},
		)
	}
	c.Assert(backends, tc.SameContents, expected)
}

func (s *stateSuite) TestListSecretBackendsForModelCAASIncludeEmpty(c *tc.C) {
	s.assertListSecretBackendsForModelCAAS(c, true)
}

func (s *stateSuite) TestListSecretBackendsForModelCAASNotIncludeEmpty(c *tc.C) {
	s.assertListSecretBackendsForModelCAAS(c, false)
}

func (s *stateSuite) TestListSecretBackendsEmpty(c *tc.C) {
	backends, err := s.state.ListSecretBackends(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(backends, tc.IsNil)
}

func (s *stateSuite) TestGetActiveModelSecretBackendIAASDefaultBackend(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)
	err := s.state.SetModelSecretBackend(c.Context(), modelUUID, provider.Internal)
	c.Assert(err, tc.IsNil)

	activeBackendID, backend, err := s.state.GetActiveModelSecretBackend(c.Context(), modelUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(activeBackendID, tc.Equals, s.internalBackendID)
	c.Assert(backend, tc.DeepEquals, &provider.ModelBackendConfig{
		ControllerUUID: s.controllerUUID,
		ModelUUID:      modelUUID.String(),
		ModelName:      "my-model",
		BackendConfig: provider.BackendConfig{
			BackendType: "controller",
		},
	})
}

func (s *stateSuite) TestGetActiveModelSecretBackendWithVaultBackend(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)
	err := s.state.SetModelSecretBackend(c.Context(), modelUUID, "my-backend")
	c.Assert(err, tc.IsNil)

	activeBackendID, backend, err := s.state.GetActiveModelSecretBackend(c.Context(), modelUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(activeBackendID, tc.Equals, s.vaultBackendID)
	c.Assert(backend, tc.DeepEquals, &provider.ModelBackendConfig{
		ControllerUUID: s.controllerUUID,
		ModelUUID:      modelUUID.String(),
		ModelName:      "my-model",
		BackendConfig: provider.BackendConfig{
			BackendType: "vault",
			Config: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
		},
	})
}

func (s *stateSuite) TestGetActiveModelSecretBackendCAASDefaultBackend(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.CAAS)
	err := s.state.SetModelSecretBackend(c.Context(), modelUUID, kubernetes.BackendName)
	c.Assert(err, tc.IsNil)

	activeBackendID, backend, err := s.state.GetActiveModelSecretBackend(c.Context(), modelUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(activeBackendID, tc.Equals, s.kubernetesBackendID)
	c.Assert(backend, tc.DeepEquals, &provider.ModelBackendConfig{
		ControllerUUID: s.controllerUUID,
		ModelUUID:      modelUUID.String(),
		ModelName:      "my-model",
		BackendConfig: provider.BackendConfig{
			BackendType: "kubernetes",
			Config: map[string]any{
				"endpoint":  "https://my-cloud.com",
				"namespace": "my-model",
				"ca-certs":  []string{"my-ca-cert"},
				"token":     "token val",
			},
		},
	})
}

func (s *stateSuite) TestGetActiveModelSecretBackendFailedWithModelNotFound(c *tc.C) {
	_, _, err := s.state.GetActiveModelSecretBackend(c.Context(), modeltesting.GenModelUUID(c))
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) TestGetSecretBackendByName(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "vault",
	})
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	}, nil)

	backend, err := s.state.GetSecretBackend(c.Context(), secretbackend.BackendIdentifier{Name: "my-backend"})
	c.Assert(err, tc.IsNil)
	c.Assert(backend, tc.DeepEquals, &secretbackend.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	})

	_, err = s.state.UpdateSecretBackend(c.Context(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		TokenRotateInterval: &rotateInternal,
	})
	c.Assert(err, tc.IsNil)
	backend, err = s.state.GetSecretBackend(c.Context(), secretbackend.BackendIdentifier{Name: "my-backend"})
	c.Assert(err, tc.IsNil)
	c.Assert(backend, tc.DeepEquals, &secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	})

	_, err = s.state.UpdateSecretBackend(c.Context(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, tc.IsNil)
	backend, err = s.state.GetSecretBackend(c.Context(), secretbackend.BackendIdentifier{Name: "my-backend"})
	c.Assert(err, tc.IsNil)
	c.Assert(backend, tc.DeepEquals, &secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	})
}

func (s *stateSuite) TestGetSecretBackendByNameNotFound(c *tc.C) {
	backend, err := s.state.GetSecretBackend(c.Context(), secretbackend.BackendIdentifier{Name: "my-backend"})
	c.Assert(err, tc.ErrorIs, backenderrors.NotFound)
	c.Assert(err, tc.ErrorMatches, `secret backend not found: "my-backend"`)
	c.Assert(backend, tc.IsNil)
}

func (s *stateSuite) TestGetSecretBackend(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "vault",
	})
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	}, nil)

	backend, err := s.state.GetSecretBackend(c.Context(), secretbackend.BackendIdentifier{ID: backendID})
	c.Assert(err, tc.IsNil)
	c.Assert(backend, tc.DeepEquals, &secretbackend.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	})

	_, err = s.state.UpdateSecretBackend(c.Context(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		TokenRotateInterval: &rotateInternal,
	})
	c.Assert(err, tc.IsNil)
	backend, err = s.state.GetSecretBackend(c.Context(), secretbackend.BackendIdentifier{ID: backendID})

	c.Assert(err, tc.IsNil)
	c.Assert(backend, tc.DeepEquals, &secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	})

	_, err = s.state.UpdateSecretBackend(c.Context(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID,
		},
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, tc.IsNil)
	backend, err = s.state.GetSecretBackend(c.Context(), secretbackend.BackendIdentifier{ID: backendID})
	c.Assert(err, tc.IsNil)
	c.Assert(backend, tc.DeepEquals, &secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		Config: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	})
}

func (s *stateSuite) TestGetSecretBackendNotFound(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	backend, err := s.state.GetSecretBackend(c.Context(), secretbackend.BackendIdentifier{ID: backendID})
	c.Assert(err, tc.ErrorIs, backenderrors.NotFound)
	c.Assert(err, tc.ErrorMatches, `secret backend not found: "`+backendID+`"`)
	c.Assert(backend, tc.IsNil)
}

func (s *stateSuite) TestSecretBackendRotated(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
		NextRotateTime:      &nextRotateTime,
	})
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID,
		Name:                "my-backend",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal,
	}, &nextRotateTime)

	newNextRotateTime := time.Now().Add(2 * rotateInternal)
	err = s.state.SecretBackendRotated(c.Context(), backendID, newNextRotateTime)
	c.Assert(err, tc.IsNil)
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
	err = s.state.SecretBackendRotated(c.Context(), backendID, newNextRotateTime)
	c.Assert(err, tc.IsNil)
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
	err = s.state.SecretBackendRotated(c.Context(), nonExistBackendID, newNextRotateTime)
	c.Assert(err, tc.ErrorIs, backenderrors.NotFound)
	c.Assert(err, tc.ErrorMatches, `secret backend not found: "`+nonExistBackendID+`"`)
}

func (s *stateSuite) TestSetModelSecretBackend(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)

	q := `
SELECT secret_backend_uuid
FROM model_secret_backend
WHERE model_uuid = ?`
	row := s.DB().QueryRow(q, modelUUID)
	var actualBackendID string
	err := row.Scan(&actualBackendID)
	c.Assert(err, tc.IsNil)
	c.Assert(actualBackendID, tc.Equals, s.vaultBackendID)

	anotherBackendID := uuid.MustNewUUID().String()
	result, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   anotherBackendID,
			Name: "another-backend",
		},
		BackendType: "vault",
	})
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, anotherBackendID)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:          anotherBackendID,
		Name:        "another-backend",
		BackendType: "vault",
	}, nil)

	err = s.state.SetModelSecretBackend(c.Context(), modelUUID, "another-backend")
	c.Assert(err, tc.IsNil)

	q = `
SELECT secret_backend_uuid
FROM model_secret_backend
WHERE model_uuid = ?`
	row = s.DB().QueryRow(q, modelUUID)
	err = row.Scan(&actualBackendID)
	c.Assert(err, tc.IsNil)
	c.Assert(actualBackendID, tc.Equals, anotherBackendID)
}

func (s *stateSuite) TestSetModelSecretBackendBackendNotFound(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)
	err := s.state.SetModelSecretBackend(c.Context(), modelUUID, "non-existing-backend-name")
	c.Assert(err, tc.ErrorIs, backenderrors.NotFound)
	c.Assert(err, tc.ErrorMatches, `cannot get secret backend "non-existing-backend-name": secret backend not found`)
}

func (s *stateSuite) TestSetModelSecretBackendModelNotFound(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	result, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID,
			Name: "my-backend",
		},
		BackendType: "vault",
	})
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.Equals, backendID)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:          backendID,
		Name:        "my-backend",
		BackendType: "vault",
	}, nil)

	modelUUID := modeltesting.GenModelUUID(c)
	err = s.state.SetModelSecretBackend(c.Context(), modelUUID, "my-backend")
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
	c.Assert(err, tc.ErrorMatches, `cannot set secret backend "my-backend" for model "`+modelUUID.String()+`": model not found`)
}

func (s *stateSuite) TestGetSecretBackendReference(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)
	secretRevisionID := uuid.MustNewUUID().String()
	_, err := s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.vaultBackendID}, modelUUID, secretRevisionID)
	c.Assert(err, tc.ErrorIsNil)

	refCount, err := s.state.GetSecretBackendReferenceCount(c.Context(), s.vaultBackendID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(refCount, tc.Equals, 1)
}

func (s *stateSuite) TestGetSecretBackendReferenceNotFound(c *tc.C) {
	backendID := uuid.MustNewUUID().String()
	refCount, err := s.state.GetSecretBackendReferenceCount(c.Context(), backendID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(refCount, tc.Equals, 0)
}

func assertSecretBackendReference(c *tc.C, db *sql.DB, backendID string, expected int) {
	q := `
SELECT COUNT(*)
FROM secret_backend_reference
WHERE secret_backend_uuid = ?`
	row := db.QueryRow(q, backendID)
	var refCount int
	err := row.Scan(&refCount)
	c.Assert(err, tc.IsNil)
	c.Assert(refCount, tc.Equals, expected)
}

func (s *stateSuite) TestAddSecretBackendReference(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)
	secretRevisionID := uuid.MustNewUUID().String()

	assertSecretBackendReference(c, s.DB(), s.vaultBackendID, 0)
	rollback, err := s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.vaultBackendID}, modelUUID, secretRevisionID)
	c.Assert(err, tc.ErrorIsNil)
	assertSecretBackendReference(c, s.DB(), s.vaultBackendID, 1)
	c.Assert(rollback(), tc.ErrorIsNil)
	assertSecretBackendReference(c, s.DB(), s.vaultBackendID, 0)
}

func (s *stateSuite) TestAddSecretBackendReferenceFailedAlreadyExists(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)
	secretRevisionID := uuid.MustNewUUID().String()

	assertSecretBackendReference(c, s.DB(), s.vaultBackendID, 0)
	_, err := s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.vaultBackendID}, modelUUID, secretRevisionID)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.vaultBackendID}, modelUUID, secretRevisionID)
	c.Assert(err, tc.ErrorIs, backenderrors.RefCountAlreadyExists)
}

func (s *stateSuite) TestAddSecretBackendReferenceFailedSecretBackendNotFound(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)
	backendID := uuid.MustNewUUID().String()
	secretRevisionID := uuid.MustNewUUID().String()
	_, err := s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: backendID}, modelUUID, secretRevisionID)
	c.Assert(err, tc.ErrorIs, backenderrors.NotFound)
}

func (s *stateSuite) TestAddSecretBackendReferenceFailedModelNotFound(c *tc.C) {
	_ = s.createModel(c, coremodel.IAAS)
	nonExistsModelUUID := modeltesting.GenModelUUID(c)
	secretRevisionID := uuid.MustNewUUID().String()
	_, err := s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.vaultBackendID}, nonExistsModelUUID, secretRevisionID)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *stateSuite) TestUpdateSecretBackendReference(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)
	secretRevisionID := uuid.MustNewUUID().String()

	assertSecretBackendReference(c, s.DB(), s.vaultBackendID, 0)
	assertSecretBackendReference(c, s.DB(), s.internalBackendID, 0)

	_, err := s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.vaultBackendID}, modelUUID, secretRevisionID)
	c.Assert(err, tc.ErrorIsNil)
	assertSecretBackendReference(c, s.DB(), s.vaultBackendID, 1)
	assertSecretBackendReference(c, s.DB(), s.internalBackendID, 0)

	rollback, err := s.state.UpdateSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.internalBackendID}, modelUUID, secretRevisionID)
	c.Assert(err, tc.ErrorIsNil)
	assertSecretBackendReference(c, s.DB(), s.vaultBackendID, 0)
	assertSecretBackendReference(c, s.DB(), s.internalBackendID, 1)
	c.Assert(rollback(), tc.ErrorIsNil)
	assertSecretBackendReference(c, s.DB(), s.vaultBackendID, 1)
	assertSecretBackendReference(c, s.DB(), s.internalBackendID, 0)
}

func (s *stateSuite) TestUpdateSecretBackendReferenceFailedNoExistingRefCountFound(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)
	secretRevisionID := uuid.MustNewUUID().String()

	_, err := s.state.UpdateSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.internalBackendID}, modelUUID, secretRevisionID)
	c.Assert(err, tc.ErrorIs, backenderrors.RefCountNotFound)
}

func (s *stateSuite) TestRemoveSecretBackendReference(c *tc.C) {
	modelUUID := s.createModel(c, coremodel.IAAS)
	secretRevisionID1 := uuid.MustNewUUID().String()
	secretRevisionID2 := uuid.MustNewUUID().String()

	_, err := s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.vaultBackendID}, modelUUID, secretRevisionID1)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.state.AddSecretBackendReference(c.Context(), &secrets.ValueRef{BackendID: s.vaultBackendID}, modelUUID, secretRevisionID2)
	c.Assert(err, tc.ErrorIsNil)

	assertSecretBackendReference(c, s.DB(), s.vaultBackendID, 2)
	err = s.state.RemoveSecretBackendReference(c.Context(), secretRevisionID1)
	c.Assert(err, tc.ErrorIsNil)
	assertSecretBackendReference(c, s.DB(), s.vaultBackendID, 1)

	err = s.state.RemoveSecretBackendReference(c.Context(), secretRevisionID2)
	c.Assert(err, tc.ErrorIsNil)
	assertSecretBackendReference(c, s.DB(), s.vaultBackendID, 0)
}

func (s *stateSuite) TestInitialWatchStatement(c *tc.C) {
	table, q := s.state.InitialWatchStatementForSecretBackendRotationChanges()
	c.Assert(table, tc.Equals, "secret_backend_rotation")
	c.Assert(q, tc.Equals, `SELECT backend_uuid FROM secret_backend_rotation`)
}

func (s *stateSuite) TestGetSecretBackendRotateChanges(c *tc.C) {
	backendID1 := uuid.MustNewUUID().String()
	rotateInternal1 := 24 * time.Hour
	nextRotateTime1 := time.Now().Add(rotateInternal1)
	_, err := s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID1,
			Name: "my-backend1",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
		NextRotateTime:      &nextRotateTime1,
	})
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID1,
		Name:                "my-backend1",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal1,
	}, &nextRotateTime1)

	backendID2 := uuid.MustNewUUID().String()
	rotateInternal2 := 48 * time.Hour
	nextRotateTime2 := time.Now().Add(rotateInternal2)
	_, err = s.state.CreateSecretBackend(c.Context(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   backendID2,
			Name: "my-backend2",
		},
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal2,
		NextRotateTime:      &nextRotateTime2,
	})
	c.Assert(err, tc.IsNil)
	s.assertSecretBackend(c, secretbackend.SecretBackend{
		ID:                  backendID2,
		Name:                "my-backend2",
		BackendType:         "vault",
		TokenRotateInterval: &rotateInternal2,
	}, &nextRotateTime2)

	changes, err := s.state.GetSecretBackendRotateChanges(c.Context(), backendID1, backendID2)
	c.Assert(err, tc.IsNil)
	c.Assert(changes, tc.HasLen, 2)
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Name < changes[j].Name
	})
	c.Assert(changes[0].ID, tc.Equals, backendID1)
	c.Assert(changes[0].Name, tc.Equals, "my-backend1")
	c.Assert(changes[0].NextTriggerTime.Equal(nextRotateTime1), tc.IsTrue)
	c.Assert(changes[1].ID, tc.Equals, backendID2)
	c.Assert(changes[1].Name, tc.Equals, "my-backend2")
	c.Assert(changes[1].NextTriggerTime.Equal(nextRotateTime2), tc.IsTrue)
}
