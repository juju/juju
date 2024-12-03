// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	corestorage "github.com/juju/juju/core/storage"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/resource"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	secretstate "github.com/juju/juju/domain/secret/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	internalcharm "github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, target_agent_version, name, type, cloud, cloud_type)
			VALUES (?, ?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id(), jujuversion.Current.String())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchCharm(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "charm")

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchCharms()
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness[[]string](s, watchertest.NewWatcherC[[]string](c, watcher))

	// Ensure that we get the charm created event.

	var id corecharm.ID
	harness.AddTest(func(c *gc.C) {
		id, _, err = svc.SetCharm(context.Background(), charm.SetCharmArgs{
			Charm:         &stubCharm{},
			Source:        corecharm.CharmHub,
			ReferenceName: "test",
			Revision:      1,
			Architecture:  arch.AMD64,
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

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchUnitLife(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit")

	svc := s.setupService(c, factory)

	s.createApplication(c, svc, "foo")
	s.createApplication(c, svc, "bar")

	var unitID1, unitID2, unitID3 string
	setup := func(c *gc.C) {
		u1 := service.AddUnitArg{
			UnitName: "foo/666",
		}
		u2 := service.AddUnitArg{
			UnitName: "foo/667",
		}
		u3 := service.AddUnitArg{
			UnitName: "bar/666",
		}
		u4 := service.AddUnitArg{
			UnitName: "bar/667",
		}
		u5 := service.AddUnitArg{
			UnitName: "bar/668",
		}

		ctx := context.Background()
		err := svc.AddUnits(ctx, "foo", u1, u2)
		c.Assert(err, jc.ErrorIsNil)
		err = svc.AddUnits(ctx, "bar", u3, u4, u5)
		c.Assert(err, jc.ErrorIsNil)

		err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/666").Scan(&unitID1); err != nil {
				return errors.Trace(err)
			}
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/667").Scan(&unitID2); err != nil {
				return errors.Trace(err)
			}
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "bar/667").Scan(&unitID3); err != nil {
				return errors.Trace(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	watcher, err := svc.WatchApplicationUnitLife("foo")
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness[[]string](s, watchertest.NewWatcherC[[]string](c, watcher))
	harness.AddTest(func(c *gc.C) {
		setup(c)
		// Update non app unit first up.
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name=?", "bar/668"); err != nil {
				return errors.Trace(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Initial event after creating the units.
		w.Check(
			watchertest.StringSliceAssert[string](unitID1, unitID2),
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
			watchertest.StringSliceAssert[string](unitID1),
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
			watchertest.StringSliceAssert[string](unitID1),
		)
	})
	harness.AddTest(func(c *gc.C) {
		// Removing dead unit, no change.
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_agent_status WHERE unit_uuid=?", unitID1); err != nil {
				return errors.Trace(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_workload_status WHERE unit_uuid=?", unitID1); err != nil {
				return errors.Trace(err)
			}
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
		// Updating different app unit with > 0 app units remaining - no change.
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
	harness.AddTest(func(c *gc.C) {
		// Removing non app unit - no change.
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name=?", "bar/666"); err != nil {
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
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_agent_status WHERE unit_uuid=?", unitID2); err != nil {
				return errors.Trace(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_workload_status WHERE unit_uuid=?", unitID2); err != nil {
				return errors.Trace(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit WHERE name=?", "foo/667"); err != nil {
				return errors.Trace(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](unitID2),
		)
	})
	harness.AddTest(func(c *gc.C) {
		// Updating different app unit with no app units remaining - no change.
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
	harness.AddTest(func(c *gc.C) {
		// Deleting different app unit with no app units remaining - no change.
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_agent_status WHERE unit_uuid=?", unitID3); err != nil {
				return errors.Trace(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_workload_status WHERE unit_uuid=?", unitID3); err != nil {
				return errors.Trace(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit WHERE name=?", "bar/667"); err != nil {
				return errors.Trace(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})
	harness.Run(c, []string{})
}

func (s *watcherSuite) TestWatchUnitLifeInitial(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit")

	svc := s.setupService(c, factory)

	var unitID1, unitID2 string
	setup := func(c *gc.C) {
		u1 := service.AddUnitArg{
			UnitName: "foo/666",
		}
		u2 := service.AddUnitArg{
			UnitName: "foo/667",
		}
		u3 := service.AddUnitArg{
			UnitName: "bar/666",
		}
		s.createApplication(c, svc, "foo", u1, u2)
		s.createApplication(c, svc, "bar", u3)

		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/666").Scan(&unitID1); err != nil {
				return errors.Trace(err)
			}
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/667").Scan(&unitID2); err != nil {
				return errors.Trace(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)

	}

	watcher, err := svc.WatchApplicationUnitLife("foo")
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness[[]string](s, watchertest.NewWatcherC[[]string](c, watcher))
	harness.AddTest(func(c *gc.C) {
		setup(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](unitID1, unitID2),
		)
	})

	harness.Run(c, []string{})
}

func (s *watcherSuite) TestWatchApplicationScale(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application_scale")

	svc := s.setupService(c, factory)

	s.createApplication(c, svc, "foo")
	s.createApplication(c, svc, "bar")

	ctx := context.Background()
	watcher, err := svc.WatchApplicationScale(ctx, "foo")
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness[struct{}](s, watchertest.NewWatcherC[struct{}](c, watcher))
	harness.AddTest(func(c *gc.C) {
		// First update after creating the app.
		err = svc.SetApplicationScale(ctx, "foo", 2)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	harness.AddTest(func(c *gc.C) {
		// Update same value.
		err = svc.SetApplicationScale(ctx, "foo", 2)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})
	harness.AddTest(func(c *gc.C) {
		// Update new value.
		err = svc.SetApplicationScale(ctx, "foo", 3)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	harness.AddTest(func(c *gc.C) {
		// Different app.
		err = svc.SetApplicationScale(ctx, "bar", 2)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchApplicationsWithPendingCharms(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application")

	svc := s.setupService(c, factory)

	ctx := context.Background()
	watcher, err := svc.WatchApplicationsWithPendingCharms(ctx)
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness[[]string](s, watchertest.NewWatcherC[[]string](c, watcher))

	var id0, id1 coreapplication.ID
	harness.AddTest(func(c *gc.C) {
		id0 = s.createApplication(c, svc, "foo")
		id1 = s.createApplication(c, svc, "bar")
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](id0.String(), id1.String()),
		)
	})

	// Updating the charm doesn't emit an event.
	harness.AddTest(func(c *gc.C) {
		db, err := factory()
		c.Assert(err, jc.ErrorIsNil)

		err = db.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
UPDATE charm SET available = TRUE
FROM application AS a
INNER JOIN charm AS c ON a.charm_uuid = c.uuid
WHERE a.uuid=?`, id0.String())
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Updating the application doesn't emit an event.
	harness.AddTest(func(c *gc.C) {
		db, err := factory()
		c.Assert(err, jc.ErrorIsNil)

		err = db.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
UPDATE application SET charm_modified_version = 1
WHERE uuid=?`, id0.String())
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Add another application with a pending charm.
	var id2 coreapplication.ID
	harness.AddTest(func(c *gc.C) {
		id2 = s.createApplication(c, svc, "baz")
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](id2.String()),
		)
	})

	// Add another application with a available charm.
	// Available charms are not pending charms!
	harness.AddTest(func(c *gc.C) {
		id2 = s.createApplicationWithCharmAndStoragePath(c, svc, "jaz", &stubCharm{}, "deadbeef", service.AddUnitArg{})
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string{})
}

func (s *watcherSuite) setupService(c *gc.C, factory domain.WatchableDBFactory) *service.WatchableService {
	modelDB := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	return service.NewWatchableService(
		state.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		secretstate.NewState(modelDB, loggertesting.WrapCheckLog(c)),
		corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
			return provider.CommonStorageProviders()
		}),
		resource.NewResourceStoreFactory(nil),
		"",
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		nil, nil, nil,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *watcherSuite) createApplication(c *gc.C, svc *service.WatchableService, name string, units ...service.AddUnitArg) coreapplication.ID {
	return s.createApplicationWithCharmAndStoragePath(c, svc, name, &stubCharm{}, "", units...)
}

func (s *watcherSuite) createApplicationWithCharmAndStoragePath(c *gc.C, svc *service.WatchableService, name string, charm internalcharm.Charm, storagePath string, units ...service.AddUnitArg) coreapplication.ID {
	ctx := context.Background()
	appID, err := svc.CreateApplication(ctx, name, charm, corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, service.AddApplicationArgs{
		ReferenceName:    name,
		CharmStoragePath: storagePath,
	}, units...)
	c.Assert(err, jc.ErrorIsNil)
	return appID
}

type stubCharm struct {
	name string
}

func (s *stubCharm) Meta() *internalcharm.Meta {
	name := s.name
	if name == "" {
		name = "test"
	}
	return &internalcharm.Meta{
		Name: name,
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
