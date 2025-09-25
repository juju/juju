// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelation_test

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation/service"
	controllerstate "github.com/juju/juju/domain/crossmodelrelation/state/controller"
	modelstate "github.com/juju/juju/domain/crossmodelrelation/state/model"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ControllerModelSuite

	modelUUID  string
	modelIdler *changestreamtesting.Idler
}

func TestWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ControllerModelSuite.SetUpTest(c)

	s.modelUUID = uuid.MustNewUUID().String()
	_, s.modelIdler = s.InitWatchableDB(c, s.modelUUID)
}

func (s *watcherSuite) TestWatchRemoteApplicationOfferers(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.modelUUID)

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchRemoteApplicationOfferers(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	db, err := s.GetWatchableDB(c.Context(), s.modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s.modelIdler, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
		err = svc.AddRemoteApplicationOfferer(c.Context(), "foo", service.AddRemoteApplicationOffererArgs{
			OfferUUID:        tc.Must(c, uuid.NewUUID).String(),
			OffererModelUUID: tc.Must(c, uuid.NewUUID).String(),
			Endpoints: []charm.Relation{{
				Name:  "db",
				Role:  charm.RoleProvider,
				Scope: charm.ScopeGlobal,
			}},
			Macaroon: newMacaroon(c, "offer-macaroon"),
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
DELETE FROM application_remote_offerer
WHERE application_uuid = (SELECT uuid FROM application WHERE name = ?)
`, "foo")
			return err
		})
		c.Assert(err, tc.ErrorIsNil)

	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) setupService(c *tc.C, factory domain.WatchableDBFactory) *service.WatchableService {
	controllerDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ControllerTxnRunner(), nil
	}
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.GetWatchableDB(ctx, s.modelUUID)
	}

	controllerState := controllerstate.NewState(controllerDB, loggertesting.WrapCheckLog(c))
	modelState := modelstate.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c))

	return service.NewWatchableService(
		controllerState,
		modelState,
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func newMacaroon(c *tc.C, id string) *macaroon.Macaroon {
	mac, err := macaroon.New(nil, []byte(id), "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	return mac
}
