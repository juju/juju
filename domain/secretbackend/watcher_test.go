// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend_test

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	domainmodeltesting "github.com/juju/juju/domain/model/state/testing"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/domain/secretbackend/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/uuid"
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

	watcher, err := svc.WatchSecretBackendRotationChanges(context.Background())
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
	wC.AssertChanges(
		corewatcher.SecretBackendRotateChange{
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

	_ = state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		_, err := state.GetSecretBackend(ctx, secretbackend.BackendIdentifier{ID: backendID1})
		c.Assert(err, gc.ErrorMatches, `secret backend not found: "`+backendID1+`"`)
		_, err = state.GetSecretBackend(ctx, secretbackend.BackendIdentifier{ID: backendID2})
		c.Assert(err, gc.ErrorMatches, `secret backend not found: "`+backendID2+`"`)
		return nil
	})

	wC.AssertNoChange()
}

func (s *watcherSuite) TestWatchModelSecretBackendChanged(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model_secretbackend_changes")
	txnRunnerFactory := func() (database.TxnRunner, error) { return factory() }

	logger := loggertesting.WrapCheckLog(c)
	state := state.NewState(txnRunnerFactory, logger)

	svc := service.NewWatchableService(
		state, logger,
		domain.NewWatcherFactory(factory, logger),
	)

	modelUUID, internalBackendName, vaultBackendName := s.createModel(c, state, txnRunnerFactory, "test-model")

	watcher, err := svc.WatchModelSecretBackendChanged(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, watcher)

	wc := watchertest.NewNotifyWatcherC(c, watcher)
	// Wait for the initial change.
	wc.AssertOneChange()

	_ = state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		err = state.SetModelSecretBackend(ctx, modelUUID, vaultBackendName)
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	wc.AssertOneChange()

	_ = state.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		err = state.SetModelSecretBackend(ctx, modelUUID, internalBackendName)
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	wc.AssertOneChange()

	// Pretend that the agent restarted and the watcher is re-created.
	watcher1, err := svc.WatchModelSecretBackendChanged(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, watcher1)
	wc1 := watchertest.NewNotifyWatcherC(c, watcher1)
	// Wait for the initial change.
	wc1.AssertOneChange()
}

func (s *watcherSuite) createModel(c *gc.C, st *state.State, txnRunner database.TxnRunnerFactory, name string) (coremodel.UUID, string, string) {
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

	modelUUID := domainmodeltesting.CreateTestModel(c, txnRunner, name)
	return modelUUID, juju.BackendName, "my-backend"
}
