// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	corecharm "github.com/juju/juju/core/charm"
	corecharmtesting "github.com/juju/juju/core/charm/testing"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/relation"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)


// baseRelationSuite is a struct embedding ModelSuite for testing relation
// between application. It provides a set of builder function to create all
// the necessary context to actually create relation, like charms and applications
type baseRelationSuite struct {
	schematesting.ModelSuite
	state *State

	// relationCount helps generation of consecutive relation_id
	relationCount int
}

func (s *baseRelationSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

// Txn executes a transactional function within a database context,
// ensuring proper error handling and assertion.
func (s *baseRelationSuite) Txn(c *tc.C, fn func(ctx context.Context, tx *sqlair.TX) error) error {
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	return db.Txn(c.Context(), fn)
}

// query executes a given SQL query with optional arguments within a
// transactional context using the test database.
func (s *baseRelationSuite) query(c *tc.C, query string, args ...any) {

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %s)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}

// addApplication adds a new application to the database with the specified
// charm UUID and application name. It returns the application UUID.
func (s *baseRelationSuite) addApplication(c *tc.C, charmUUID corecharm.ID, appName string) coreapplication.ID {
	appUUID := coreapplicationtesting.GenApplicationUUID(c)
	s.query(c, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) 
VALUES (?, ?, ?, ?, ?)
`, appUUID, appName, 0 /* alive */, charmUUID.String(), network.AlphaSpaceId)
	return appUUID
}

// addApplicationEndpoint inserts a new application endpoint into the database
// with the specified UUIDs. Returns the endpoint uuid.
func (s *baseRelationSuite) addApplicationEndpoint(c *tc.C, applicationUUID coreapplication.ID,
	charmRelationUUID string) string {
	// TODO(gfouillet): introduce proper UUID for this one, from corerelation & corerelationtesting
	applicationEndpointUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?, ?, ?, ?)
`, applicationEndpointUUID, applicationUUID, charmRelationUUID, network.AlphaSpaceId)
	return applicationEndpointUUID
}

// addCharm inserts a new charm into the database and returns the UUID.
func (s *baseRelationSuite) addCharm(c *tc.C) corecharm.ID {
	charmUUID := corecharmtesting.GenCharmID(c)
	// The UUID is also used as the reference_name as there is a unique
	// constraint on the reference_name, revision and source_id.
	s.query(c, `
INSERT INTO charm (uuid, reference_name, architecture_id) 
VALUES (?, ?, 0)
`, charmUUID, charmUUID)
	return charmUUID
}

func (s *baseRelationSuite) addCharmMetadata(c *tc.C, charmUUID corecharm.ID, subordinate bool) {
	s.query(c, `
INSERT INTO charm_metadata (charm_uuid, name, subordinate) 
VALUES (?, ?, ?)
`, charmUUID, charmUUID, subordinate)
}

// addCharmRelation inserts a new charm relation into the database with the
// given UUID and attributes. Returns the relation UUID.
func (s *baseRelationSuite) addCharmRelation(c *tc.C, charmUUID corecharm.ID, r charm.Relation) string {
	// TODO(gfouillet): introduce proper UUID for this one, from corecharm and corecharmtesting
	charmRelationUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, interface, optional, capacity, scope_id) 
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, charmRelationUUID, charmUUID, r.Name, s.encodeRoleID(r.Role), r.Interface, r.Optional, r.Limit, s.encodeScopeID(r.Scope))
	return charmRelationUUID
}

// addCharmRelationWithDefaults inserts a new charm relation into the database
// with the given UUID and predefined attributes. Returns the relation UUID.
func (s *baseRelationSuite) addCharmRelationWithDefaults(c *tc.C, charmUUID corecharm.ID) string {
	// TODO(gfouillet): introduce proper UUID for this one, from corecharm and corecharmtesting
	charmRelationUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name) 
VALUES (?, ?, 0, 0, 'fake-provides')
`, charmRelationUUID, charmUUID)
	return charmRelationUUID
}

func (s *baseRelationSuite) doesUUIDExist(c *tc.C, table, uuid string) bool {
	found := false
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(fmt.Sprintf(`
SELECT uuid
FROM   %s
WHERE  uuid = ?
`, table), uuid).Scan(&uuid)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		found = true
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return found
}

// encodeRoleID returns the ID used in the database for the given charm role. This
// reflects the contents of the charm_relation_role table.
func (s *baseRelationSuite) encodeRoleID(role charm.RelationRole) int {
	return map[charm.RelationRole]int{
		charm.RoleProvider: 0,
		charm.RoleRequirer: 1,
		charm.RolePeer:     2,
	}[role]
}

// encodeStatusID returns the ID used in the database for the given relation
// status. This reflects the contents of the relation_status_type table.
func (s *baseRelationSuite) encodeStatusID(status corestatus.Status) int {
	return map[corestatus.Status]int{
		corestatus.Joining:    0,
		corestatus.Joined:     1,
		corestatus.Broken:     2,
		corestatus.Suspending: 3,
		corestatus.Suspended:  4,
	}[status]
}

// encodeScopeID returns the ID used in the database for the given charm scope. This
// reflects the contents of the charm_relation_scope table.
func (s *baseRelationSuite) encodeScopeID(role charm.RelationScope) int {
	return map[charm.RelationScope]int{
		charm.ScopeGlobal:    0,
		charm.ScopeContainer: 1,
	}[role]
}

// newEndpointIdentifier converts an endpoint string into a relation.EndpointIdentifier and asserts no parsing errors.
func (s *baseRelationSuite) newEndpointIdentifier(c *tc.C, endpoint string) relation.CandidateEndpointIdentifier {
	result, err := relation.NewCandidateEndpointIdentifier(endpoint)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to parse endpoint %q: %v", endpoint,
		errors.ErrorStack(err)))
	return result
}

func (s *baseRelationSuite) setLife(c *tc.C, table string, uuid string, dying life.Life) {
	s.query(c, fmt.Sprintf(`
UPDATE %s SET life_id = ?
WHERE uuid = ?`, table), dying, uuid)
}

// addRelation inserts a new relation into the database with default relation
// and life IDs. Returns the relation UUID.
func (s *baseRelationSuite) addRelation(c *tc.C) corerelation.UUID {
	relationUUID := corerelationtesting.GenRelationUUID(c)
	s.query(c, `
INSERT INTO relation (uuid, life_id, relation_id) 
VALUES (?,0,?)
`, relationUUID, s.relationCount)
	s.relationCount++
	return relationUUID
}

// addRelationEndpoint inserts a relation endpoint into the database
// using the provided UUIDs for relation. Returns the endpoint UUID.
func (s *baseRelationSuite) addRelationEndpoint(c *tc.C, relationUUID corerelation.UUID,
	applicationEndpointUUID string) string {
	// TODO(gfouillet): introduce proper UUID for this one, from corerelation & corerelationtesting
	relationEndpointUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?,?,?)
`, relationEndpointUUID, relationUUID, applicationEndpointUUID)
	return relationEndpointUUID
}

// addRelationUnit inserts a relation unit into the database using the
// provided UUIDs for relation. Returns the relation unit UUID.
func (s *baseRelationSuite) addRelationUnit(c *tc.C, unitUUID coreunit.UUID, relationEndpointUUID string) corerelation.UnitUUID {
	relationUnitUUID := corerelationtesting.GenRelationUnitUUID(c)
	s.query(c, `
INSERT INTO relation_unit (uuid, relation_endpoint_uuid, unit_uuid)
VALUES (?,?,?)
`, relationUnitUUID, relationEndpointUUID, unitUUID)
	return relationUnitUUID
}

// addRelationWithID inserts a new relation into the database with the given
// ID, and default life ID. Returns the relation UUID.
func (s *baseRelationSuite) addRelationWithID(c *tc.C, relationID int) corerelation.UUID {
	relationUUID := corerelationtesting.GenRelationUUID(c)
	s.query(c, `
INSERT INTO relation (uuid, life_id, relation_id) 
VALUES (?,0,?)
`, relationUUID, relationID)
	// avoid clashes when unit both addRelationHelper in the same method (even it should be avoided)
	if s.relationCount < relationID {
		s.relationCount = relationID + 1
	}
	return relationUUID
}

// addRelationWithLifeAndID inserts a new relation into the database with the
// given details. Returns the relation UUID.
func (s *baseRelationSuite) addRelationWithLifeAndID(c *tc.C, life corelife.Value, relationID int) corerelation.UUID {
	relationUUID := corerelationtesting.GenRelationUUID(c)
	s.query(c, `
INSERT INTO relation (uuid, relation_id, life_id)
SELECT ?,  ?, id
FROM life
WHERE value = ?
`, relationUUID, relationID, life)
	// avoid clashes when unit both addRelationHelper in the same method (even it should be avoided)
	if s.relationCount < relationID {
		s.relationCount = relationID + 1
	}
	return relationUUID
}

// addUnit adds a new unit to the specified application in the database with
// the given UUID and name. Returns the unit uuid.
func (s *baseRelationSuite) addUnit(c *tc.C, unitName coreunit.Name, appUUID coreapplication.ID, charmUUID corecharm.ID) coreunit.UUID {
	unitUUID := coreunittesting.GenUnitUUID(c)
	netNodeUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO net_node (uuid) 
VALUES (?)
ON CONFLICT DO NOTHING
`, netNodeUUID)

	s.query(c, `
INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid)
VALUES (?, ?, ?, ?, ?, ?)
`, unitUUID, unitName, 0 /* alive */, appUUID, charmUUID, netNodeUUID)
	return unitUUID
}

// addUnitWithLife adds a new unit to the specified application in the database with
// the given UUID, name and life. Returns the unit uuid.
func (s *baseRelationSuite) addUnitWithLife(c *tc.C, unitName coreunit.Name, appUUID coreapplication.ID,
	charmUUID corecharm.ID, life corelife.Value) coreunit.UUID {
	unitUUID := coreunittesting.GenUnitUUID(c)
	netNodeUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO net_node (uuid) 
VALUES (?)
ON CONFLICT DO NOTHING
`, netNodeUUID)

	s.query(c, `
INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid)
SELECT ?, ?, id, ?, ?, ?
FROM life
WHERE value = ?
`, unitUUID, unitName, appUUID, charmUUID, netNodeUUID, life)
	return unitUUID
}
