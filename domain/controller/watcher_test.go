// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/controller/service"
	"github.com/juju/juju/domain/controller/state"
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

type model struct {
	UUID        string `db:"uuid"`
	Activated   bool   `db:"activated"`
	ModelTypeID int    `db:"model_type_id"`
	Name        string `db:"name"`
	CloudUUID   string `db:"cloud_uuid"`
	LifeID      int    `db:"life_id"`
	OwnerUUID   string `db:"owner_uuid"`
}

func (s *watcherSuite) TestWatchController(c *gc.C) {
	logger := loggertesting.WrapCheckLog(c)
	watchableDBFactory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model")
	watcherFactory := domain.NewWatcherFactory(watchableDBFactory, logger)
	st := state.NewState(func() (database.TxnRunner, error) { return watchableDBFactory() })
	controllerSvc := service.NewService(st, watcherFactory)

	// Create a controller service watcher.
	watcher, err := controllerSvc.Watch(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watcher, gc.NotNil)

	// Create a new model named test-model.
	modelName := "test-model"
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), modelName)
	modelUUIDStr := modelUUID.String()

	// Set model activated status to false.
	s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE model SET activated = ? WHERE uuid = ?", false, modelUUID)
		c.Assert(err, jc.ErrorIsNil)
		rowsAffected, err := res.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(int(rowsAffected), gc.Equals, 1)
		return nil
	})

	// Ensure that the initial activated status of model is false.
	var testModel model
	row := s.DB().QueryRow(`SELECT activated FROM model WHERE  uuid = ?`, modelUUIDStr)
	err = row.Scan(&testModel.Activated)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testModel.Activated, jc.IsFalse)

	wc := watchertest.NewNotifyWatcherC(c, watcher)

	// Tests that the watcher sends a change when any model field is updated.
	s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {

		// Update activated status of model.
		res, err := tx.ExecContext(ctx, "UPDATE model SET activated = ? WHERE uuid = ?", true, modelUUIDStr)
		c.Assert(err, jc.ErrorIsNil)
		rowsAffected, err := res.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(int(rowsAffected), gc.Equals, 1)

		// Update name of model.
		res, err = tx.ExecContext(ctx, "UPDATE model SET name = ? WHERE uuid = ?", "new-test-model", modelUUIDStr)
		c.Assert(err, jc.ErrorIsNil)
		rowsAffected, err = res.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(int(rowsAffected), gc.Equals, 1)

		selectModelQuery := `
			SELECT uuid, activated, model_type_id, name, cloud_uuid, life_id, owner_uuid 
			FROM model 
			WHERE uuid = ?
		`
		row := tx.QueryRow(selectModelQuery, modelUUIDStr)
		err = row.Scan(&testModel.UUID, &testModel.Activated, &testModel.ModelTypeID, &testModel.Name, &testModel.CloudUUID, &testModel.LifeID, &testModel.OwnerUUID)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(testModel.UUID, gc.Equals, modelUUIDStr)
		c.Check(testModel.Activated, jc.IsTrue)

		// Insert into and update table that is not model.
		res, err = tx.ExecContext(ctx, "Insert into cloud_type (id, type) values (100, 'testing')")
		c.Assert(err, jc.ErrorIsNil)
		rowsAffected, err = res.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(int(rowsAffected), gc.Equals, 1)

		res, err = tx.ExecContext(ctx, "UPDATE cloud_type SET type = 'test' WHERE id = 100")
		c.Assert(err, jc.ErrorIsNil)
		rowsAffected, err = res.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(int(rowsAffected), gc.Equals, 1)

		return nil
	})
	wc.AssertNChanges(2)

	// Tests that the watcher sends a change when a model is deleted.
	modeltesting.DeleteTestModel(c, context.Background(), s.TxnRunnerFactory(), modelUUID)
	wc.AssertOneChange()
}
