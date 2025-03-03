// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"
	"testing"

	jc "github.com/juju/testing/checkers"
	"gopkg.in/check.v1"
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
	check.TestingT(t)
}

func (s *watcherSuite) updateModelActiveStatus(c *gc.C, modelUUID string, status bool) error {
	return s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE model SET activated = ? WHERE uuid = ?", status, modelUUID)
		if err != nil {
			return err
		}
		rowsAffected, err := res.RowsAffected()
		c.Check(int(rowsAffected), gc.Equals, 1)

		return err
	})
}

func (s *watcherSuite) updateModelName(c *gc.C, modelUUID string, newName string) error {
	return s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "UPDATE model SET name = ? WHERE uuid = ?", newName, modelUUID)
		if err != nil {
			return err
		}
		rowsAffected, err := res.RowsAffected()
		c.Check(int(rowsAffected), gc.Equals, 1)

		return err
	})
}

func (s *watcherSuite) deleteModel(c *gc.C, modelUUID string) error {
	return s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE from model WHERE uuid = ?", modelUUID)
		if err != nil {
			return err
		}
		rowsAffected, err := res.RowsAffected()
		c.Check(int(rowsAffected), gc.Equals, 1)

		return err
	})
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

func (s *watcherSuite) deleteModelWithDependencyTables(c *gc.C, ctx context.Context, tx *sql.Tx, testModel model) {
	res, err := tx.ExecContext(ctx, "DELETE from user WHERE uuid = ?", testModel.OwnerUUID)
	c.Assert(err, jc.ErrorIsNil)
	rowsAffected, err := res.RowsAffected()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(int(rowsAffected), gc.Equals, 1)

	res, err = tx.ExecContext(ctx, "DELETE from cloud WHERE uuid = ?", testModel.CloudUUID)
	c.Assert(err, jc.ErrorIsNil)
	rowsAffected, err = res.RowsAffected()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(int(rowsAffected), gc.Equals, 1)

	res, err = tx.ExecContext(ctx, "DELETE from model WHERE uuid = ?", testModel.UUID)
	c.Assert(err, jc.ErrorIsNil)
	rowsAffected, err = res.RowsAffected()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(int(rowsAffected), gc.Equals, 1)

}

func (s *watcherSuite) TestWatchController(c *gc.C) {
	logger := loggertesting.WrapCheckLog(c)
	watchableDBFactory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model")
	watcherFactory := domain.NewWatcherFactory(watchableDBFactory, logger)
	st := state.NewState(func() (database.TxnRunner, error) { return watchableDBFactory() })
	controllerSvc := service.NewService(st, watcherFactory)

	watcher, err := controllerSvc.Watch(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watcher, gc.NotNil)

	var testModel model
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-model")
	modelUUIDStr := modelUUID.String()
	err = s.updateModelActiveStatus(c, modelUUIDStr, false)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that the initial activated status of model is false.
	row := s.DB().QueryRow(`SELECT activated FROM model WHERE  uuid = ?`, modelUUIDStr)
	err = row.Scan(&testModel.Activated)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testModel.Activated, jc.IsFalse)

	wc := watchertest.NewNotifyWatcherC(c, watcher)
	// s.updateModelActiveStatus(c, modelUUIDStr, true)
	// wc.AssertOneChange()

	// s.updateModelName(c, modelUUIDStr, "new-name")
	// wc.AssertOneChange()

	// dbInitialModel := modeltesting.DbInitialModel{
	// 	UUID: modelUUIDStr,
	// }
	// modeltesting.DeleteTestModel(c, s.TxnRunnerFactory(), dbInitialModel)
	// wc.AssertOneChange()

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		defer tx.Rollback()
		res, err := tx.ExecContext(ctx, "UPDATE model SET activated = ? WHERE uuid = ?", true, modelUUIDStr)
		c.Assert(err, jc.ErrorIsNil)
		rowsAffected, err := res.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(int(rowsAffected), gc.Equals, 1)

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

		err = tx.Commit()
		c.Assert(err, jc.ErrorIsNil)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNChanges(2)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		defer tx.Rollback()

		res, err := tx.ExecContext(ctx, "DELETE from user WHERE uuid = ?", testModel.OwnerUUID)
		c.Assert(err, jc.ErrorIsNil)
		rowsAffected, err := res.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(int(rowsAffected), gc.Equals, 1)

		res, err = tx.ExecContext(ctx, "DELETE from cloud_region WHERE cloud_uuid = ?", testModel.CloudUUID)
		c.Assert(err, jc.ErrorIsNil)
		rowsAffected, err = res.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(int(rowsAffected), gc.Equals, 1)

		res, err = tx.ExecContext(ctx, "DELETE from cloud_credential WHERE cloud_uuid = ?", testModel.CloudUUID)
		c.Assert(err, jc.ErrorIsNil)
		rowsAffected, err = res.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(int(rowsAffected), gc.Equals, 1)

		res, err = tx.ExecContext(ctx, "DELETE from cloud WHERE uuid = ?", testModel.CloudUUID)
		c.Assert(err, jc.ErrorIsNil)
		rowsAffected, err = res.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(int(rowsAffected), gc.Equals, 1)

		res, err = tx.ExecContext(ctx, "DELETE from model WHERE uuid = ?", testModel.UUID)
		c.Assert(err, jc.ErrorIsNil)
		rowsAffected, err = res.RowsAffected()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(int(rowsAffected), gc.Equals, 1)

		err = tx.Commit()
		c.Assert(err, jc.ErrorIsNil)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}
