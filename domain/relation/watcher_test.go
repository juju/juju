// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.charmUUID = charmtesting.GenCharmID(c)
	s.charmRelationUUID = uuid.MustNewUUID()
	s.appUUID = applicationtesting.GenApplicationUUID(c)
	s.appEndpointUUID = uuid.MustNewUUID()

	// Populate DB with charm, application and endpoints
	s.addCharm(c, s.charmUUID, "app")
	s.addCharmRelation(c, s.charmUUID, s.charmRelationUUID, 0)
	s.addApplication(c, s.charmUUID, s.appUUID, "my-application")
	s.addApplicationEndpoint(c, s.appEndpointUUID, s.appUUID, s.charmRelationUUID)
}

// TestWatchUnitRelations ensures the unit relation watcher correctly captures
// create, update, and delete events in the database.
func (s *watcherSuite) TestWatchUnitRelations(c *gc.C) {

	// Arrange: create the required state, with one relation endpoint and related
	// objects.
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "relation_application_setting")
	relationUUID := relationtesting.GenRelationUUID(c)
	relationEndpointUUID := relationtesting.GenEndpointUUID(c)

	// Populate DB with relation endpoint.
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID, relationUUID, s.appEndpointUUID)

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchApplicationSettings(context.Background(), relationUUID, s.appUUID)
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Act: ensure we get the created event.
	harness.AddTest(func(c *gc.C) {
		s.act(c, `
INSERT INTO relation_application_setting (relation_endpoint_uuid, key, value)
VALUES (?, 'key', 'value')
`, relationEndpointUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Act: ensure we get the updated event.
	harness.AddTest(func(c *gc.C) {
		s.act(c, `
UPDATE relation_application_setting
SET value = 'new-value'
WHERE relation_endpoint_uuid = ?
AND key = 'key'
`, relationEndpointUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Act: ensure we get the deleted event.
	harness.AddTest(func(c *gc.C) {
		s.act(c, `
DELETE FROM relation_application_setting
WHERE relation_endpoint_uuid = ?
AND key = 'key'
`, relationEndpointUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchLifeSuspendedStatusPrincipal(c *gc.C) {
	// Arrange: create the required state, with one relation and its status.
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.ModelUUID())
	relationUUID := relationtesting.GenRelationUUID(c)
	relationEndpointUUID := relationtesting.GenEndpointUUID(c)

	charmTwoUUID := charmtesting.GenCharmID(c)
	charmRelationTwoUUID := uuid.MustNewUUID()
	appTwoUUID := applicationtesting.GenApplicationUUID(c)
	relationEndpointTwoUUID := relationtesting.GenEndpointUUID(c)
	appEndpointTwoUUID := uuid.MustNewUUID()
	s.addCharm(c, charmTwoUUID, "two")
	s.addCharmRelation(c, charmTwoUUID, charmRelationTwoUUID, 1)
	s.addApplication(c, charmTwoUUID, appTwoUUID, "two")
	s.addApplicationEndpoint(c, appEndpointTwoUUID, appTwoUUID, charmRelationTwoUUID)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID, relationUUID, s.appEndpointUUID)
	s.addRelationEndpoint(c, relationEndpointTwoUUID, relationUUID, appEndpointTwoUUID)
	s.addRelationStatus(c, relationUUID, 1)

	unitUUID := unittesting.GenUnitUUID(c)
	s.addUnit(c, unitUUID, "my-application/0", s.appUUID, s.charmUUID)

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchLifeSuspendedStatus(context.Background(), unitUUID)
	c.Assert(err, jc.ErrorIsNil)

	relationKey := relationtesting.GenNewKey(c, "two:fake-provides my-application:fake-provides").String()
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Act: change the relation life.
	harness.AddTest(func(c *gc.C) {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE relation SET life_id = 1 WHERE uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: received changed of relation key.
		w.Check(
			watchertest.StringSliceAssert[string](relationKey),
		)
	})

	// Act: change the relation status other than suspended.
	harness.AddTest(func(c *gc.C) {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE relation_status SET relation_status_type_id = 3 WHERE relation_uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: no change received. Change received only if status changes to
		// suspended.
		w.AssertNoChange()
	})

	// Act: change the relation status to suspended.
	harness.AddTest(func(c *gc.C) {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE relation_status SET relation_status_type_id = 4 WHERE relation_uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: received changed of relation key, relation status changed to suspended.
		w.Check(
			watchertest.StringSliceAssert[string](relationKey),
		)
	})

	// Act: change the relation status to joined and life to dead, to get
	// changes on both tables watched.
	harness.AddTest(func(c *gc.C) {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE relation SET life_id = 2 WHERE uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "UPDATE relation_status SET relation_status_type_id = 1 WHERE relation_uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: with changes in both tables at the same time, the relation
		// key is sent once.
		w.Check(
			watchertest.StringSliceAssert[string](relationKey),
		)
	})

	harness.Run(c, []string{relationKey})
}

func (s *watcherSuite) TestWatchLifeSuspendedStatusSubordinate(c *gc.C) {
	// Arrange: create the required state, with one relation and its status.
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, s.ModelUUID())
	relationUUID := relationtesting.GenRelationUUID(c)
	relationEndpointUUID := relationtesting.GenEndpointUUID(c)

	charmTwoUUID := charmtesting.GenCharmID(c)
	charmRelationTwoUUID := uuid.MustNewUUID()
	appTwoUUID := applicationtesting.GenApplicationUUID(c)
	relationEndpointTwoUUID := relationtesting.GenEndpointUUID(c)
	appEndpointTwoUUID := uuid.MustNewUUID()
	s.addCharm(c, charmTwoUUID, "two")
	s.addCharmRelation(c, charmTwoUUID, charmRelationTwoUUID, 1)
	s.addApplication(c, charmTwoUUID, appTwoUUID, "two")
	s.addApplicationEndpoint(c, appEndpointTwoUUID, appTwoUUID, charmRelationTwoUUID)
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID, relationUUID, s.appEndpointUUID)
	s.addRelationEndpoint(c, relationEndpointTwoUUID, relationUUID, appEndpointTwoUUID)
	s.addRelationStatus(c, relationUUID, 1)

	subordinateUnitUUID := unittesting.GenUnitUUID(c)
	principalUnitUUID := unittesting.GenUnitUUID(c)
	s.setCharmSubordinate(c, s.charmUUID, true)
	s.addUnit(c, subordinateUnitUUID, "my-application/0", s.appUUID, s.charmUUID)
	s.addUnit(c, principalUnitUUID, "two/0", appTwoUUID, charmTwoUUID)
	s.setUnitSubordinate(c, subordinateUnitUUID, principalUnitUUID)

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchLifeSuspendedStatus(context.Background(), subordinateUnitUUID)
	c.Assert(err, jc.ErrorIsNil)

	relationKey := relationtesting.GenNewKey(c, "two:fake-provides my-application:fake-provides").String()
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Act 0: change the relation life.
	harness.AddTest(func(c *gc.C) {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE relation SET life_id = 1 WHERE uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: received changed of relation key.
		w.Check(
			watchertest.StringSliceAssert[string](relationKey),
		)
	})

	// Act 1: change the relation status other than suspended.
	harness.AddTest(func(c *gc.C) {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE relation_status SET relation_status_type_id = 3 WHERE relation_uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: no change received. Change received only if status changes to
		// suspended.
		w.AssertNoChange()
	})

	// Act 2: change the relation status to suspended.
	harness.AddTest(func(c *gc.C) {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE relation_status SET relation_status_type_id = 4 WHERE relation_uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		// Assert: received changed of relation key, relation status changed to suspended.
		w.Check(
			watchertest.StringSliceAssert[string](relationKey),
		)
	})

	// Act 3: change the relation status to joined and life to dead, to get
	// changes on both tables watched.
	harness.AddTest(func(c *gc.C) {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, "UPDATE relation SET life_id = 2 WHERE uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			if _, err := tx.ExecContext(ctx, "UPDATE relation_status SET relation_status_type_id = 1 WHERE relation_uuid=?", relationUUID); err != nil {
				return errors.Capture(err)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
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

func (s *watcherSuite) setupService(c *gc.C, factory domain.WatchableDBFactory) *service.WatchableService {
	modelDB := func() (database.TxnRunner, error) {
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
func (s *watcherSuite) setCharmSubordinate(c *gc.C, charmUUID corecharm.ID, subordinate bool) {
	s.arrange(c, `
INSERT INTO charm_metadata (charm_uuid, name, subordinate)
VALUES (?,?,true)
ON CONFLICT DO UPDATE SET subordinate = ?
`, charmUUID, charmUUID, subordinate)
}

// setUnitSubordinate sets unit 1 to be a subordinate of unit 2.
func (s *watcherSuite) setUnitSubordinate(c *gc.C, subordinate, principal coreunit.UUID) {
	s.arrange(c, `
INSERT INTO unit_principal (unit_uuid, principal_uuid)
VALUES (?,?)
`, subordinate, principal)
}

// addApplication adds a new application to the database with the specified UUID and name.
func (s *watcherSuite) addApplication(c *gc.C, charmUUID corecharm.ID, appUUID coreapplication.ID, appName string) {
	s.arrange(c, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) 
VALUES (?, ?, ?, ?, ?)
`, appUUID, appName, 0 /* alive */, charmUUID, network.AlphaSpaceId)
}

// addApplicationEndpoint inserts a new application endpoint into the database with the specified UUIDs and relation data.
func (s *watcherSuite) addApplicationEndpoint(c *gc.C, applicationEndpointUUID uuid.UUID, applicationUUID coreapplication.ID, charmRelationUUID uuid.UUID) {
	s.arrange(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?, ?, ?, ?)
`, applicationEndpointUUID.String(), applicationUUID, charmRelationUUID.String(), network.AlphaSpaceId)
}

// addCharm inserts a new charm into the database with a predefined UUID, reference name, and architecture ID.
func (s *watcherSuite) addCharm(c *gc.C, charmUUID corecharm.ID, charmName string) {
	s.arrange(c, `
INSERT INTO charm (uuid, reference_name, architecture_id) 
VALUES (?, ?, 0)
`, charmUUID, charmName)
}

// addCharmRelation inserts a new charm relation into the database with the given UUID and predefined attributes.
func (s *watcherSuite) addCharmRelation(c *gc.C, charmUUID corecharm.ID, charmRelationUUID uuid.UUID, kind int) {
	s.arrange(c, `
INSERT INTO charm_relation (uuid, charm_uuid, kind_id, scope_id, role_id, name)
VALUES (?, ?, ?,0,?, 'fake-provides')
`, charmRelationUUID.String(), charmUUID, kind, kind)
}

// addRelation inserts a new relation into the database with the given UUID and default relation and life IDs.
func (s *watcherSuite) addRelation(c *gc.C, relationUUID relation.UUID) {
	s.arrange(c, `
INSERT INTO relation (uuid, life_id, relation_id) 
VALUES (?,0,?)
`, relationUUID, 1)
}

// addRelationEndpoint inserts a relation endpoint into the database using the provided UUIDs for relation and endpoint.
func (s *watcherSuite) addRelationEndpoint(c *gc.C, relationEndpointUUID relation.EndpointUUID, relationUUID relation.UUID, applicationEndpointUUID uuid.UUID) {
	s.arrange(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?,?,?)
`, relationEndpointUUID, relationUUID, applicationEndpointUUID.String())
}

// addRelationStatus inserts a relation_status row into the database using the
// provided UUID for relation and status id.
func (s *watcherSuite) addRelationStatus(c *gc.C, relationUUID relation.UUID, status_id int) {
	s.arrange(c, `
INSERT INTO relation_status (relation_uuid, relation_status_type_id, updated_at)
VALUES (?,?,?)
`, relationUUID, status_id, time.Now())
}

// addUnit adds a new unit to the specified application in the database with
// the given UUID and name.
func (s *watcherSuite) addUnit(
	c *gc.C,
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
func (s *watcherSuite) arrange(c *gc.C, query string, args ...any) {
	s.query(c, func(err error) gc.CommentInterface {
		return gc.Commentf("(Arrange) failed to populate DB: %v",
			errors.ErrorStack(err))
	}, query, args...)
}

// act is dedicated to update the db during the test, as an action
func (s *watcherSuite) act(c *gc.C, query string, args ...any) {
	s.query(c, func(err error) gc.CommentInterface {
		return gc.Commentf("(Act) failed to update DB: %v",
			errors.ErrorStack(err))
	}, query, args...)
}

// query executes a database query within a standard transaction. If something goes wrong,
// the assertion allows to define a specific error as comment interface.
func (s *watcherSuite) query(c *gc.C, comment func(error) gc.CommentInterface, query string, args ...any) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %s)", err, query, args)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil, comment(err))
}
