// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	coretesting "github.com/juju/juju/core/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	statemodel "github.com/juju/juju/domain/crossmodelrelation/state/model"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type watchDyingModelSuite struct {
	changestreamtesting.ModelSuite
}

func TestWatchDyingModelSuite(t *testing.T) {
	tc.Run(t, &watchDyingModelSuite{})
}

func (s *watchDyingModelSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "test", "iaas", "prod", "test-model", "ec2")
		`, modelUUID.String(), internaltesting.ControllerTag.Id())
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO model_life (model_uuid, life_id)
VALUES (?, 0);
		`, modelUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watchDyingModelSuite) TestWatchDyingModel(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "some-model-uuid")
	log := loggertesting.WrapCheckLog(c)

	modelState := statemodel.NewState(
		func(ctx context.Context) (runner database.TxnRunner, err error) {
			return s.ModelTxnRunner(), nil
		},
		model.UUID(s.ModelUUID()),
		clock.WallClock,
		log,
	)
	svc := NewWatchableService(
		nil,
		modelState,
		nil,
		domain.NewWatcherFactory(factory, log),
		clock.WallClock,
		log,
	)

	w, err := svc.WatchDyingModel(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	wc := watchertest.NewNotifyWatcherC(c, w)

	// Consume the initial notification emitted on watcher startup; it
	// carries no change data.
	wc.AssertAtLeastOneChange()

	// The model life trigger logs to the custom model life namespace,
	// so the watcher must observe the model transitioning to dying.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE model_life SET life_id = 1 WHERE life_id = 0")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, tc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("watcher did not observe the model life change")
	}
}
