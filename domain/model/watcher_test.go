// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"database/sql"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/model/service"
	"github.com/juju/juju/domain/model/state"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ControllerSuite
}

var _ = gc.Suite(&watcherSuite{})

func Test(t *testing.T) {
	gc.TestingT(t)
}

type dbModel struct {
	UUID        string `db:"uuid"`
	Activated   bool   `db:"activated"`
	ModelTypeID int    `db:"model_type_id"`

	Name      string `db:"name"`
	CloudUUID string `db:"cloud_uuid"`
	LifeID    int    `db:"life_id"`
	OwnerUUID string `db:"owner_uuid"`
}

func (s *watcherSuite) TestWatchControllerDBModels(c *gc.C) {
	logger := loggertesting.WrapCheckLog(c)
	watchableDBFactory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model")
	watcherFactory := domain.NewWatcherFactory(watchableDBFactory, logger)
	st := state.NewState(func() (database.TxnRunner, error) { return watchableDBFactory() })
	modelSvc := service.NewWatchableService(st, nil, loggertesting.WrapCheckLog(c), watcherFactory)

	// Create a controller service watcher.
	watcher, err := modelSvc.WatchActivatedModels(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watcher, gc.NotNil)

	modelName := "test-model"
	var modelUUID model.UUID
	var modelUUIDStr string

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Verifies that watchers do not receive any changes when newly unactivated models are created.
	harness.AddTest(func(c *gc.C) {
		// Create a new model named test-model and activate it.
		modelUUID = modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), modelName)
		modelUUIDStr = modelUUID.String()

		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			// Set model activated status to false. This should not trigger a change event.
			_, err := tx.ExecContext(ctx, "UPDATE model SET activated = ? WHERE uuid = ?", false, modelUUID)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)

		// Ensure that the initial activated status of model is indeed false.
		var testModel dbModel
		row := s.DB().QueryRow(`SELECT activated FROM model WHERE uuid = ?`, modelUUIDStr)
		err = row.Scan(&testModel.Activated)
		c.Check(err, jc.ErrorIsNil)
		c.Check(testModel.Activated, jc.IsFalse)
	}, func(w watchertest.WatcherC[[]string]) {
		// Get the change.
		w.AssertNoChange()
	})

	// Verifies that watchers do not receive any changes when unactivated models are updated.
	harness.AddTest(func(c *gc.C) {
		updatedName := "new-test-model"
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			// Update name of unactivated model. This should not trigger a change event.
			_, err := tx.ExecContext(ctx, "UPDATE model SET name = ? WHERE uuid = ?", updatedName, modelUUIDStr)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
		// Checks if model name is updated successfully.
		selectModelQuery := `
			SELECT uuid, activated, model_type_id, name, cloud_uuid, life_id, owner_uuid 
			FROM model 
			WHERE uuid = ?`
		var testModel dbModel
		row := s.DB().QueryRow(selectModelQuery, modelUUIDStr)
		err = row.Scan(&testModel.UUID, &testModel.Activated, &testModel.ModelTypeID,
			&testModel.Name, &testModel.CloudUUID, &testModel.LifeID, &testModel.OwnerUUID)
		c.Check(err, jc.ErrorIsNil)
		c.Check(testModel.UUID, gc.Equals, modelUUIDStr)
		c.Check(testModel.Activated, jc.IsFalse)
		c.Check(testModel.Name, gc.Equals, updatedName)
	}, func(w watchertest.WatcherC[[]string]) {
		// Get the change.
		w.AssertNoChange()
	})

	// Verifies that watchers receive changes when models are activated.
	harness.AddTest(func(c *gc.C) {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			// Update activated status of model. This should trigger a change event.
			_, err := tx.ExecContext(ctx, "UPDATE model SET activated = ? WHERE uuid = ?", true, modelUUIDStr)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				modelUUIDStr,
			),
		)
	})

	// Verifies that watchers receive changes when activated models are updated.
	harness.AddTest(func(c *gc.C) {
		updatedName := "new-test-model-2"
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			// Update name of activated model. This should trigger a change event.
			_, err := tx.ExecContext(ctx, "UPDATE model SET name = ? WHERE uuid = ?", updatedName, modelUUIDStr)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
		// Checks if model name is updated successfully.
		selectModelQuery := `
		SELECT uuid, activated, model_type_id, name, cloud_uuid, life_id, owner_uuid 
		FROM model 
		WHERE uuid = ?`
		var testModel dbModel
		row := s.DB().QueryRow(selectModelQuery, modelUUIDStr)
		err = row.Scan(&testModel.UUID, &testModel.Activated, &testModel.ModelTypeID,
			&testModel.Name, &testModel.CloudUUID, &testModel.LifeID, &testModel.OwnerUUID)
		c.Check(err, jc.ErrorIsNil)
		c.Check(testModel.UUID, gc.Equals, modelUUIDStr)
		c.Check(testModel.Activated, jc.IsTrue)
		c.Check(testModel.Name, gc.Equals, updatedName)

	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				modelUUIDStr,
			),
		)
	})

	// Verifies that watchers do not receive changes when entities other than models are updated.
	harness.AddTest(func(c *gc.C) {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			// Insert into table that is not model. This should not trigger a change event.
			_, err := tx.ExecContext(ctx, "INSERT into cloud_type (id, type) values (100, 'testing')")
			if err != nil {
				return err
			}
			// Update entity that is not model. This should not trigger a change event.
			_, err = tx.ExecContext(ctx, "UPDATE cloud_type SET type = 'test' WHERE id = 100")
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Verifies that watchers do not receive changes when models are deleted.
	harness.AddTest(func(c *gc.C) {
		// Deletes model from table. This should not trigger a change event.
		modeltesting.DeleteTestModel(c, context.Background(), s.TxnRunnerFactory(), modelUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string(nil))
}
