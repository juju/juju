// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend_test

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	userstate "github.com/juju/juju/domain/access/state"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/domain/model"
	modelestate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/domain/secretbackend/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/version"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatchSecretBackendRotationChanges(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "secretbackend_rotation_changes")

	logger := loggertesting.WrapCheckLog(c)
	state := state.NewState(func() (database.TxnRunner, error) { return factory() }, logger)

	svc := service.NewWatchableService(
		state, logger,
		domain.NewWatcherFactory(factory, logger),
	)

	watcher, err := svc.WatchSecretBackendRotationChanges()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, watcher)

	wC := watchertest.NewSecretBackendRotateWatcherC(c, watcher)
	// Wait for the initial change.
	wC.AssertChanges([]corewatcher.SecretBackendRotateChange(nil)...)

	backendID1 := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	result, err := state.CreateSecretBackend(context.Background(),
		secretbackend.CreateSecretBackendParams{
			BackendIdentifier: secretbackend.BackendIdentifier{
				ID:   backendID1,
				Name: "my-backend1",
			},
			BackendType:         "vault",
			TokenRotateInterval: &rotateInternal,
			NextRotateTime:      &nextRotateTime,
			Config: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
	)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, backendID1)

	backendID2 := uuid.MustNewUUID().String()
	result, err = state.CreateSecretBackend(context.Background(),
		secretbackend.CreateSecretBackendParams{
			BackendIdentifier: secretbackend.BackendIdentifier{
				ID:   backendID2,
				Name: "my-backend2",
			},
			BackendType:         "vault",
			TokenRotateInterval: &rotateInternal,
			NextRotateTime:      &nextRotateTime,
			Config: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
	)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, backendID2)

	// Triggered by INSERT.
	wC.AssertChanges(
		corewatcher.SecretBackendRotateChange{
			ID:              backendID1,
			Name:            "my-backend1",
			NextTriggerTime: nextRotateTime,
		},
		corewatcher.SecretBackendRotateChange{
			ID:              backendID2,
			Name:            "my-backend2",
			NextTriggerTime: nextRotateTime,
		},
	)

	nameChange := "my-backend1-updated"
	_, err = state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID1,
		},
		NewName: &nameChange,
		Config: map[string]string{
			"key1": "value1-updated",
			"key3": "value3",
		},
	})
	c.Assert(err, gc.IsNil)
	// NOT triggered - updated the backend name and config.
	wC.AssertNoChange()

	newRotateInternal := 48 * time.Hour
	newNextRotateTime := time.Now().Add(newRotateInternal)
	_, err = state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID: backendID2,
		},
		TokenRotateInterval: &newRotateInternal,
		NextRotateTime:      &newNextRotateTime,
		Config: map[string]string{
			"key1": "value1-updated",
			"key3": "value3",
		},
	})
	c.Assert(err, gc.IsNil)
	// Triggered - updated the rotation time.
	wC.AssertChanges(corewatcher.SecretBackendRotateChange{
		ID:              backendID2,
		Name:            "my-backend2",
		NextTriggerTime: newNextRotateTime,
	},
	)

	// NOT triggered - delete the backend.
	err = state.DeleteSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: backendID1}, false)
	c.Assert(err, gc.IsNil)
	err = state.DeleteSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: backendID2}, false)
	c.Assert(err, gc.IsNil)

	_, err = state.GetSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: backendID1})
	c.Assert(err, gc.ErrorMatches, `secret backend not found: "`+backendID1+`"`)
	_, err = state.GetSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: backendID2})
	c.Assert(err, gc.ErrorMatches, `secret backend not found: "`+backendID2+`"`)

	wC.AssertNoChange()
}

func (s *watcherSuite) TestWatchModelSecretBackendChanged(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model_secretbackend_changes")

	logger := loggertesting.WrapCheckLog(c)
	state := state.NewState(func() (database.TxnRunner, error) { return factory() }, logger)

	svc := service.NewWatchableService(
		state, logger,
		domain.NewWatcherFactory(factory, logger),
	)

	modelUUID, internalBackendName, vaultBackendName := s.createModel(c, state, "test-model")

	watcher, err := svc.WatchModelSecretBackendChanged(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, watcher)

	wc := watchertest.NewNotifyWatcherC(c, watcher)
	// Wait for the initial change.
	wc.AssertOneChange()
	wc.AssertNoChange()

	err = state.SetModelSecretBackend(context.Background(), modelUUID, internalBackendName)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
	wc.AssertNoChange()

	err = state.SetModelSecretBackend(context.Background(), modelUUID, vaultBackendName)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
	wc.AssertNoChange()

	// Pretend that the agent restarted and the watcher is re-created.
	watcher1, err := svc.WatchModelSecretBackendChanged(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, watcher1)
	wc1 := watchertest.NewNotifyWatcherC(c, watcher1)
	// Wait for the initial change.
	wc1.AssertOneChange()
	wc1.AssertNoChange()
}

func (s *watcherSuite) createModel(c *gc.C, st *state.State, name string) (coremodel.UUID, string, string) {
	// Create internal controller secret backend.
	internalBackendID := uuid.MustNewUUID().String()
	result, err := st.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   internalBackendID,
			Name: juju.BackendName,
		},
		BackendType: juju.BackendType,
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, internalBackendID)

	vaultBackendID := uuid.MustNewUUID().String()
	result, err = st.CreateSecretBackend(context.Background(), secretbackend.CreateSecretBackendParams{
		BackendIdentifier: secretbackend.BackendIdentifier{
			ID:   vaultBackendID,
			Name: "my-backend",
		},
		BackendType: "vault",
		Config: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, vaultBackendID)

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
		coremodel.IAAS,
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

	err = st.SetModelSecretBackend(context.Background(), modelUUID, "my-backend")
	c.Assert(err, jc.ErrorIsNil)
	return modelUUID, juju.BackendName, "my-backend"
}
