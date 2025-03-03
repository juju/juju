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

func (s *watcherSuite) TestWatchController(c *gc.C) {
	logger := loggertesting.WrapCheckLog(c)
	watchableDBFactory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "model")
	watcherFactory := domain.NewWatcherFactory(watchableDBFactory, logger)
	st := state.NewState(func() (database.TxnRunner, error) { return watchableDBFactory() })
	controllerSvc := service.NewService(st, watcherFactory)

	watcher, err := controllerSvc.Watch(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watcher, gc.NotNil)

	type model struct {
		UUID        string `db:"uuid"`
		Activated   bool   `db:"activated"`
		ModelTypeID int    `db:"model_type_id"`
		Name        string `db:"name"`
		CloudUUID   string `db:"cloud_uuid"`
		LifeID      int    `db:"life_id"`
		OwnerUUID   string `db:"owner_uuid"`
	}

	var testModel model
	modelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-model").String()
	err = s.updateModelActiveStatus(c, modelUUID, false)
	c.Assert(err, jc.ErrorIsNil)

	row := s.DB().QueryRow(`SELECT activated FROM model WHERE  uuid = ?`, modelUUID)
	err = row.Scan(&testModel.Activated)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testModel.Activated, jc.IsFalse)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(func(c *gc.C) {
		// Update model active status.
		err = s.updateModelActiveStatus(c, modelUUID, true)
		c.Assert(err, jc.ErrorIsNil)

		// Check if model active status is updated.
		row := s.DB().QueryRow(`SELECT activated FROM model WHERE  uuid = ?`, modelUUID)
		err = row.Scan(&testModel.Activated)

		c.Assert(err, jc.ErrorIsNil)
		c.Check(testModel.Activated, jc.IsFalse)
	}, func(w watchertest.WatcherC[[]string]) {
		// Get the change.
		w.Check(
			watchertest.StringSliceAssert(
				modelUUID,
			),
		)
	})

	harness.Run(c, []string{"false"})
}
