// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend_test

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/domain/secretbackend/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/testing"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatchSecretBackendRotationChanges(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "secretbackend")

	logger := testing.NewCheckLogger(c)
	state := state.NewState(func() (database.TxnRunner, error) { return factory() }, logger)
	svc := service.NewWatchableService(
		state, logger,
		domain.NewWatcherFactory(factory, logger),
		testing.ControllerTag.Id(), nil, nil,
	)

	watcher, err := svc.WatchSecretBackendRotationChanges()
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the initial change.
	select {
	case <-watcher.Changes():
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	backendID1 := uuid.MustNewUUID().String()
	rotateInternal := 24 * time.Hour
	nextRotateTime := time.Now().Add(rotateInternal)
	result, err := state.CreateSecretBackend(context.Background(),
		secretbackend.CreateSecretBackendParams{
			ID:                  backendID1,
			Name:                "my-backend1",
			BackendType:         "vault",
			TokenRotateInterval: &rotateInternal,
			NextRotateTime:      &nextRotateTime,
			Config: map[string]interface{}{
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
			ID:                  backendID2,
			Name:                "my-backend2",
			BackendType:         "vault",
			TokenRotateInterval: &rotateInternal,
			NextRotateTime:      &nextRotateTime,
			Config: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
	)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, backendID2)

	// Get the change.
	select {
	case change := <-watcher.Changes():
		c.Assert(change, jc.SameContents, []corewatcher.SecretBackendRotateChange{
			{
				ID:              backendID1,
				Name:            "my-backend1",
				NextTriggerTime: nextRotateTime.UTC().Round(time.Second),
			},
			{
				ID:              backendID2,
				Name:            "my-backend2",
				NextTriggerTime: nextRotateTime.UTC().Round(time.Second),
			},
		})
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	// NOT triggered - update the backend name and config.
	nameChange := "my-backend1-updated"
	err = state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		ID:         backendID1,
		NameChange: &nameChange,
		Config: map[string]interface{}{
			"key1": "value1-updated",
			"key3": "value3",
		},
	})
	c.Assert(err, gc.IsNil)

	// Triggered - update the rotation time.
	newRotateInternal := 48 * time.Hour
	newNextRotateTime := time.Now().Add(newRotateInternal)
	err = state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		ID:                  backendID2,
		TokenRotateInterval: &newRotateInternal,
		NextRotateTime:      &newNextRotateTime,
		Config: map[string]interface{}{
			"key1": "value1-updated",
			"key3": "value3",
		},
	})
	c.Assert(err, gc.IsNil)

	// Get the change.
	select {
	case change := <-watcher.Changes():
		c.Assert(change, gc.DeepEquals, []corewatcher.SecretBackendRotateChange{
			{
				ID:              backendID2,
				Name:            "my-backend2",
				NextTriggerTime: newNextRotateTime.UTC().Round(time.Second),
			},
		})
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for change")
	}

	// NOT triggered - delete the backend.
	err = state.DeleteSecretBackend(context.Background(), backendID1, false)
	c.Assert(err, gc.IsNil)
	err = state.DeleteSecretBackend(context.Background(), backendID2, false)
	c.Assert(err, gc.IsNil)

	_, err = state.GetSecretBackend(context.Background(), backendID1)
	c.Assert(err, gc.ErrorMatches, `secret backend "`+backendID1+`" not found`)
	_, err = state.GetSecretBackend(context.Background(), backendID2)
	c.Assert(err, gc.ErrorMatches, `secret backend "`+backendID2+`" not found`)

	// Get the change.
	select {
	case change := <-watcher.Changes():
		c.Fatalf("unexpected change: %v", change)
	case <-time.After(testing.ShortWait):
	}
}
