// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	relationtesting "github.com/juju/juju/core/relation/testing"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	domainrelation "github.com/juju/juju/domain/relation"
	"github.com/juju/juju/domain/relation/service"
	"github.com/juju/juju/domain/relation/state"
	domaintesting "github.com/juju/juju/domain/testing"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite

	charmUUID         corecharm.ID
	charmRelationUUID uuid.UUID
	appUUID           coreapplication.ID
	appEndpointUUID   uuid.UUID
	appName           string
	// helps generation of consecutive relation_id
	relationCount int
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.charmUUID = charmtesting.GenCharmID(c)
	s.charmRelationUUID = uuid.MustNewUUID()
	s.appUUID = applicationtesting.GenApplicationUUID(c)
	s.appEndpointUUID = uuid.MustNewUUID()
	s.appName = "my-application"
	s.relationCount = 1

	// Populate DB with charm, application and endpoints
	s.addCharm(c, s.charmUUID, "app")
	s.addCharmRelation(c, s.charmUUID, s.charmRelationUUID, 0)
	s.addApplication(c, s.charmUUID, s.appUUID, s.appName)
	s.addApplicationEndpoint(c, s.appEndpointUUID, s.appUUID, s.charmRelationUUID)
}

func (s *watcherSuite) TestWatchLifeSuspendedStatusPrincipal(c *tc.C) {
	// Arrange: create the required state, with one relation and its status.
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.ModelUUID())

	relationUUID, _, _ := s.setupSecondAppAndRelate(c, "two")
	unitUUID := unittesting.GenUnitUUID(c)
	s.addUnit(c, unitUUID, "my-application/0", s.appUUID, s.charmUUID)

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchLifeSuspendedStatus(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)

	relationKey := relationtesting.GenNewKey(c, "two:fake-1 my-application:fake-0").String()
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Act 0: change the relation life.
	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE relation SET life_id = 1 WHERE uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: received changed of relation key.
		w.Check(
			watchertest.StringSliceAssert[string](relationKey),
		)
	})

	// Act 1: change the relation status other than suspended.
	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx,
				"UPDATE relation_status SET relation_status_type_id = 3 WHERE relation_uuid=?", relationUUID,
			); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: no change received. Change received only if status changes to
		// suspended.
		w.AssertNoChange()
	})

	// Act 2: change the relation status to suspended.
	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx,
				"UPDATE relation_status SET relation_status_type_id = 4 WHERE relation_uuid=?", relationUUID,
			); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: received change of relation key,
		// relation status changed to suspended.
		w.Check(
			watchertest.StringSliceAssert[string](relationKey),
		)
	})

	// Act 3: add a relation unrelated to the current unit.
	harness.AddTest(c, func(c *tc.C) {
		_ = s.setupSecondRelationNotFound(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Act 4: change the relation status to joined and life to dead, to get
	// changes on both tables watched.
	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE relation SET life_id = 2 WHERE uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(
				ctx, "UPDATE relation_status SET relation_status_type_id = 1 WHERE relation_uuid=?", relationUUID,
			); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: with changes in both tables at the same time, the relation
		// key is sent once.
		w.Check(
			watchertest.StringSliceAssert[string](relationKey),
		)
	})

	harness.Run(c, []string{relationKey})
}

func (s *watcherSuite) TestWatchLifeSuspendedStatusSubordinate(c *tc.C) {
	// Arrange: create the required state, with one relation and its status.
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.ModelUUID())

	relationUUID, appTwoUUID, charmTwoUUID := s.setupSecondAppAndRelate(c, "two")

	subordinateUnitUUID := unittesting.GenUnitUUID(c)
	principalUnitUUID := unittesting.GenUnitUUID(c)
	s.setCharmSubordinate(c, s.charmUUID, true)
	s.addUnit(c, subordinateUnitUUID, "my-application/0", s.appUUID, s.charmUUID)
	s.addUnit(c, principalUnitUUID, "two/0", appTwoUUID, charmTwoUUID)
	s.setUnitSubordinate(c, subordinateUnitUUID, principalUnitUUID)

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchLifeSuspendedStatus(c.Context(), subordinateUnitUUID)
	c.Assert(err, tc.ErrorIsNil)

	relationKey := relationtesting.GenNewKey(c, "two:fake-1 my-application:fake-0").String()
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Act 0: change the relation life.
	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE relation SET life_id = 1 WHERE uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: received changed of relation key.
		w.Check(
			watchertest.StringSliceAssert[string](relationKey),
		)
	})

	// Act 1: change the relation status other than suspended.
	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx,
				"UPDATE relation_status SET relation_status_type_id = 3 WHERE relation_uuid=?", relationUUID,
			); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: no change received. Change received only if status changes to
		// suspended.
		w.AssertNoChange()
	})

	// Act 2: change the relation status to suspended.
	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx,
				"UPDATE relation_status SET relation_status_type_id = 4 WHERE relation_uuid=?", relationUUID,
			); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: received changed of relation key, relation status changed to suspended.
		w.Check(
			watchertest.StringSliceAssert[string](relationKey),
		)
	})

	var relationTwoUUID relation.UUID
	// Act 3: add a relation unrelated to the current unit.
	harness.AddTest(c, func(c *tc.C) {
		relationTwoUUID = s.setupSecondRelationNotFound(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Act 4: change the relation status to joined and life to dead, to get
	// changes on both tables watched. Change the second relations status also,
	// only the first relation should trigger an event.
	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE relation SET life_id = 2 WHERE uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(
				ctx, "UPDATE relation_status SET relation_status_type_id = 1 WHERE relation_uuid=?", relationUUID,
			); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx,
				"UPDATE relation SET life_id = 2 WHERE uuid=?", relationTwoUUID,
			); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: with changes in both tables at the same time, the relation
		// key is sent once.
		w.Check(
			watchertest.StringSliceAssert[string](relationKey),
		)
	})

	// Act: run test harness.
	// Assert: initial event is relationKey.
	harness.Run(c, []string{relationKey})
}

func (s *watcherSuite) setupSecondAppAndRelate(
	c *tc.C, appNameTwo string,
) (relation.UUID, coreapplication.ID, corecharm.ID) {
	relationUUID := relationtesting.GenRelationUUID(c)
	relationEndpointUUID := relationtesting.GenEndpointUUID(c)

	charmTwoUUID := charmtesting.GenCharmID(c)
	charmRelationTwoUUID := uuid.MustNewUUID()
	appTwoUUID := applicationtesting.GenApplicationUUID(c)
	relationEndpointTwoUUID := relationtesting.GenEndpointUUID(c)
	appEndpointTwoUUID := uuid.MustNewUUID()
	s.addCharm(c, charmTwoUUID, appNameTwo)
	s.addCharmRelation(c, charmTwoUUID, charmRelationTwoUUID, 1)
	s.addApplication(c, charmTwoUUID, appTwoUUID, appNameTwo)
	s.addApplicationEndpoint(c, appEndpointTwoUUID, appTwoUUID, charmRelationTwoUUID)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID, relationUUID, s.appEndpointUUID)
	s.addRelationEndpoint(c, relationEndpointTwoUUID, relationUUID, appEndpointTwoUUID)
	s.addRelationStatus(c, relationUUID, 1)

	return relationUUID, appTwoUUID, charmTwoUUID
}

// setupSecondRelationNotFound adds a relation between new applications
// foo and bar. Neither are the application under test.
func (s *watcherSuite) setupSecondRelationNotFound(c *tc.C) relation.UUID {
	charmOneUUID := charmtesting.GenCharmID(c)
	charmRelationOneUUID := uuid.MustNewUUID()
	appOneUUID := applicationtesting.GenApplicationUUID(c)
	appEndpointOneUUID := uuid.MustNewUUID()
	s.addCharm(c, charmOneUUID, "foo")
	s.addCharmRelation(c, charmOneUUID, charmRelationOneUUID, 1)
	s.addApplication(c, charmOneUUID, appOneUUID, "foo")
	s.addApplicationEndpoint(c, appEndpointOneUUID, appOneUUID, charmRelationOneUUID)

	charmTwoUUID := charmtesting.GenCharmID(c)
	charmRelationTwoUUID := uuid.MustNewUUID()
	appTwoUUID := applicationtesting.GenApplicationUUID(c)
	appEndpointTwoUUID := uuid.MustNewUUID()
	s.addCharm(c, charmTwoUUID, "bar")
	s.addCharmRelation(c, charmTwoUUID, charmRelationTwoUUID, 1)
	s.addApplication(c, charmTwoUUID, appTwoUUID, "bar")
	s.addApplicationEndpoint(c, appEndpointTwoUUID, appTwoUUID, charmRelationTwoUUID)

	relationUUID := relationtesting.GenRelationUUID(c)
	relationEndpointOneUUID := relationtesting.GenEndpointUUID(c)
	relationEndpointTwoUUID := relationtesting.GenEndpointUUID(c)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointOneUUID, relationUUID, appEndpointOneUUID)
	s.addRelationEndpoint(c, relationEndpointTwoUUID, relationUUID, appEndpointTwoUUID)
	s.addRelationStatus(c, relationUUID, 1)

	return relationUUID
}

func (s *watcherSuite) TestWatchRelatedUnitsUnitScope(c *tc.C) {
	// Arrange:
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "relation_application_settings_hash")
	config := s.setupTestWatchRelationUnit(c)

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchRelatedUnits(c.Context(), config.watched0UUID, config.relationUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Act: insert relation_unit for watched/0 (enter scope) => no event
	harness.AddTest(c, func(c *tc.C) {
		s.act(c, "INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid) VALUES (?, ?, ?)",
			relationtesting.GenRelationUnitUUID(c), config.otherRelationUUID, config.watched0UUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Act: delete relation_unit for watched/0 (leave scope) => no event
	harness.AddTest(c, func(c *tc.C) {
		s.act(c, "DELETE FROM relation_unit WHERE unit_uuid = ?", config.watched0UUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Act: insert relation_unit for other/0 (enter scope) => event with other/0
	// unit_uuid
	harness.AddTest(c, func(c *tc.C) {
		s.act(c, "INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid) VALUES (?, ?, ?)",
			relationtesting.GenRelationUnitUUID(c), config.otherRelationUUID, config.other0UUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](domainrelation.EncodeUnitUUID(config.other0UUID.String())),
		)
	})

	// Act: delete relation_unit for other/0 (leave scope) => event with other/0
	// unit_uuid
	harness.AddTest(c, func(c *tc.C) {
		s.act(c, "DELETE FROM relation_unit WHERE unit_uuid = ?", config.other0UUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](domainrelation.EncodeUnitUUID(config.other0UUID.String())),
		)
	})

	// Act: insert relation_unit for watched/1 (enter scope) => event with
	// watched/1 unit_uuid
	harness.AddTest(c, func(c *tc.C) {
		s.act(c, "INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid) VALUES (?, ?, ?)",
			relationtesting.GenRelationUnitUUID(c), config.watchedRelationUUID, config.watched1UUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](domainrelation.EncodeUnitUUID(config.watched1UUID.String())),
		)
	})

	// Act: delete relation_unit for watched/1 (leave scope) => event with
	// watched/1 unit_uuid
	harness.AddTest(c, func(c *tc.C) {
		s.act(c, "DELETE FROM relation_unit WHERE unit_uuid = ?", config.watched1UUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](domainrelation.EncodeUnitUUID(config.watched1UUID.String())),
		)
	})

	// Act: run test harness.
	// Assert: initial events are related units
	harness.Run(c, transform.Slice(config.initialEvents, func(uuid coreunit.UUID) string {
		return domainrelation.EncodeUnitUUID(uuid.String())
	}))
}
func (s *watcherSuite) TestWatchRelatedUnitsSettings(c *tc.C) {
	// Arrange:
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "relation_unit_settings_hash")
	config := s.setupTestWatchRelationUnit(c)

	// Relation units UUID
	watchedRelUnit0UUID := relationtesting.GenRelationUnitUUID(c)
	watchedRelUnit1UUID := relationtesting.GenRelationUnitUUID(c)
	otherRelUnit0UUID := relationtesting.GenRelationUnitUUID(c)

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchRelatedUnits(c.Context(), config.watched0UUID, config.relationUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Arrange: Add relation unit, before updating hash settings => will
	// generate event for units except the watched one.
	harness.AddTest(c, func(c *tc.C) {
		s.arrange(c, `
INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid)
VALUES (?,?,?),
       (?,?,?),
       (?,?,?)`,
			watchedRelUnit0UUID, config.watchedRelationUUID, config.watched0UUID,
			watchedRelUnit1UUID, config.watchedRelationUUID, config.watched1UUID,
			otherRelUnit0UUID, config.otherRelationUUID, config.other0UUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](domainrelation.EncodeUnitUUID(config.other0UUID.String()),
				domainrelation.EncodeUnitUUID(config.watched1UUID.String())),
		)
	})

	// Act: update setting_hash in watched/0 unit setting => no event
	harness.AddTest(c, func(c *tc.C) {
		s.act(c, "INSERT INTO relation_unit_settings_hash (relation_unit_uuid, sha256) VALUES (?, 'hash')",
			watchedRelUnit0UUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Act: update setting_hash in other/0 unit setting => event with other/0
	// unit_uuid
	harness.AddTest(c, func(c *tc.C) {
		s.act(c, "INSERT INTO relation_unit_settings_hash (relation_unit_uuid, sha256) VALUES (?, 'hash')",
			otherRelUnit0UUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](domainrelation.EncodeUnitUUID(config.other0UUID.String())),
		)
	})

	// Act: update setting_hash in watched/1 unit setting => event with
	// watched/1 unit_uuid
	harness.AddTest(c, func(c *tc.C) {
		s.act(c, "INSERT INTO relation_unit_settings_hash (relation_unit_uuid, sha256) VALUES (?, 'hash')",
			watchedRelUnit1UUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](domainrelation.EncodeUnitUUID(config.watched1UUID.String())),
		)
	})

	// Act: update settings hash for "other" application => event with other app UUID.
	harness.AddTest(c, func(c *tc.C) {
		s.act(c, "INSERT INTO relation_application_settings_hash (relation_endpoint_uuid, sha256) VALUES (?, 'hash')",
			config.otherRelationUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](domainrelation.EncodeApplicationUUID(config.otherUUID.String())),
		)
	})

	// Act: update settings hash for watched application => no event.
	harness.AddTest(c, func(c *tc.C) {
		s.act(c, "INSERT INTO relation_application_settings_hash (relation_endpoint_uuid, sha256) VALUES (?, 'hash')",
			config.watchedRelationUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Act: run test harness.
	// Assert: initial events are related units
	harness.Run(c, transform.Slice(config.initialEvents, func(uuid coreunit.UUID) string {
		return domainrelation.EncodeUnitUUID(uuid.String())
	}))
}

func (s *watcherSuite) TestWatchRelatedUnitsPeerAppSettings(c *tc.C) {
	// Arrange:
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "relation_unit_settings_hash")
	config := s.setupTestWatchPeerRelationUnit(c)

	// Relation units UUID
	watchedRelUnit0UUID := relationtesting.GenRelationUnitUUID(c)
	watchedRelUnit1UUID := relationtesting.GenRelationUnitUUID(c)

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchRelatedUnits(c.Context(), config.watched0UUID, config.relationUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Arrange: Add relation unit, before updating hash settings => will generate
	// event for units except the watched one.
	harness.AddTest(c, func(c *tc.C) {
		s.arrange(c, `
INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid)
VALUES (?,?,?),
       (?,?,?)`,
			watchedRelUnit0UUID, config.watchedRelationUUID, config.watched0UUID,
			watchedRelUnit1UUID, config.watchedRelationUUID, config.watched1UUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](
				domainrelation.EncodeUnitUUID(config.watched1UUID.String())),
		)
	})

	// Act: update settings hash for watched application =>
	// event with watched app UUID.
	harness.AddTest(c, func(c *tc.C) {
		s.act(c, "INSERT INTO relation_application_settings_hash (relation_endpoint_uuid, sha256) VALUES (?, 'hash')",
			config.watchedRelationUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert[string](domainrelation.EncodeApplicationUUID(config.watchedUUID.String())),
		)
	})

	// Act: run test harness.
	// Assert: initial events are related units
	harness.Run(c, transform.Slice(config.initialEvents, func(uuid coreunit.UUID) string {
		return domainrelation.EncodeUnitUUID(uuid.String())
	}))
}

type testWatchRelationUnit struct {
	watchedUnit1                           coreunit.Name
	relationUUID                           relation.UUID
	other0UUID, watched0UUID, watched1UUID coreunit.UUID
	otherRelationUUID, watchedRelationUUID relation.EndpointUUID
	otherUUID                              coreapplication.ID
	initialEvents                          []coreunit.UUID
}

func (s *watcherSuite) setupTestWatchRelationUnit(c *tc.C) testWatchRelationUnit {
	// Arrange:
	// - 2 apps linked through a relation
	//   - watched/0: we will create a watcher on this unit
	//   - watched/1: second unit on the same app, required to verify watcher
	//                behavior
	//   - other/0: only one unit on the second app, no need more.
	config := testWatchRelationUnit{}
	config.watchedUnit1 = "watched/0"
	config.relationUUID = relationtesting.GenRelationUUID(c)

	charmUUID := charmtesting.GenCharmID(c)
	watchedUUID := applicationtesting.GenApplicationUUID(c)
	config.otherUUID = applicationtesting.GenApplicationUUID(c)
	config.watched0UUID = unittesting.GenUnitUUID(c)
	config.watched1UUID = unittesting.GenUnitUUID(c)
	config.other0UUID = unittesting.GenUnitUUID(c)
	charmRelationProviderUUID := uuid.MustNewUUID()
	charmRelationRequiresUUID := uuid.MustNewUUID()
	watchedEndpointUUID := uuid.MustNewUUID()
	otherEndpointUUID := uuid.MustNewUUID()
	config.watchedRelationUUID = relationtesting.GenEndpointUUID(c)
	config.otherRelationUUID = relationtesting.GenEndpointUUID(c)
	s.addCharm(c, charmUUID, "whatever")
	s.addCharmRelation(c, charmUUID, charmRelationProviderUUID, 0)
	s.addCharmRelation(c, charmUUID, charmRelationRequiresUUID, 1)
	s.addApplication(c, charmUUID, watchedUUID, "watched")
	s.addApplication(c, charmUUID, config.otherUUID, "other")
	s.addUnit(c, config.watched0UUID, config.watchedUnit1, watchedUUID, charmUUID)
	s.addUnit(c, config.watched1UUID, "watched/1", watchedUUID, charmUUID)
	s.addUnit(c, config.other0UUID, "other/0", config.otherUUID, charmUUID)
	s.addApplicationEndpoint(c, watchedEndpointUUID, watchedUUID, charmRelationProviderUUID)
	s.addApplicationEndpoint(c, otherEndpointUUID, config.otherUUID, charmRelationRequiresUUID)
	s.addRelation(c, config.relationUUID)
	s.addRelationEndpoint(c, config.watchedRelationUUID, config.relationUUID, watchedEndpointUUID)
	s.addRelationEndpoint(c, config.otherRelationUUID, config.relationUUID, otherEndpointUUID)

	config.initialEvents = []coreunit.UUID{config.other0UUID, config.watched1UUID}
	sort.Slice(config.initialEvents, func(i, j int) bool { return config.initialEvents[i] < config.initialEvents[j] })

	return config
}

type testWatchPeerRelationUnit struct {
	watchedUnit1               coreunit.Name
	relationUUID               relation.UUID
	watched0UUID, watched1UUID coreunit.UUID
	watchedRelationUUID        relation.EndpointUUID
	watchedUUID                coreapplication.ID
	initialEvents              []coreunit.UUID
}

func (s *watcherSuite) setupTestWatchPeerRelationUnit(c *tc.C) testWatchPeerRelationUnit {
	// Arrange:
	// - 2 apps linked through a relation
	//   - watched/0 : we will create a watcher on this unit
	//   - watched/1 : second unit on the same app, required to verify watcher behavior
	//   - other/0 : only one unit on the second app, no need more.
	config := testWatchPeerRelationUnit{}
	config.watchedUnit1 = "watched/0"
	config.relationUUID = relationtesting.GenRelationUUID(c)

	charmUUID := charmtesting.GenCharmID(c)
	config.watchedUUID = applicationtesting.GenApplicationUUID(c)
	config.watched0UUID = unittesting.GenUnitUUID(c)
	config.watched1UUID = unittesting.GenUnitUUID(c)
	charmRelationPeerUUID := uuid.MustNewUUID()
	watchedEndpointUUID := uuid.MustNewUUID()
	config.watchedRelationUUID = relationtesting.GenEndpointUUID(c)
	s.addCharm(c, charmUUID, "whatever")
	s.addCharmRelation(c, charmUUID, charmRelationPeerUUID, 2)
	s.addApplication(c, charmUUID, config.watchedUUID, "watched")
	s.addUnit(c, config.watched0UUID, config.watchedUnit1, config.watchedUUID, charmUUID)
	s.addUnit(c, config.watched1UUID, "watched/1", config.watchedUUID, charmUUID)
	s.addApplicationEndpoint(c, watchedEndpointUUID, config.watchedUUID, charmRelationPeerUUID)
	s.addRelation(c, config.relationUUID)
	s.addRelationEndpoint(c, config.watchedRelationUUID, config.relationUUID, watchedEndpointUUID)

	config.initialEvents = []coreunit.UUID{config.watched1UUID}

	return config
}

func (s *watcherSuite) setupService(c *tc.C, factory domain.WatchableDBFactory) *service.WatchableService {
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	return service.NewWatchableService(
		state.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		domaintesting.NoopLeaderEnsurer(),
		loggertesting.WrapCheckLog(c),
	)
}

// setCharmSubordinate updates the charm's metadata to mark it as subordinate,
// or inserts it if not present in the database.
func (s *watcherSuite) setCharmSubordinate(c *tc.C, charmUUID corecharm.ID, subordinate bool) {
	s.arrange(c, `
INSERT INTO charm_metadata (charm_uuid, name, subordinate)
VALUES (?,?,true)
ON CONFLICT DO UPDATE SET subordinate = ?
`, charmUUID, charmUUID, subordinate)
}

// setUnitSubordinate sets unit 1 to be a subordinate of unit 2.
func (s *watcherSuite) setUnitSubordinate(c *tc.C, subordinate, principal coreunit.UUID) {
	s.arrange(c, `
INSERT INTO unit_principal (unit_uuid, principal_uuid)
VALUES (?,?)
`, subordinate, principal)
}

// addApplication adds a new application to the database with the specified UUID
// and name.
func (s *watcherSuite) addApplication(c *tc.C, charmUUID corecharm.ID, appUUID coreapplication.ID, appName string) {
	s.arrange(c, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) 
VALUES (?, ?, ?, ?, ?)
`, appUUID, appName, 0 /* alive */, charmUUID, network.AlphaSpaceId)
}

// addApplicationEndpoint inserts a new application endpoint into the database
// with the specified UUIDs and relation data.
func (s *watcherSuite) addApplicationEndpoint(c *tc.C, applicationEndpointUUID uuid.UUID, applicationUUID coreapplication.ID, charmRelationUUID uuid.UUID) {
	s.arrange(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?, ?, ?, ?)
`, applicationEndpointUUID.String(), applicationUUID, charmRelationUUID.String(), network.AlphaSpaceId)
}

// addCharm inserts a new charm into the database with a predefined UUID,
// reference name, and architecture ID.
func (s *watcherSuite) addCharm(c *tc.C, charmUUID corecharm.ID, charmName string) {
	s.arrange(c, `
INSERT INTO charm (uuid, reference_name, architecture_id) 
VALUES (?, ?, 0)
`, charmUUID, charmName)
}

// addCharmRelation inserts a new charm relation into the database with the
// given UUID and predefined attributes.
func (s *watcherSuite) addCharmRelation(c *tc.C, charmUUID corecharm.ID, charmRelationUUID uuid.UUID, roleID int) {
	name := fmt.Sprintf("fake-%d", roleID)
	s.arrange(c, `
INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name)
VALUES (?, ?, 0,?, ?)
`, charmRelationUUID.String(), charmUUID, roleID, name)
}

// addRelation inserts a new relation into the database with the given UUID and
// default relation and life IDs.
func (s *watcherSuite) addRelation(c *tc.C, relationUUID relation.UUID) {
	s.arrange(c, `
INSERT INTO relation (uuid, life_id, relation_id, scope_id)
VALUES (?,0,?, 0)
`, relationUUID, s.relationCount)
	s.relationCount++
}

// addRelationEndpoint inserts a relation endpoint into the database using the
// provided UUIDs for relation and endpoint.
func (s *watcherSuite) addRelationEndpoint(
	c *tc.C, relationEndpointUUID relation.EndpointUUID, relationUUID relation.UUID, applicationEndpointUUID uuid.UUID,
) {
	s.arrange(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?,?,?)
`, relationEndpointUUID, relationUUID, applicationEndpointUUID.String())
}

// addRelationStatus inserts a relation_status row into the database using the
// provided UUID for relation and status id.
func (s *watcherSuite) addRelationStatus(c *tc.C, relationUUID relation.UUID, status_id int) {
	s.arrange(c, `
INSERT INTO relation_status (relation_uuid, relation_status_type_id, updated_at)
VALUES (?,?,?)
`, relationUUID, status_id, time.Now())
}

// addUnit adds a new unit to the specified application in the database with
// the given UUID and name.
func (s *watcherSuite) addUnit(
	c *tc.C,
	unitUUID coreunit.UUID,
	unitName coreunit.Name,
	appUUID coreapplication.ID,
	charmUUID corecharm.ID,
) {
	fakeNetNodeUUID := "fake-net-node-uuid"
	s.arrange(c, `
INSERT INTO net_node (uuid) 
VALUES (?)
ON CONFLICT DO NOTHING
`, fakeNetNodeUUID)

	s.arrange(c, `
INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid)
VALUES (?, ?, ?, ?, ?, ?)
`, unitUUID, unitName, 0 /* alive */, appUUID, charmUUID, fakeNetNodeUUID)
}

// query is dedicated to build up the initial state of the db during a test
func (s *watcherSuite) arrange(c *tc.C, query string, args ...any) {
	s.query(c, func(err error) tc.CommentInterface {
		return tc.Commentf("(Arrange) failed to populate DB: %v",
			errors.ErrorStack(err))
	}, query, args...)
}

// act is dedicated to update the db during the test, as an action
func (s *watcherSuite) act(c *tc.C, query string, args ...any) {
	s.query(c, func(err error) tc.CommentInterface {
		return tc.Commentf("(Act) failed to update DB: %v",
			errors.ErrorStack(err))
	}, query, args...)
}

// query executes a database query within a standard transaction. If something
// goes wrong, the assertion allows to define a specific error as comment
// interface.
func (s *watcherSuite) query(c *tc.C, comment func(error) tc.CommentInterface, query string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %s)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, comment(err))
}
