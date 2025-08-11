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
	"github.com/juju/juju/core/instance"
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
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	machineservice "github.com/juju/juju/domain/machine/service"
	machinestate "github.com/juju/juju/domain/machine/state"
	domainnetwork "github.com/juju/juju/domain/network"
	removalstatemodel "github.com/juju/juju/domain/removal/state/model"
	"github.com/juju/juju/domain/resolve"
	resolvestate "github.com/juju/juju/domain/resolve/state"
	"github.com/juju/juju/domain/status"
	statusstate "github.com/juju/juju/domain/status/state"
	domaintesting "github.com/juju/juju/domain/testing"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite
}

func TestWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "prod",  "iaas", "test-model", "ec2")
		`, modelUUID.String(), testing.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) TestWatchCharm(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "charm")

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchCharms(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	st := state.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c))

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Ensure that we get the charm created event.

	var id corecharm.ID
	harness.AddTest(c, func(c *tc.C) {
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

	// Ensure that we get the charm created event when an application is created.

	var appID coreapplication.ID
	harness.AddTest(c, func(c *tc.C) {
		appID = s.createIAASApplication(c, svc, "foo")
		id, err = st.GetCharmIDByApplicationName(c.Context(), "foo")
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(id.String()),
		)
	})

	// Ensure that we get the charm deleted event.
	//
	// We do this by removing the application, since this will trigger a cleanup
	// of the charm.

	removalSt := removalstatemodel.NewState(modelDB, loggertesting.WrapCheckLog(c))
	harness.AddTest(c, func(c *tc.C) {
		_, _, err := removalSt.EnsureApplicationNotAliveCascade(c.Context(), appID.String())
		c.Assert(err, tc.ErrorIsNil)
		err = removalSt.DeleteApplication(c.Context(), appID.String())
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(id.String()),
		)
	})

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchApplicationUnitLife(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit")

	svc := s.setupService(c, factory)

	s.createIAASApplication(c, svc, "foo")
	s.createIAASApplication(c, svc, "bar")

	var unitID1, unitID2, unitID3 string
	setup := func(c *tc.C) {

		ctx := c.Context()
		_, _, err := svc.AddIAASUnits(ctx, "foo", service.AddIAASUnitArg{}, service.AddIAASUnitArg{})
		c.Assert(err, tc.ErrorIsNil)
		_, _, err = svc.AddIAASUnits(ctx, "bar", service.AddIAASUnitArg{}, service.AddIAASUnitArg{}, service.AddIAASUnitArg{})
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

	watcher, err := svc.WatchApplicationUnitLife(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
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

func (s *watcherSuite) TestWatchApplicationUnitLifeInitial(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit")

	svc := s.setupService(c, factory)

	var unitID1, unitID2 string
	setup := func(c *tc.C) {
		s.createIAASApplication(c, svc, "foo", service.AddIAASUnitArg{}, service.AddIAASUnitArg{})
		s.createIAASApplication(c, svc, "bar", service.AddIAASUnitArg{})

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

	watcher, err := svc.WatchApplicationUnitLife(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[[]string](s, watchertest.NewWatcherC[[]string](c, watcher))
	harness.AddTest(c, func(c *tc.C) {
		setup(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](unitID1, unitID2),
		)
	})

	harness.Run(c, []string{})
}

func (s *watcherSuite) TestWatchUnitLife(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit")

	svc := s.setupService(c, factory)
	s.createIAASApplication(c, svc, "foo", service.AddIAASUnitArg{}, service.AddIAASUnitArg{})

	watcher, err := svc.WatchUnitLife(c.Context(), unit.Name("foo/0"))
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	harness.AddTest(c, func(c *tc.C) {
		// Test unit life going to dying fires.
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name=?", "foo/0"); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		// Test unit life going to dead fires.
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 2 WHERE name=?", "foo/0"); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		// Test unit removal fires.
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			var unitID1 string
			if err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name=?", "foo/0").Scan(&unitID1); err != nil {
				return errors.Capture(err)
			}
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
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		// Test annother unit life going to dying does not fire.
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE name=?", "foo/1"); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})
	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchApplicationScale(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application_scale")

	svc := s.setupService(c, factory)

	s.createCAASApplication(c, svc, "foo")
	s.createCAASApplication(c, svc, "bar")

	ctx := c.Context()
	watcher, err := svc.WatchApplicationScale(ctx, "foo")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[struct{}](s, watchertest.NewWatcherC[struct{}](c, watcher))
	harness.AddTest(c, func(c *tc.C) {
		// First update after creating the app.
		err = svc.SetApplicationScale(ctx, "foo", 2)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	harness.AddTest(c, func(c *tc.C) {
		// Update same value.
		err = svc.SetApplicationScale(ctx, "foo", 2)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})
	harness.AddTest(c, func(c *tc.C) {
		// Update new value.
		err = svc.SetApplicationScale(ctx, "foo", 3)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
		id0 = s.createIAASApplication(c, svc, "foo")
		id1 = s.createIAASApplication(c, svc, "bar")
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](id0.String(), id1.String()),
		)
	})

	// Updating the charm doesn't emit an event.
	harness.AddTest(c, func(c *tc.C) {
		db, err := factory(c.Context())
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
	harness.AddTest(c, func(c *tc.C) {
		db, err := factory(c.Context())
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
	harness.AddTest(c, func(c *tc.C) {
		id2 = s.createIAASApplication(c, svc, "baz")
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](id2.String()),
		)
	})

	// Add another application with an available charm.
	// Available charms are not pending charms!
	harness.AddTest(c, func(c *tc.C) {
		id2 = s.createIAASApplicationWithCharmAndStoragePath(c, svc, "jaz", &stubCharm{}, "deadbeef", service.AddIAASUnitArg{})
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string{})
}

func (s *watcherSuite) TestWatchApplication(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application")

	svc := s.setupService(c, factory)

	appName := "foo"
	appUUID := s.createIAASApplication(c, svc, appName)

	ctx := c.Context()
	watcher, err := svc.WatchApplication(ctx, appName)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[struct{}](s, watchertest.NewWatcherC[struct{}](c, watcher))

	// Assert that a change to the charm modified version triggers the watcher.
	harness.AddTest(c, func(c *tc.C) {
		db, err := factory(c.Context())
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
	harness.AddTest(c, func(c *tc.C) {
		db, err := factory(c.Context())
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
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
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
	appUUID := s.createIAASApplication(c, svc, appName)

	ctx := c.Context()
	watcher, err := svc.WatchApplicationConfig(ctx, appName)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[struct{}](s, watchertest.NewWatcherC[struct{}](c, watcher))

	// Assert that a change to the config triggers the watcher.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert the same change doesn't trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert multiple changes to the config triggers the watcher.
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"trust": "true",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that nothing changes if nothing happens.
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
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

	db, err := factory(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	svc := s.setupService(c, factory)

	appName := "foo"
	appUUID := s.createIAASApplication(c, svc, appName)

	ctx := c.Context()
	watcher, err := svc.WatchApplicationConfigHash(ctx, appName)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[[]string](s, watchertest.NewWatcherC[[]string](c, watcher))

	// Assert that a change to the config triggers the watcher.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		hash := s.getApplicationConfigHash(c, db, appUUID)
		w.Check(watchertest.StringSliceAssert(hash))
	})

	// Assert the same change doesn't trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "baz",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert multiple changes to the config triggers the watcher.
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[[]string]) {
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

func (s *watcherSuite) TestWatchApplicationSettings(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application_setting")
	svc := s.setupService(c, factory)

	appName := "foo"
	appUUID := s.createIAASApplication(c, svc, appName)

	ctx := c.Context()
	watcher, err := svc.WatchApplicationSettings(ctx, appName)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Assert that a change to the settings triggers the watcher.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"trust": "true",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert no change is emitted is we change a config value.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"foo": "bar",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert no change is emitted if we update a setting to the same value.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.UpdateApplicationConfig(ctx, appUUID, map[string]string{
			"trust": "true",
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchApplicationSettingsBadName(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application_setting")
	svc := s.setupService(c, factory)

	_, err := svc.WatchApplicationSettings(c.Context(), "bad-name")
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *watcherSuite) TestWatchUnitAddressesHashEmptyInitial(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit_addresses_hash")
	svc := s.setupService(c, factory)

	appName := "foo"
	_ = s.createIAASApplication(c, svc, appName, service.AddIAASUnitArg{})

	ctx := c.Context()
	watcher, err := svc.WatchUnitAddressesHash(ctx, "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC[[]string](c, watcher))

	// Assert that nothing changes if nothing happens.
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Hash of the initial state, which only includes the default bindings.
	harness.Run(c, []string{"58a7406eca6cb5e9324e98f37bd09366d2b622027cb07d3a172992106981dedd"})
}

func (s *watcherSuite) TestWatchUnitAddressesHash(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit_addresses_hash")
	svc := s.setupService(c, factory)

	appName := "foo"
	appID := s.createIAASApplication(c, svc, appName, service.AddIAASUnitArg{})
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

	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Hash of the initial state.
	harness.Run(c, []string{"eb27bc0dd239e03fd70690f95e3cb9b55013da43cd7606e6c972fb2c3d576f38"})
}

func (s *watcherSuite) TestWatchCloudServiceAddressesHash(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit_addresses_hash")
	svc := s.setupService(c, factory)

	appName := "foo"
	appID := s.createCAASApplication(c, svc, appName, service.AddUnitArg{})

	ctx := c.Context()

	// Add a cloud service to get an initial state.
	err := svc.UpdateCloudService(ctx, "foo", "foo-provider", network.ProviderAddresses{
		{
			MachineAddress: network.NewMachineAddress("10.0.0.1"),
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := svc.WatchUnitAddressesHash(ctx, "foo/0")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
		// Change the address for the cloud service should trigger a change.
		err := svc.UpdateCloudService(ctx, "foo", "foo-provider", network.ProviderAddresses{
			{
				MachineAddress: network.NewMachineAddress("192.168.0.1"),
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[[]string]) {
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

func (s *watcherSuite) TestWatchUnitAddRemoveOnMachineInitialEvents(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit_insert")
	svc := s.setupService(c, factory)

	s.createIAASApplication(c, svc, "foo",
		service.AddIAASUnitArg{},
		service.AddIAASUnitArg{},
		service.AddIAASUnitArg{
			AddUnitArg: service.AddUnitArg{
				Placement: &instance.Placement{Scope: instance.MachineScope, Directive: "0"},
			},
		},
	)

	ctx := c.Context()
	watcher, err := svc.WatchUnitAddRemoveOnMachine(ctx, "0")
	c.Assert(err, tc.ErrorIsNil)

	select {
	case initial := <-watcher.Changes():
		c.Assert(initial, tc.SameContents, []string{"foo/0", "foo/2"})
	case <-ctx.Done():
		c.Fatalf("timed out waiting for initial change")
	}
}

func (s *watcherSuite) TestWatchUnitAddRemoveOnMachine(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "unit_insert")
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	svc := s.setupService(c, factory)
	st := state.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c))
	machineSvc := machineservice.NewProviderService(
		machinestate.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		func(ctx context.Context) (machineservice.Provider, error) {
			return machineservice.NewNoopProvider(), nil
		},
		nil,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	removalSt := removalstatemodel.NewState(modelDB, loggertesting.WrapCheckLog(c))

	res0, err := machineSvc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	res1, err := machineSvc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	ctx := c.Context()
	watcher, err := svc.WatchUnitAddRemoveOnMachine(ctx, res0.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
		s.createIAASApplication(c, svc, "foo",
			service.AddIAASUnitArg{
				AddUnitArg: service.AddUnitArg{
					Placement: &instance.Placement{Scope: instance.MachineScope, Directive: res0.MachineName.String()},
				},
			},
			service.AddIAASUnitArg{
				AddUnitArg: service.AddUnitArg{
					Placement: &instance.Placement{Scope: instance.MachineScope, Directive: res1.MachineName.String()},
				},
			},
			service.AddIAASUnitArg{
				AddUnitArg: service.AddUnitArg{
					Placement: &instance.Placement{Scope: instance.MachineScope, Directive: res0.MachineName.String()},
				},
			},
		)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"foo/0", "foo/2"}))
	})

	harness.AddTest(c, func(c *tc.C) {
		unitUUID, err := st.GetUnitUUIDByName(c.Context(), "foo/0")
		c.Assert(err, tc.ErrorIsNil)
		_, err = removalSt.EnsureUnitNotAliveCascade(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"foo/0"}))
	})

	harness.AddTest(c, func(c *tc.C) {
		unitUUID, err := st.GetUnitUUIDByName(c.Context(), "foo/0")
		c.Assert(err, tc.ErrorIsNil)
		_, err = removalSt.EnsureUnitNotAliveCascade(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
		err = removalSt.MarkUnitAsDead(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
		err = removalSt.DeleteUnit(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"foo/0"}))
	})

	harness.AddTest(c, func(c *tc.C) {
		unitUUID, err := st.GetUnitUUIDByName(c.Context(), "foo/1")
		c.Assert(err, tc.ErrorIsNil)
		_, err = removalSt.EnsureUnitNotAliveCascade(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
		err = removalSt.MarkUnitAsDead(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
		err = removalSt.DeleteUnit(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string{})
}

func (s *watcherSuite) TestWatchUnitAddRemoveOnMachineSubordinates(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "custom_unit_name_lifecycle")
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	svc := s.setupService(c, factory)
	st := state.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c))
	machineSvc := machineservice.NewProviderService(
		machinestate.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		func(ctx context.Context) (machineservice.Provider, error) {
			return machineservice.NewNoopProvider(), nil
		},
		nil,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	removalSt := removalstatemodel.NewState(modelDB, loggertesting.WrapCheckLog(c))

	res0, err := machineSvc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	res1, err := machineSvc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	ctx := c.Context()
	watcher, err := svc.WatchUnitAddRemoveOnMachine(ctx, res0.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
		s.createIAASApplication(c, svc, "foo",
			service.AddIAASUnitArg{
				AddUnitArg: service.AddUnitArg{
					Placement: &instance.Placement{Scope: instance.MachineScope, Directive: res0.MachineName.String()},
				},
			},
			service.AddIAASUnitArg{
				AddUnitArg: service.AddUnitArg{
					Placement: &instance.Placement{Scope: instance.MachineScope, Directive: res1.MachineName.String()},
				},
			},
		)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"foo/0"}))
	})

	var subordinateAppID coreapplication.ID
	harness.AddTest(c, func(c *tc.C) {
		subordinateAppID = s.createIAASApplicationWithCharmAndStoragePath(c, svc, "bar", &stubCharm{subordinate: true}, "deadbeef")
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddIAASSubordinateUnit(ctx, subordinateAppID, "foo/0")
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"bar/0"}))
	})

	harness.AddTest(c, func(c *tc.C) {
		err := svc.AddIAASSubordinateUnit(ctx, subordinateAppID, "foo/1")
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		unitUUID, err := st.GetUnitUUIDByName(c.Context(), "bar/0")
		c.Assert(err, tc.ErrorIsNil)
		_, err = removalSt.EnsureUnitNotAliveCascade(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
		err = removalSt.MarkUnitAsDead(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
		err = removalSt.DeleteUnit(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"bar/0"}))
	})

	harness.AddTest(c, func(c *tc.C) {
		unitUUID, err := st.GetUnitUUIDByName(c.Context(), "bar/1")
		c.Assert(err, tc.ErrorIsNil)
		_, err = removalSt.EnsureUnitNotAliveCascade(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
		err = removalSt.MarkUnitAsDead(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
		err = removalSt.DeleteUnit(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string{})
}

func (s *watcherSuite) TestWatchUnitAddRemoveOnMachineBadName(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "custom_unit_name_lifecycle")
	svc := s.setupService(c, factory)

	_, err := svc.WatchUnitAddRemoveOnMachine(c.Context(), "bad-name")
	c.Assert(err, tc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *watcherSuite) TestWatchApplicationsInitialEvent(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application")
	svc := s.setupService(c, factory)

	app1 := s.createCAASApplication(c, svc, "foo")
	app2 := s.createCAASApplication(c, svc, "bar")

	ctx := c.Context()
	watcher, err := svc.WatchApplications(ctx)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case initial := <-watcher.Changes():
		c.Assert(initial, tc.SameContents, []string{app1.String(), app2.String()})
	case <-ctx.Done():
		c.Fatalf("timed out waiting for initial change")
	}
}

func (s *watcherSuite) TestWatchApplications(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "application")
	svc := s.setupService(c, factory)

	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	removalSt := removalstatemodel.NewState(modelDB, loggertesting.WrapCheckLog(c))

	ctx := c.Context()
	watcher, err := svc.WatchApplications(ctx)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	var appID coreapplication.ID
	harness.AddTest(c, func(c *tc.C) {
		appID = s.createCAASApplication(c, svc, "foo")
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{appID.String()}))
	})

	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
	UPDATE application SET name = ?
	WHERE uuid=?`, "bar", appID)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{appID.String()}))
	})

	harness.AddTest(c, func(c *tc.C) {
		_, _, err := removalSt.EnsureApplicationNotAliveCascade(c.Context(), appID.String())
		c.Assert(err, tc.ErrorIsNil)
		err = removalSt.DeleteApplication(c.Context(), appID.String())
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{appID.String()}))
	})

	harness.Run(c, []string{})
}

func (s *watcherSuite) TestWatchApplicationExposed(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "v_application_exposed_endpoint")

	svc := s.setupService(c, factory)

	appName := "foo"
	appID := s.createIAASApplication(c, svc, appName)

	ctx := c.Context()
	watcher, err := svc.WatchApplicationExposed(ctx, appName)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness[struct{}](s, watchertest.NewWatcherC[struct{}](c, watcher))

	// Assert that a change to the exposed endpoints triggers the watcher.
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
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
	s.createIAASApplication(c, svc, appName, service.AddIAASUnitArg{}, service.AddIAASUnitArg{})

	ctx := c.Context()

	unitName := unit.Name("foo/0")
	otherUnitName := unit.Name("foo/1")

	unitUUID, err := svc.GetUnitUUID(ctx, unitName)
	c.Assert(err, tc.ErrorIsNil)
	otherUnitUUID, err := svc.GetUnitUUID(ctx, otherUnitName)
	c.Assert(err, tc.ErrorIsNil)

	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	statusState := statusstate.NewModelState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c))
	resolveState := resolvestate.NewState(modelDB)
	removalSt := removalstatemodel.NewState(modelDB, loggertesting.WrapCheckLog(c))

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
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
		w.AssertNoChange()
	})

	// Assert no change is emitted from just changing the status.
	// Conveniently, setting this also allows us to resolve in the next test
	harness.AddTest(c, func(c *tc.C) {
		statusState.SetUnitAgentStatus(ctx, unitUUID, status.StatusInfo[status.UnitAgentStatusType]{Status: status.UnitAgentStatusError})
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Assert resolving the unit triggers a change.
	harness.AddTest(c, func(c *tc.C) {
		err := resolveState.ResolveUnit(ctx, unitUUID, resolve.ResolveModeNoHooks)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that refreshing a unit's charm triggers a change.
	// NOTE: refresh has not been implemented yet, so change the charm_uuid value
	// manually
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {
		_, err := removalSt.EnsureUnitNotAliveCascade(ctx, unitUUID.String())
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Assert that refreshing another unit's charm does not trigger a change.
	harness.AddTest(c, func(c *tc.C) {
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
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
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

func (s *watcherSuite) TestWatchUnitAddresses(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "ip_address")

	svc := s.setupService(c, factory)

	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	s.createIAASApplication(c, svc, "foo", service.AddIAASUnitArg{})

	// Insert a net node first.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		insertNetNode0 := `INSERT INTO net_node (uuid) VALUES (?)`
		_, err := tx.ExecContext(ctx, insertNetNode0, netNodeUUID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE unit SET net_node_uuid = ? WHERE name = ?", netNodeUUID, "foo/0")
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := svc.WatchUnitAddresses(c.Context(), unit.Name("foo/0"))
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Assert that an insertion to the net node address triggers the watcher.
	harness.AddTest(c, func(c *tc.C) {
		err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			insertLLD := `INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) VALUES (?, ?, ?, ?, ?, ?, ?)`
			_, err = tx.ExecContext(ctx, insertLLD, "lld0-uuid", netNodeUUID, "lld0-name", 1500, "00:11:22:33:44:55", 0, 0)
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
			_, err = tx.ExecContext(ctx, insertIPAddress, "ip-address0-uuid", "lld0-uuid", "10.0.0.1", netNodeUUID, 0, 3, 1, 1, "subnet-uuid")
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
	harness.AddTest(c, func(c *tc.C) {
		err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			updateIPAddress := `UPDATE ip_address SET address_value = ? WHERE net_node_uuid = ?`
			_, err = tx.ExecContext(ctx, updateIPAddress, "10.0.0.255", netNodeUUID)
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
	harness.AddTest(c, func(c *tc.C) {
		err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			updateIPAddress := `UPDATE ip_address SET scope_id = ? WHERE net_node_uuid = ?`
			_, err = tx.ExecContext(ctx, updateIPAddress, 1, netNodeUUID)
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
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[struct{}]) {
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
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	providerGetter := func(ctx context.Context) (service.Provider, error) {
		return machineservice.NewNoopProvider(), nil
	}
	caasProviderGetter := func(ctx context.Context) (service.CAASProvider, error) {
		return nil, coreerrors.NotSupported
	}

	registryGetter := corestorage.ConstModelStorageRegistry(func() internalstorage.ProviderRegistry {
		return internalstorage.NotImplementedProviderRegistry{}
	})
	state := state.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c))

	return service.NewWatchableService(
		state,
		domaintesting.NoopLeaderEnsurer(),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		nil,
		providerGetter,
		caasProviderGetter,
		service.NewStorageProviderValidator(registryGetter, state),
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *watcherSuite) createIAASApplication(c *tc.C, svc *service.WatchableService, name string, units ...service.AddIAASUnitArg) coreapplication.ID {
	return s.createIAASApplicationWithCharmAndStoragePath(c, svc, name, &stubCharm{}, "", units...)
}

func (s *watcherSuite) createIAASApplicationWithCharmAndStoragePath(c *tc.C, svc *service.WatchableService, name string, ch internalcharm.Charm, storagePath string, units ...service.AddIAASUnitArg) coreapplication.ID {
	ctx := c.Context()
	appID, err := svc.CreateIAASApplication(ctx, name, ch, corecharm.Origin{
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

func (s *watcherSuite) createCAASApplication(c *tc.C, svc *service.WatchableService, name string, units ...service.AddUnitArg) coreapplication.ID {
	ctx := c.Context()
	s.createSubnetForCAASModel(c)
	appID, err := svc.CreateCAASApplication(ctx, name, &stubCharm{}, corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, service.AddApplicationArgs{
		ReferenceName:    name,
		CharmStoragePath: "",
		DownloadInfo: &charm.DownloadInfo{
			Provenance:  charm.ProvenanceDownload,
			DownloadURL: "http://example.com",
		},
	}, units...)
	c.Assert(err, tc.ErrorIsNil)
	return appID
}

func (s *watcherSuite) createSubnetForCAASModel(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Only insert the subnet it if doesn't exist.
		var rowCount int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM subnet`).Scan(&rowCount); err != nil {
			return err
		}
		if rowCount != 0 {
			return nil
		}

		subnetUUID := uuid.MustNewUUID().String()
		_, err := tx.ExecContext(ctx, "INSERT INTO subnet (uuid, cidr) VALUES (?, ?)", subnetUUID, "0.0.0.0/0")
		if err != nil {
			return err
		}
		subnetUUID2 := uuid.MustNewUUID().String()
		_, err = tx.ExecContext(ctx, "INSERT INTO subnet (uuid, cidr) VALUES (?, ?)", subnetUUID2, "::/0")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
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
