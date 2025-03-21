// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/relation/service"
	"github.com/juju/juju/domain/relation/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite

	charmUUID         string
	charmRelationUUID string
	appUUID           string
	appEndpointUUID   string
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.charmUUID = uuid.MustNewUUID().String()
	s.charmRelationUUID = uuid.MustNewUUID().String()
	s.appUUID = uuid.MustNewUUID().String()

	// Populate DB with charm, application and endpoints
	s.addCharm(c, s.charmUUID)
	s.addCharmRelation(c, s.charmUUID, s.charmRelationUUID)
	s.addApplication(c, s.charmUUID, s.appUUID, "my-application")
	s.addApplicationEndpoint(c, s.appEndpointUUID, s.appUUID, s.charmRelationUUID)
}

// TestWatchUnitRelations ensures the unit relation watcher correctly captures
// create, update, and delete events in the database.
func (s *watcherSuite) TestWatchUnitRelations(c *gc.C) {

	// Arrange: create the required state, with one relation endpoint and related
	// objects.
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "relation_application_setting")
	relationUUID := uuid.MustNewUUID().String()
	relationEndpointUUID := uuid.MustNewUUID().String()

	// Populate DB with relation endpoint.
	s.addRelation(c, relationUUID)
	s.addRelationEndpoint(c, relationEndpointUUID, relationUUID, s.appEndpointUUID)

	svc := s.setupService(c, factory)
	watcher, err := svc.WatchApplicationSettings(context.Background(), relation.UUID(relationUUID), coreapplication.ID(s.appUUID))
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

func (s *watcherSuite) setupService(c *gc.C, factory domain.WatchableDBFactory) *service.WatchableService {
	modelDB := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	return service.NewWatchableService(
		state.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c)),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		loggertesting.WrapCheckLog(c),
	)
}

// addApplication adds a new application to the database with the specified UUID and name.
func (s *watcherSuite) addApplication(c *gc.C, charmUUID, appUUID, appName string) {
	s.arrange(c, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) 
VALUES (?, ?, ?, ?, ?)
`, appUUID, appName, 0 /* alive */, charmUUID, network.AlphaSpaceId)
}

// addApplicationEndpoint inserts a new application endpoint into the database with the specified UUIDs and relation data.
func (s *watcherSuite) addApplicationEndpoint(c *gc.C, applicationEndpointUUID string, applicationUUID, charmRelationUUID string) {
	s.arrange(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?,?,?,0)
`, applicationEndpointUUID, applicationUUID, charmRelationUUID)
}

// addCharm inserts a new charm into the database with a predefined UUID, reference name, and architecture ID.
func (s *watcherSuite) addCharm(c *gc.C, charmUUID string) {
	s.arrange(c, `
INSERT INTO charm (uuid, reference_name, architecture_id) 
VALUES (?, 'app', 0)
`, charmUUID)
}

// addCharmRelation inserts a new charm relation into the database with the given UUID and predefined attributes.
func (s *watcherSuite) addCharmRelation(c *gc.C, charmUUID, charmRelationUUID string) {
	s.arrange(c, `
INSERT INTO charm_relation (uuid, charm_uuid, kind_id, name) 
VALUES (?, ?, 0, 'fake-provides')
`, charmRelationUUID, charmUUID)
}

// addRelation inserts a new relation into the database with the given UUID and default relation and life IDs.
func (s *watcherSuite) addRelation(c *gc.C, relationUUID string) {
	s.arrange(c, `
INSERT INTO relation (uuid, life_id, relation_id) 
VALUES (?,0,?)
`, relationUUID, 1)
}

// addRelationEndpoint inserts a relation endpoint into the database using the provided UUIDs for relation and endpoint.
func (s *watcherSuite) addRelationEndpoint(c *gc.C, relationEndpointUUID string, relationUUID string, applicationEndpointUUID string) {
	s.arrange(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?,?,?)
`, relationEndpointUUID, relationUUID, applicationEndpointUUID)
}

// arrange is dedicated to build up the initial state of the db during a test
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
