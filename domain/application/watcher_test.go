// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/resolve"
	resolvestate "github.com/juju/juju/domain/resolve/state"
	"github.com/juju/juju/domain/status"
	statusstate "github.com/juju/juju/domain/status/state"
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

func TestWatcherSuite(t *stdtesting.T) { tc.Run(t, &watcherSuite{}) }
func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchCharm(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "charm")

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchCharms(context.Background())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Ensure that we get the charm created event.

	var id corecharm.ID
	harness.AddTest(func(c *tc.C) {
		id, _, err = svc.SetCharm(c.Context(), charm.SetCharmArgs{
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
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(id.String()),
		)
	})

	// Ensure that we get the charm deleted event.

	harness.AddTest(func(c *tc.C) {
		err := svc.DeleteCharm(c.Context(), charm.CharmLocator{
			Name:     "test",
			Revision: 1,
			Source:   charm.CharmHubSource,
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(id.String()),
		)
	})

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchUnitLife(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit")

	svc := s.setupService(c, factory)

	s.createApplication(c, svc, "foo")
	s.createApplication(c, svc, "bar")

	var unitID1, unitID2, unitID3 string
	setup := func(c *tc.C) {

		ctx := c.Context()
		err := svc.AddUnits(ctx, "foo", service.AddUnitArg{}, service.AddUnitArg{})
		c.Assert(err, tc.ErrorIsNil)
		err = svc.AddUnits(ctx, "bar", service.AddUnitArg{}, service.AddUnitArg{}, service.AddUnitArg{})
		c.Assert(err, tc.ErrorIsNil)

		err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/0").Scan(&unitID1); err != nil {
				return errors.Capture(err)
			}
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/1").Scan(&unitID2); err != nil {
				return errors.Capture(err)
			}
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "bar/1").Scan(&unitID3); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}

	watcher, err := svc.WatchApplicationUnitLife(context.Background(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	harness.AddTest(func(c *tc.C) {
		setup(c)
		// Update non app unit first up.
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name=?", "bar/0"); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Initial event after creating the units.
		w.Check(
			watchertest.StringSliceAssert[string](unitID1, unitID2),
		)
	})
	harness.AddTest(func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name=?", "foo/0"); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](unitID1),
		)
	})
	harness.AddTest(func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name=?", "foo/0"); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](unitID1),
		)
	})
	harness.AddTest(func(c *tc.C) {
		// Removing dead unit, no change.
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_agent_status WHERE unit_uuid=?", unitID1); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_workload_status WHERE unit_uuid=?", unitID1); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_workload_version WHERE unit_uuid=?", unitID1); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_constraint WHERE unit_uuid=?", unitID1); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit WHERE name=?", "foo/0"); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})
	harness.AddTest(func(c *tc.C) {
		// Updating different app unit with > 0 app units remaining - no change.
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name=?", "bar/1"); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})
	harness.AddTest(func(c *tc.C) {
		// Removing non app unit - no change.
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name=?", "bar/0"); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})
	harness.AddTest(func(c *tc.C) {
		// Removing non dead unit - change.
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_agent_status WHERE unit_uuid=?", unitID2); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_workload_status WHERE unit_uuid=?", unitID2); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_workload_version WHERE unit_uuid=?", unitID2); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_constraint WHERE unit_uuid=?", unitID2); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit WHERE name=?", "foo/1"); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](unitID2),
		)
	})
	harness.AddTest(func(c *tc.C) {
		// Updating different app unit with no app units remaining - no change.
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name=?", "bar/667"); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})
	harness.AddTest(func(c *tc.C) {
		// Deleting different app unit with no app units remaining - no change.
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_agent_status WHERE unit_uuid=?", unitID3); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_workload_status WHERE unit_uuid=?", unitID3); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM unit_workload_version WHERE unit_uuid=?", unitID3); err != nil {
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
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})
	harness.Run(c, []string{})
}

func (s *watcherSuite) TestWatchUnitLifeInitial(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit")

	svc := s.setupService(c, factory)

	var unitID1, unitID2 string
	setup := func(c *tc.C) {
		s.createApplication(c, svc, "foo", service.AddUnitArg{}, service.AddUnitArg{})
		s.createApplication(c, svc, "bar", service.AddUnitArg{})

		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/0").Scan(&unitID1); err != nil {
				return errors.Capture(err)
			}
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/1").Scan(&unitID2); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)

	}

	watcher, err := svc.WatchApplicationUnitLife(context.Background(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[[]string](s, watchertest.NewWatcherC[[]string](c, watcher))
	harness.AddTest(func(c *tc.C) {
		setup(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](unitID1, unitID2),
		)
	})

	harness.Run(c, []string{})
}

func (s *watcherSuite) TestWatchApplicationScale(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application_scale")

	svc := s.setupService(c, factory)

	s.createApplication(c, svc, "foo")
	s.createApplication(c, svc, "bar")

	ctx := c.Context()
	watcher, err := svc.WatchApplicationScale(ctx, "foo")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[struct{}](s, watchertest.NewWatcherC[struct{}](c, watcher))
	harness.AddTest(func(c *tc.C) {
		// First update after creating the app.
		err = svc.SetApplicationScale(ctx, "foo", 2)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	harness.AddTest(func(c *tc.C) {
		// Update same value.
		err = svc.SetApplicationScale(ctx, "foo", 2)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})
	harness.AddTest(func(c *tc.C) {
		// Update new value.
		err = svc.SetApplicationScale(ctx, "foo", 3)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	harness.AddTest(func(c *tc.C) {
		// Different app.
		err = svc.SetApplicationScale(ctx, "bar", 2)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchApplicationsWithPendingCharms(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application")

	svc := s.setupService(c, factory)

	ctx := c.Context()
	watcher, err := svc.WatchApplicationsWithPendingCharms(ctx)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[[]string](s, watchertest.NewWatcherC[[]string](c, watcher))

	var id0, id1 coreapplication.ID
	harness.AddTest(func(c *tc.C) {
		id0 = s.createApplication(c, svc, "foo")
		id1 = s.createApplication(c, svc, "bar")
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](id0.String(), id1.String()),
		)
	})

	// Updating the charm doesn't emit an event.
	harness.AddTest(func(c *tc.C) {
		db, err := factory()
		c.Assert(err, tc.ErrorIsNil)

		err = db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
UPDATE charm SET available = TRUE
FROM application AS a
INNER JOIN charm AS c ON a.charm_uuid = c.uuid
WHERE a.uuid=?`, id0.String())
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Updating the parts of the application table ignored by the mapper doesn't
	// emit an event.
	harness.AddTest(func(c *tc.C) {
		db, err := factory()
		c.Assert(err, tc.ErrorIsNil)

		err = db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
UPDATE application SET charm_modified_version = 1
WHERE uuid=?`, id0.String())
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Add another application with a pending charm.
	var id2 coreapplication.ID
	harness.AddTest(func(c *tc.C) {
		id2 = s.createApplication(c, svc, "baz")
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](id2.String()),
		)
	})

	// Add another application with an available charm.
	// Available charms are not pending charms!
	harness.AddTest(func(c *tc.C) {
		id2 = s.createApplicationWithCharmAndStoragePath(c, svc, "jaz", &stubCharm{}, "deadbeef", service.AddUnitArg{})
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string{})
}

func (s *watcherSuite) TestWatchApplication(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application")

	svc := s.setupService(c, factory)

	appName := "foo"
	appUUID := s.createApplication(c, svc, appName)

	ctx := c.Context()
	watcher, err := svc.WatchApplication(ctx, appName)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[struct{}](s, watchertest.NewWatcherC[struct{}](c, watcher))

	// Assert that a change to the charm modified version triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		db, err := factory()
		c.Assert(err, tc.ErrorIsNil)

		err = db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
UPDATE application SET charm_modified_version = 1
WHERE uuid=?`, appUUID)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that a changing the name to itself does not trigger the watcher.
	harness.AddTest(func(c *tc.C) {
		db, err := factory()
		c.Assert(err, tc.ErrorIsNil)

		err = db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
UPDATE application SET name = ?
WHERE uuid=?`, appName, appUUID)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchApplicationBadName(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application")
	svc := s.setupService(c, factory)

	_, err := svc.WatchApplication(c.Context(), "bad-name")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *watcherSuite) TestWatchApplicationConfig(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application_config_hash")

	svc := s.setupService(c, factory)

	appName := "foo"
	appUUID := s.createApplication(c, svc, appName)

	ctx := c.Context()
	watcher, err := svc.WatchApplicationConfig(ctx, appName)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[struct{}](s, watchertest.NewWatcherC[struct{}](c, watcher))

	// Assert that a change to the config triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert the same change doesn't trigger a change.
	harness.AddTest(func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert multiple changes to the config triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, tc.ErrorIsNil)
		err = svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "blah",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that the trust also triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"trust": "true",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchApplicationConfigBadName(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application_config_hash")
	svc := s.setupService(c, factory)

	_, err := svc.WatchApplicationConfig(c.Context(), "bad-name")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *watcherSuite) TestWatchApplicationConfigHash(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application_config_hash")

	db, err := factory()
	c.Assert(err, tc.ErrorIsNil)

	svc := s.setupService(c, factory)

	appName := "foo"
	appUUID := s.createApplication(c, svc, appName)

	ctx := c.Context()
	watcher, err := svc.WatchApplicationConfigHash(ctx, appName)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[[]string](s, watchertest.NewWatcherC[[]string](c, watcher))

	// Assert that a change to the config triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		hash := s.getApplicationConfigHash(c, db, appUUID)
		w.Check(watchertest.StringSliceAssert(hash))
	})

	// Assert the same change doesn't trigger a change.
	harness.AddTest(func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert multiple changes to the config triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, tc.ErrorIsNil)
		err = svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "blah",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// We should only see one hash change.
		hash := s.getApplicationConfigHash(c, db, appUUID)
		w.Check(watchertest.StringSliceAssert(hash))
	})

	// Assert that the trust also triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"trust": "true",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// We should only see one hash change.
		hash := s.getApplicationConfigHash(c, db, appUUID)
		w.Check(watchertest.StringSliceAssert(hash))
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *tc.C) {}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	hash := s.getApplicationConfigHash(c, db, appUUID)
	harness.Run(c, []string{hash})
}

func (s *watcherSuite) TestWatchApplicationConfigHashBadName(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application_config_hash")
	svc := s.setupService(c, factory)

	_, err := svc.WatchApplicationConfigHash(c.Context(), "bad-name")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *watcherSuite) TestWatchUnitAddressesHashEmptyInitial(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit_addresses_hash")
	svc := s.setupService(c, factory)

	appName := "foo"
	_ = s.createApplication(c, svc, appName, service.AddUnitArg{})

	ctx := c.Context()
	watcher, err := svc.WatchUnitAddressesHash(ctx, "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC[[]string](c, watcher))

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *tc.C) {}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Hash of the initial state, which only includes the default bindings.
	harness.Run(c, []string{"58a7406eca6cb5e9324e98f37bd09366d2b622027cb07d3a172992106981dedd"})
}

func (s *watcherSuite) TestWatchUnitAddressesHash(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit_addresses_hash")
	svc := s.setupService(c, factory)

	appName := "foo"
	appID := s.createApplication(c, svc, appName, service.AddUnitArg{})
	// Create an ip address for the unit.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertNetNode := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, insertNetNode, "net-node-uuid")
		if err != nil {
			return err
		}
		updateUnit := `UPDATE unit SET net_node_uuid = ? WHERE name = ?`
		_, err = tx.ExecContext(ctx, updateUnit, "net-node-uuid", "foo/0")
		if err != nil {
			return err
		}
		insertLLD := `INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) VALUES (?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertLLD, "lld-uuid", "net-node-uuid", "lld-name", 1500, "00:11:22:33:44:55", 0, 0)
		if err != nil {
			return err
		}
		insertSpace := `INSERT INTO space (uuid, name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertSpace, "space0-uuid", "space0")
		if err != nil {
			return err
		}
		insertSubnet := `INSERT INTO subnet (uuid, cidr, space_uuid) VALUES (?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertSubnet, "subnet-uuid", "10.0.0.0/24", "space0-uuid")
		if err != nil {
			return err
		}
		insertIPAddress := `INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertIPAddress, "ip-address-uuid", "lld-uuid", "10.0.0.1", "net-node-uuid", 0, 0, 0, 0, "subnet-uuid")
		if err != nil {
			return err
		}
		insertCharm := `INSERT INTO charm (uuid, reference_name) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertCharm, "charm0-uuid", "foo-charm")
		if err != nil {
			return err
		}
		insertCharmRelation := `INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name) VALUES (?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertCharmRelation, "charm-relation0-uuid", "charm0-uuid", "0", "0", "endpoint0")
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
	c.Assert(err, tc.ErrorIsNil)

	ctx := c.Context()
	watcher, err := svc.WatchUnitAddressesHash(ctx, "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(func(c *tc.C) {
		// Change the address for that net node should trigger a change.
		err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			updateIPAddress := `UPDATE ip_address SET address_value = ? WHERE uuid = ?`
			_, err = tx.ExecContext(ctx, updateIPAddress, "192.168.0.1", "ip-address-uuid")
			if err != nil {
				return err
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertChange()
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *tc.C) {}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Hash of the initial state.
	harness.Run(c, []string{"eb27bc0dd239e03fd70690f95e3cb9b55013da43cd7606e6c972fb2c3d576f38"})
}

func (s *watcherSuite) TestWatchCloudServiceAddressesHash(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit_addresses_hash")
	svc := s.setupService(c, factory)

	appName := "foo"
	appID := s.createApplication(c, svc, appName, service.AddUnitArg{})

	ctx := c.Context()

	// Add a cloud service to get an initial state.
	err := svc.UpdateCloudService(ctx, "foo", "foo-provider", network.NewSpaceAddresses("10.0.0.1"))
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := svc.WatchUnitAddressesHash(ctx, "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(func(c *tc.C) {
		// Change the address for the cloud service should trigger a change.
		err := svc.UpdateCloudService(ctx, "foo", "foo-provider", network.NewSpaceAddresses("192.168.0.1"))
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertChange()
	})

	harness.AddTest(func(c *tc.C) {
		// Add an endpoint binding should trigger a change.
		err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			insertCharm := `INSERT INTO charm (uuid, reference_name) VALUES (?, ?)`
			_, err = tx.ExecContext(ctx, insertCharm, "charm0-uuid", "foo-charm")
			if err != nil {
				return err
			}
			insertCharmRelation := `INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name) VALUES (?, ?, ?, ?, ?)`
			_, err = tx.ExecContext(ctx, insertCharmRelation, "charm-relation0-uuid", "charm0-uuid", "0", "0", "endpoint0")
			if err != nil {
				return err
			}
			insertEndpoint := `INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, ?, ?)`
			_, err = tx.ExecContext(ctx, insertEndpoint, "app-endpoint0-uuid", appID, network.AlphaSpaceId, "charm-relation0-uuid")
			if err != nil {
				return err
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertChange()
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *tc.C) {}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Hash of the initial state.
	harness.Run(c, []string{"722ba9e367e446b51bd7a473ab0a6002f8eb2f848a03169d7dfa63b7a88e3e8a"})
}

func (s *watcherSuite) TestWatchUnitAddressesHashBadName(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit_addresses_hash")
	svc := s.setupService(c, factory)

	_, err := svc.WatchUnitAddressesHash(c.Context(), "bad-unit-name")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *watcherSuite) TestWatchApplicationExposed(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "v_application_exposed_endpoint")

	svc := s.setupService(c, factory)

	appName := "foo"
	appID := s.createApplication(c, svc, appName)

	ctx := c.Context()
	watcher, err := svc.WatchApplicationExposed(ctx, appName)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[struct{}](s, watchertest.NewWatcherC[struct{}](c, watcher))

	// Assert that a change to the exposed endpoints triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		err := svc.MergeExposeSettings(ctx, "foo", map[string]application.ExposedEndpoint{
			"": {
				ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Create a new endpoint
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
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
		insertCharmRelation := `INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name) VALUES (?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertCharmRelation, "charm-relation0-uuid", "charm0-uuid", "0", "0", "endpoint0")
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
	c.Assert(err, tc.ErrorIsNil)
	// Assert that a single endpoint exposed to spaces triggers a change.
	harness.AddTest(func(c *tc.C) {
		err := svc.MergeExposeSettings(ctx, "foo", map[string]application.ExposedEndpoint{
			"endpoint0": {
				ExposeToSpaceIDs: set.NewStrings("space0-uuid"),
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert multiple changes to the exposed endpoints triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		err := svc.MergeExposeSettings(ctx, "foo", map[string]application.ExposedEndpoint{
			"endpoint0": {
				ExposeToCIDRs: set.NewStrings("10.0.0.0/24", "10.0.1.0/24"),
			},
		})
		c.Assert(err, tc.ErrorIsNil)
		err = svc.MergeExposeSettings(ctx, "foo", map[string]application.ExposedEndpoint{
			"": {
				ExposeToSpaceIDs: set.NewStrings("space0-uuid"),
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchApplicationExposedBadName(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "v_application_exposed_endpoint")
	svc := s.setupService(c, factory)

	_, err := svc.WatchApplicationExposed(c.Context(), "bad-name")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *watcherSuite) TestWatchUnitForLegacyUniter(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.ModelUUID())

	svc := s.setupService(c, factory)

	appName := "foo"
	s.createApplication(c, svc, appName, service.AddUnitArg{}, service.AddUnitArg{})

	ctx := c.Context()

	unitName := unit.Name("foo/0")
	otherUnitName := unit.Name("foo/1")

	unitUUID, err := svc.GetUnitUUID(ctx, unitName)
	c.Assert(err, tc.ErrorIsNil)
	otherUnitUUID, err := svc.GetUnitUUID(ctx, otherUnitName)
	c.Assert(err, tc.ErrorIsNil)

	modelDB := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	statusState := statusstate.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c))
	resolveState := resolvestate.NewState(modelDB)

	alternateCharmID, _, err := svc.SetCharm(c.Context(), charm.SetCharmArgs{
		Charm:         &stubCharm{},
		Source:        corecharm.CharmHub,
		ReferenceName: "alternate",
		Revision:      1,
		Architecture:  arch.AMD64,
		DownloadInfo: &charm.DownloadInfo{
			Provenance:  charm.ProvenanceDownload,
			DownloadURL: "http://example.com",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := svc.WatchUnitForLegacyUniter(ctx, unitName)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Capture the initial event
	harness.AddTest(func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
		w.AssertNoChange()
	})

	// Assert no change is emitted from just changing the status.
	// Conveniently, setting this also allows us to resolve in the next test
	harness.AddTest(func(c *tc.C) {
		statusState.SetUnitAgentStatus(ctx, unitUUID, status.StatusInfo[status.UnitAgentStatusType]{Status: status.UnitAgentStatusError})
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert resolving the unit triggers a change.
	harness.AddTest(func(c *tc.C) {
		err := resolveState.ResolveUnit(ctx, unitUUID, resolve.ResolveModeNoHooks)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that refreshing a unit's charm triggers a change.
	// NOTE: refresh has not been implemented yet, so change the charm_uuid value
	// manually
	harness.AddTest(func(c *tc.C) {
		stmt := `UPDATE unit SET charm_uuid = ? WHERE uuid = ?`
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, stmt, alternateCharmID, unitUUID)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that adding a subordinate unit triggers a change
	// NOTE: subordinate units have not been implemented yet, so insert directly into
	// the unit_principal table
	harness.AddTest(func(c *tc.C) {
		stmt := `INSERT INTO unit_principal (unit_uuid, principal_uuid) VALUES (?, ?)`
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, stmt, otherUnitUUID, unitUUID)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that removing a subordinate unit triggers a change
	harness.AddTest(func(c *tc.C) {
		stmt := `DELETE FROM unit_principal`
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, stmt)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that changing the life of a unit triggers a change
	harness.AddTest(func(c *tc.C) {
		err := svc.EnsureUnitDead(ctx, unitName, noOpRevoker{})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that refreshing another unit's charm does not trigger a change.
	harness.AddTest(func(c *tc.C) {
		stmt := `UPDATE unit SET charm_uuid = ? WHERE uuid = ?`
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, stmt, alternateCharmID, otherUnitUUID)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchUnitForLegacyUniterBadName(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.ModelUUID())
	svc := s.setupService(c, factory)

	_, err := svc.WatchUnitForLegacyUniter(c.Context(), unit.Name("foo/0"))
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *watcherSuite) TestWatchNetNodeAddress(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "ip_address")

	svc := s.setupService(c, factory)

	ctx := context.Background()

	// Insert a net node first.
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		insertNetNode0 := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, insertNetNode0, "net-node-uuid")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := svc.WatchNetNodeAddress(ctx, "net-node-uuid")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Assert that an insertion to the net node address triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			insertLLD := `INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) VALUES (?, ?, ?, ?, ?, ?, ?)`
			_, err = tx.ExecContext(ctx, insertLLD, "lld0-uuid", "net-node-uuid", "lld0-name", 1500, "00:11:22:33:44:55", 0, 0)
			if err != nil {
				return err
			}
			insertSpace := `INSERT INTO space (uuid, name) VALUES (?, ?)`
			_, err = tx.ExecContext(ctx, insertSpace, "space0-uuid", "space0")
			if err != nil {
				return err
			}
			insertSubnet := `INSERT INTO subnet (uuid, cidr, space_uuid) VALUES (?, ?, ?)`
			_, err = tx.ExecContext(ctx, insertSubnet, "subnet-uuid", "10.0.0.0/24", "space0-uuid")
			if err != nil {
				return err
			}
			insertIPAddress := `INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
			_, err = tx.ExecContext(ctx, insertIPAddress, "ip-address0-uuid", "lld0-uuid", "10.0.0.1", "net-node-uuid", 0, 3, 1, 1, "subnet-uuid")
			if err != nil {
				return err
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that a change of value to the net node address triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			updateIPAddress := `UPDATE ip_address SET address_value = ? WHERE net_node_uuid = ?`
			_, err = tx.ExecContext(ctx, updateIPAddress, "10.0.0.255", "net-node-uuid")
			if err != nil {
				return err
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that a change of scope to the net node address triggers the watcher.
	harness.AddTest(func(c *tc.C) {
		err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			updateIPAddress := `UPDATE ip_address SET scope_id = ? WHERE net_node_uuid = ?`
			_, err = tx.ExecContext(ctx, updateIPAddress, 1, "net-node-uuid")
			if err != nil {
				return err
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) getApplicationConfigHash(c *tc.C, db changestream.WatchableDB, appUUID coreapplication.ID) string {
	var hash string
	err := db.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `SELECT sha256 FROM application_config_hash WHERE application_uuid=?`, appUUID.String())
		err := row.Scan(&hash)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return hash
}

func (s *watcherSuite) setupService(c *tc.C, factory domain.WatchableDBFactory) *service.WatchableService {
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

func (s *watcherSuite) createApplication(c *tc.C, svc *service.WatchableService, name string, units ...service.AddUnitArg) coreapplication.ID {
	return s.createApplicationWithCharmAndStoragePath(c, svc, name, &stubCharm{}, "", units...)
}

func (s *watcherSuite) createApplicationWithCharmAndStoragePath(c *tc.C, svc *service.WatchableService, name string, ch internalcharm.Charm, storagePath string, units ...service.AddUnitArg) coreapplication.ID {
	ctx := c.Context()
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
	c.Assert(err, tc.ErrorIsNil)
	return appID
}

type stubCharm struct {
	name        string
	subordinate bool
}

func (s *stubCharm) Meta() *internalcharm.Meta {
	name := s.name
	if name == "" {
		name = "test"
	}
	return &internalcharm.Meta{
		Name:        name,
		Subordinate: s.subordinate,
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

type noOpRevoker struct{}

func (noOpRevoker) RevokeLeadership(applicationName string, unitName unit.Name) error {
	return nil
}
