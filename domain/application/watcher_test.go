// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	domaintesting "github.com/juju/juju/domain/testing"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
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
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchCharm(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "charm")

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchCharms()
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Ensure that we get the charm created event.

	var id corecharm.ID
	harness.AddTest(func(c *gc.C) {
		id, _, err = svc.SetCharm(context.Background(), charm.SetCharmArgs{
			Charm:         &stubCharm{},
			Source:        corecharm.CharmHub,
			ReferenceName: "test",
			Revision:      1,
			Architecture:  arch.AMD64,
			DownloadInfo: &charm.DownloadInfo{
				Provenance:  charm.ProvenanceDownload,
				DownloadURL: "http://example.com",
			},
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(id.String()),
		)
	})

	// Ensure that we get the charm deleted event.

	harness.AddTest(func(c *gc.C) {
		err := svc.DeleteCharm(context.Background(), charm.CharmLocator{
			Name:     "test",
			Revision: 1,
			Source:   charm.CharmHubSource,
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(id.String()),
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

		storageDir := c.MkDir()
		ctx := context.Background()
		err := svc.AddUnits(ctx, storageDir, "foo", nil, u1, u2)
		c.Assert(err, jc.ErrorIsNil)
		err = svc.AddUnits(ctx, storageDir, "bar", nil, u3, u4, u5)
		c.Assert(err, jc.ErrorIsNil)

		err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/666").Scan(&unitID1); err != nil {
				return errors.Capture(err)
			}
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/667").Scan(&unitID2); err != nil {
				return errors.Capture(err)
			}
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "bar/667").Scan(&unitID3); err != nil {
				return errors.Capture(err)
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
				return errors.Capture(err)
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
				return errors.Capture(err)
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
				return errors.Capture(err)
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
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_workload_status WHERE unit_uuid=?", unitID1); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_constraint WHERE unit_uuid=?", unitID1); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit WHERE name=?", "foo/666"); err != nil {
				return errors.Capture(err)
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
				return errors.Capture(err)
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
				return errors.Capture(err)
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
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_workload_status WHERE unit_uuid=?", unitID2); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_constraint WHERE unit_uuid=?", unitID2); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit WHERE name=?", "foo/667"); err != nil {
				return errors.Capture(err)
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
				return errors.Capture(err)
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
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_workload_status WHERE unit_uuid=?", unitID3); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_constraint WHERE unit_uuid=?", unitID3); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit WHERE name=?", "bar/667"); err != nil {
				return errors.Capture(err)
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
				return errors.Capture(err)
			}
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/667").Scan(&unitID2); err != nil {
				return errors.Capture(err)
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

	// Updating the parts of the application table ignored by the mapper doesn't
	// emit an event.
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

	// Add another application with an available charm.
	// Available charms are not pending charms!
	harness.AddTest(func(c *gc.C) {
		id2 = s.createApplicationWithCharmAndStoragePath(c, svc, "jaz", &stubCharm{}, "deadbeef", service.AddUnitArg{
			UnitName: "foo/668",
		})
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string{})
}

func (s *watcherSuite) TestWatchApplication(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application")

	svc := s.setupService(c, factory)

	appName := "foo"
	appUUID := s.createApplication(c, svc, appName)

	ctx := context.Background()
	watcher, err := svc.WatchApplication(ctx, appName)
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness[struct{}](s, watchertest.NewWatcherC[struct{}](c, watcher))

	// Assert that a change to the charm modified version triggers the watcher.
	harness.AddTest(func(c *gc.C) {
		db, err := factory()
		c.Assert(err, jc.ErrorIsNil)

		err = db.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
UPDATE application SET charm_modified_version = 1
WHERE uuid=?`, appUUID)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that a changing the name to itself does not trigger the watcher.
	harness.AddTest(func(c *gc.C) {
		db, err := factory()
		c.Assert(err, jc.ErrorIsNil)

		err = db.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
UPDATE application SET name = ?
WHERE uuid=?`, appName, appUUID)
			return err
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *gc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchApplicationBadName(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application")
	svc := s.setupService(c, factory)

	_, err := svc.WatchApplication(context.Background(), "bad-name")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *watcherSuite) TestWatchApplicationConfig(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application_config_hash")

	svc := s.setupService(c, factory)

	appName := "foo"
	appUUID := s.createApplication(c, svc, appName)

	ctx := context.Background()
	watcher, err := svc.WatchApplicationConfig(ctx, appName)
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness[struct{}](s, watchertest.NewWatcherC[struct{}](c, watcher))

	// Assert that a change to the config triggers the watcher.
	harness.AddTest(func(c *gc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert the same change doesn't trigger a change.
	harness.AddTest(func(c *gc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert multiple changes to the config triggers the watcher.
	harness.AddTest(func(c *gc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, jc.ErrorIsNil)
		err = svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "blah",
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that the trust also triggers the watcher.
	harness.AddTest(func(c *gc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"trust": "true",
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *gc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchApplicationConfigBadName(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application_config_hash")
	svc := s.setupService(c, factory)

	_, err := svc.WatchApplicationConfig(context.Background(), "bad-name")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *watcherSuite) TestWatchApplicationConfigHash(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application_config_hash")

	db, err := factory()
	c.Assert(err, jc.ErrorIsNil)

	svc := s.setupService(c, factory)

	appName := "foo"
	appUUID := s.createApplication(c, svc, appName)

	ctx := context.Background()
	watcher, err := svc.WatchApplicationConfigHash(ctx, appName)
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness[[]string](s, watchertest.NewWatcherC[[]string](c, watcher))

	// Assert that a change to the config triggers the watcher.
	harness.AddTest(func(c *gc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		hash := s.getApplicationConfigHash(c, db, appUUID)
		w.Check(watchertest.StringSliceAssert(hash))
	})

	// Assert the same change doesn't trigger a change.
	harness.AddTest(func(c *gc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert multiple changes to the config triggers the watcher.
	harness.AddTest(func(c *gc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, jc.ErrorIsNil)
		err = svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "blah",
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// We should only see one hash change.
		hash := s.getApplicationConfigHash(c, db, appUUID)
		w.Check(watchertest.StringSliceAssert(hash))
	})

	// Assert that the trust also triggers the watcher.
	harness.AddTest(func(c *gc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"trust": "true",
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// We should only see one hash change.
		hash := s.getApplicationConfigHash(c, db, appUUID)
		w.Check(watchertest.StringSliceAssert(hash))
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *gc.C) {}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	hash := s.getApplicationConfigHash(c, db, appUUID)
	harness.Run(c, []string{hash})
}

func (s *watcherSuite) TestWatchApplicationConfigHashBadName(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application_config_hash")
	svc := s.setupService(c, factory)

	_, err := svc.WatchApplicationConfigHash(context.Background(), "bad-name")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *watcherSuite) TestWatchApplicationExposed(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "v_application_exposed_endpoint")

	svc := s.setupService(c, factory)

	appName := "foo"
	appID := s.createApplication(c, svc, appName)

	ctx := context.Background()
	watcher, err := svc.WatchApplicationExposed(ctx, appName)
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness[struct{}](s, watchertest.NewWatcherC[struct{}](c, watcher))

	// Assert that a change to the exposed endpoints triggers the watcher.
	harness.AddTest(func(c *gc.C) {
		err := svc.MergeExposeSettings(ctx, "foo", map[string]application.ExposedEndpoint{
			"": {
				ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
			},
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Create a new endpoint
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		insertSpace := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err := tx.ExecContext(ctx, insertSpace, "space0-uuid", "space0")
		if err != nil {
			return err
		}
		insertCharm := `INSERT INTO charm (uuid, reference_name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertCharm, "charm0-uuid", "foo-charm")
		if err != nil {
			return err
		}
		insertCharmRelation := `INSERT INTO charm_relation (uuid, charm_uuid, kind_id, scope_id, role_id, name) VALUES (?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertCharmRelation, "charm-relation0-uuid", "charm0-uuid", "0", "0", "0", "endpoint0")
		if err != nil {
			return err
		}
		insertEndpoint := `INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertEndpoint, "app-endpoint0-uuid", appID, "space0-uuid", "charm-relation0-uuid")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	// Assert that a single endpoint exposed to spaces triggers a change.
	harness.AddTest(func(c *gc.C) {
		err := svc.MergeExposeSettings(ctx, "foo", map[string]application.ExposedEndpoint{
			"endpoint0": {
				ExposeToSpaceIDs: set.NewStrings("space0-uuid"),
			},
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert multiple changes to the exposed endpoints triggers the watcher.
	harness.AddTest(func(c *gc.C) {
		err := svc.MergeExposeSettings(ctx, "foo", map[string]application.ExposedEndpoint{
			"endpoint0": {
				ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
			},
		})
		c.Assert(err, jc.ErrorIsNil)
		err = svc.MergeExposeSettings(ctx, "foo", map[string]application.ExposedEndpoint{
			"": {
				ExposeToSpaceIDs: set.NewStrings("space0-uuid"),
			},
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *gc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchApplicationExposedBadName(c *gc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "v_application_exposed_endpoint")
	svc := s.setupService(c, factory)

	_, err := svc.WatchApplicationExposed(context.Background(), "bad-name")
	c.Assert(err, jc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *watcherSuite) getApplicationConfigHash(c *gc.C, db changestream.WatchableDB, appUUID coreapplication.ID) string {
	var hash string
	err := db.StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `SELECT sha256 FROM application_config_hash WHERE application_uuid=?`, appUUID.String())
		err := row.Scan(&hash)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	return hash
}

func (s *watcherSuite) setupService(c *gc.C, factory domain.WatchableDBFactory) *service.WatchableService {
	modelDB := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	notSupportedProviderGetter := func(ctx context.Context) (service.Provider, error) {
		return nil, coreerrors.NotSupported
	}
	notSupportedFeatureProviderGetter := func(ctx context.Context) (service.SupportedFeatureProvider, error) {
		return nil, coreerrors.NotSupported
	}
	notSupportedCAASApplicationproviderGetter := func(ctx context.Context) (service.CAASApplicationProvider, error) {
		return nil, coreerrors.NotSupported
	}

	return service.NewWatchableService(
		state.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		domaintesting.NoopLeaderEnsurer(),
		corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
			return provider.CommonStorageProviders()
		}),
		"",
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		nil, notSupportedProviderGetter,
		notSupportedFeatureProviderGetter, notSupportedCAASApplicationproviderGetter, nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *watcherSuite) createApplication(c *gc.C, svc *service.WatchableService, name string, units ...service.AddUnitArg) coreapplication.ID {
	return s.createApplicationWithCharmAndStoragePath(c, svc, name, &stubCharm{}, "", units...)
}

func (s *watcherSuite) createApplicationWithCharmAndStoragePath(c *gc.C, svc *service.WatchableService, name string, ch internalcharm.Charm, storagePath string, units ...service.AddUnitArg) coreapplication.ID {
	ctx := context.Background()
	appID, err := svc.CreateApplication(ctx, name, ch, corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, service.AddApplicationArgs{
		ReferenceName:    name,
		CharmStoragePath: storagePath,
		DownloadInfo: &charm.DownloadInfo{
			Provenance:  charm.ProvenanceDownload,
			DownloadURL: "http://example.com",
		},
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
	return &internalcharm.Config{
		Options: map[string]internalcharm.Option{
			"foo": {
				Type:    "string",
				Default: "bar",
			},
		},
	}
}

func (s *stubCharm) Actions() *internalcharm.Actions {
	return nil
}

func (s *stubCharm) Revision() int {
	return 0
}

func (s *stubCharm) Version() string {
	return ""
}
