// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend_test

import (
	"context"
	"database/sql"
	"sort"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
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
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "secretbackend")

	logger := testing.NewCheckLogger(c)
	state := state.NewState(func() (database.TxnRunner, error) { return factory() }, logger)

	db, err := state.DB()
	c.Assert(err, jc.ErrorIsNil)

	svc := service.NewWatchableService(
		state, logger,
		domain.NewWatcherFactory(factory, logger),
		testing.ControllerTag.Id(), nil, nil,
	)

	watcher, err := svc.WatchSecretBackendRotationChanges()
	c.Assert(err, jc.ErrorIsNil)

	s.AddCleanup(func(*gc.C) {
		workertest.DirtyKill(c, watcher)
	})

	// Wait for the initial change.
	assertChanges(c, db, watcher, []corewatcher.SecretBackendRotateChange(nil)...)

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
	err = state.UpdateSecretBackend(context.Background(), secretbackend.UpdateSecretBackendParams{
		ID:         backendID1,
		NameChange: &nameChange,
		Config: map[string]interface{}{
			"key1": "value1-updated",
			"key3": "value3",
		},
	})
	c.Assert(err, gc.IsNil)

	// Triggered - UPDATE the rotation time.
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

	_, err = state.GetSecretBackend(context.Background(), backendID1)
	c.Assert(err, gc.ErrorMatches, `secret backend "`+backendID1+`" not found`)
	_, err = state.GetSecretBackend(context.Background(), backendID2)
	c.Assert(err, gc.ErrorMatches, `secret backend "`+backendID2+`" not found`)

	select {
	case change := <-watcher.Changes():
		c.Fatalf("unexpected change: %v", change)
	case <-time.After(testing.ShortWait):
	}
}

func assertChanges(c *gc.C, db database.TxnRunner, watcher corewatcher.SecretBackendRotateWatcher, expected ...corewatcher.SecretBackendRotateChange) {
	timeOut := time.After(testing.LongWait)
	var changes []corewatcher.SecretBackendRotateChange
CheckAllChangesReceived:
	for {
		select {
		case change := <-watcher.Changes():
			changes = append(changes, change...)
			c.Logf("changes received: %#v", changes)
			c.Logf("changes expected: %#v", expected)
			if len(changes) == len(expected) {
				sort.Slice(changes, func(i, j int) bool {
					return changes[i].Name < changes[j].Name
				})
				for i := range changes {
					c.Assert(changes[i].ID, gc.Equals, expected[i].ID)
					c.Assert(changes[i].Name, gc.Equals, expected[i].Name)
					c.Assert(changes[i].NextTriggerTime.Equal(expected[i].NextTriggerTime), jc.IsTrue)
				}
				break CheckAllChangesReceived // Got all the changes.
			}
		case <-timeOut:
			changeLog(c, db)
			c.Fatalf(`changes received: %#v
changes expected: %#v`, changes, expected)
		}
	}
}
