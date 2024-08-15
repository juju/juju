// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	internalcharm "github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage/provider"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatchCharm(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "charm")

	svc := service.NewWatchableService(
		state.NewState(func() (database.TxnRunner, error) { return factory() }, loggertesting.WrapCheckLog(c)),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
		nil,
		loggertesting.WrapCheckLog(c),
	)
	watcher, err := svc.WatchCharms()
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness[[]string](s, watchertest.NewWatcherC[[]string](c, watcher))

	// Ensure that we get the charm created event.

	var id corecharm.ID
	harness.AddTest(func(c *gc.C) {
		id, _, err = svc.SetCharm(context.Background(), charm.SetCharmArgs{
			Charm:    &stubCharm{},
			Source:   internalcharm.CharmHub,
			Revision: 1,
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](id.String()),
		)
	})

	// Ensure that we get the charm deleted event.

	harness.AddTest(func(c *gc.C) {
		err := svc.DeleteCharm(context.Background(), id)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](id.String()),
		)
	})

	harness.Run(c)
}

type stubCharm struct{}

func (s *stubCharm) Meta() *internalcharm.Meta {
	return &internalcharm.Meta{
		Name: "test",
	}
}

func (s *stubCharm) Manifest() *internalcharm.Manifest {
	return &internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name: "ubuntu",
			Channel: internalcharm.Channel{
				Risk: internalcharm.Stable,
			},
			Architectures: []string{"amd64"},
		}},
	}
}

func (s *stubCharm) Config() *internalcharm.Config {
	return nil
}

func (s *stubCharm) Actions() *internalcharm.Actions {
	return nil
}

func (s *stubCharm) Revision() int {
	return 0
}

func ptr[T any](v T) *T {
	return &v
}

func (s *watcherSuite) createApplication(c *gc.C, svc *service.Service, name string, units ...service.AddUnitArg) coreapplication.ID {
	ctx := context.Background()
	appID, err := svc.CreateApplication(ctx, name, &stubCharm{}, corecharm.Origin{
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, service.AddApplicationArgs{}, units...)
	c.Assert(err, jc.ErrorIsNil)
	return appID
}

func (s *watcherSuite) TestWatchUnitLife(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit")

	svc := service.NewWatchableService(
		state.NewState(func() (database.TxnRunner, error) { return factory() }, loggertesting.WrapCheckLog(c)),
		domain.NewWatcherFactory(factory,
			loggertesting.WrapCheckLog(c),
		),
		provider.CommonStorageProviders(),
		loggertesting.WrapCheckLog(c),
	)

	s.createApplication(c, &svc.Service, "foo")
	s.createApplication(c, &svc.Service, "bar")

	var id1, id2 string
	setup := func(c *gc.C) {
		u1 := service.AddUnitArg{
			UnitName: ptr("foo/666"),
		}
		u2 := service.AddUnitArg{
			UnitName: ptr("foo/667"),
		}
		u3 := service.AddUnitArg{
			UnitName: ptr("bar/667"),
		}

		ctx := context.Background()
		err := svc.AddUnits(ctx, "foo", u1, u2)
		c.Assert(err, jc.ErrorIsNil)
		err = svc.AddUnits(ctx, "bar", u3)
		c.Assert(err, jc.ErrorIsNil)

		err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/666").Scan(&id1); err != nil {
				return errors.Trace(err)
			}
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/667").Scan(&id2); err != nil {
				return errors.Trace(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	watcher, err := svc.WatchApplicationUnitLife(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness[[]string](s, watchertest.NewWatcherC[[]string](c, watcher))
	harness.AddTest(func(c *gc.C) {
		// First update after creating the units.
		setup(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](id1, id2),
		)
	})
	harness.AddTest(func(c *gc.C) {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name=?", "foo/666"); err != nil {
				return errors.Trace(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](id1),
		)
	})
	harness.AddTest(func(c *gc.C) {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name=?", "foo/666"); err != nil {
				return errors.Trace(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](id1),
		)
	})
	harness.AddTest(func(c *gc.C) {
		// Removing dead unit, no change.
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit WHERE name=?", "foo/666"); err != nil {
				return errors.Trace(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})
	harness.AddTest(func(c *gc.C) {
		// Removing non dead unit - change.
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit WHERE name=?", "foo/667"); err != nil {
				return errors.Trace(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](id2),
		)
	})
	harness.AddTest(func(c *gc.C) {
		// Updating different app unit - no change.
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name=?", "bar/667"); err != nil {
				return errors.Trace(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c)
}
