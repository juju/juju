// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend_test

import (
	"context"
	"database/sql"
	"sort"
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

func changeLog(c *gc.C, db database.TxnRunner) {
	q := `
SELECT edit_type_id, namespace_id, changed, created_at
FROM change_log
`[1:]
	err := db.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q)
		c.Assert(err, jc.ErrorIsNil)
		defer rows.Close()
		var (
			editTypeID  int
			namespaceID int
			changed     string
			createdAt   sql.NullTime
		)
		c.Log("change log table records:")
		for rows.Next() {
			err = rows.Scan(&editTypeID, &namespaceID, &changed, &createdAt)
			c.Assert(err, jc.ErrorIsNil)
			c.Logf(
				"editTypeID: %d, namespaceID: %d, changed: %s, createdAt: %v",
				editTypeID, namespaceID, changed, createdAt.Time,
			)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchSecretBackendRotationChanges(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "secretbackend_rotation_changes")

	logger := testing.NewCheckLogger(c)
	state := state.NewState(func() (database.TxnRunner, error) { return factory() }, logger)

	db, err := state.DB()
	c.Assert(err, jc.ErrorIsNil)

	svc := service.NewWatchableService(
		state, logger,
		domain.NewWatcherFactory(factory, logger),
		testing.ControllerTag.Id(), nil,
	)

	watcher, err := svc.WatchSecretBackendRotationChanges()
	c.Assert(err, jc.ErrorIsNil)

	// Wait for the initial change.
	assertChanges(c, db, watcher, []corewatcher.SecretBackendRotateChange(nil)...)

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
	assertChanges(c, db, watcher,
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

	// NOT triggered - update the backend name and config.
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

	// Triggered - UPDATE the rotation time.
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

	assertChanges(c, db, watcher,
		corewatcher.SecretBackendRotateChange{
			ID:              backendID2,
			Name:            "my-backend2",
			NextTriggerTime: newNextRotateTime,
		},
	)

	// NOT triggered - delete the backend.
	err = state.DeleteSecretBackend(context.Background(), backendID1, false)
	c.Assert(err, gc.IsNil)
	err = state.DeleteSecretBackend(context.Background(), backendID2, false)
	c.Assert(err, gc.IsNil)

	_, err = state.GetSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: backendID1})
	c.Assert(err, gc.ErrorMatches, `secret backend not found: "`+backendID1+`"`)
	_, err = state.GetSecretBackend(context.Background(), secretbackend.BackendIdentifier{ID: backendID2})
	c.Assert(err, gc.ErrorMatches, `secret backend not found: "`+backendID2+`"`)

	select {
	case change := <-watcher.Changes():
		c.Fatalf("unexpected change: %v", change)
	case <-time.After(testing.ShortWait):
	}
}

func assertChanges(
	c *gc.C, db database.TxnRunner, watcher corewatcher.SecretBackendRotateWatcher,
	expected ...corewatcher.SecretBackendRotateChange,
) {
	timeOut := time.After(testing.LongWait)
	var received []corewatcher.SecretBackendRotateChange

CheckAllChangesReceived:
	for {
		select {
		case change := <-watcher.Changes():
			received = append(received, change...)
			c.Logf("received => %#v", received)
			c.Logf("expected => %#v", expected)
			if len(received) == len(expected) {
				sort.Slice(received, func(i, j int) bool {
					return received[i].Name < received[j].Name
				})
				for i := range received {
					c.Assert(received[i].ID, gc.Equals, expected[i].ID)
					c.Assert(received[i].Name, gc.Equals, expected[i].Name)
					c.Assert(received[i].NextTriggerTime.Equal(expected[i].NextTriggerTime), jc.IsTrue)
				}
				break CheckAllChangesReceived // Got all the changes.
			}
		case <-timeOut:
			changeLog(c, db)
			c.Logf(`changes received: %#v;\nchanges expected: %#v`, received, expected)
			c.Fail()
		}
	}
}
