// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	corecharmtesting "github.com/juju/juju/core/charm/testing"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	appcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/status"
	internalerrors "github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type baseSuite struct {
	schematesting.ModelSuite
	state *State

	relationCount int
}

func (s *baseSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory(), coremodel.UUID(s.ModelUUID()), clock.WallClock, loggertesting.WrapCheckLog(c))
	s.relationCount = 0

	c.Cleanup(func() {
		s.state = nil
		s.relationCount = 0
	})
}

func (s *baseSuite) readOffers(c *tc.C) []nameAndUUID {
	rows, err := s.DB().QueryContext(c.Context(), `SELECT * FROM offer`)
	c.Assert(err, tc.IsNil)
	defer func() { _ = rows.Close() }()
	foundOffers := []nameAndUUID{}
	for rows.Next() {
		var found nameAndUUID
		err = rows.Scan(&found.UUID, &found.Name)
		c.Assert(err, tc.IsNil)
		foundOffers = append(foundOffers, found)
	}
	return foundOffers
}

func (s *baseSuite) readOfferEndpoints(c *tc.C) []offerEndpoint {
	rows, err := s.DB().QueryContext(c.Context(), `SELECT * FROM offer_endpoint`)
	c.Assert(err, tc.IsNil)
	defer func() { _ = rows.Close() }()
	foundOfferEndpoints := []offerEndpoint{}
	for rows.Next() {
		var found offerEndpoint
		err = rows.Scan(&found.OfferUUID, &found.EndpointUUID)
		c.Assert(err, tc.IsNil)
		foundOfferEndpoints = append(foundOfferEndpoints, found)
	}
	return foundOfferEndpoints
}

// query executes a given SQL query with optional arguments within a
// transactional context using the test database.
func (s *baseSuite) query(c *tc.C, query string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return internalerrors.Errorf("%w: query: %s (args: %s)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) populate DB: %v",
		internalerrors.ErrorStack(err)))
}

// addApplication adds a new application to the database with the specified
// charm UUID and application name. It returns the application UUID.
func (s *baseSuite) addApplication(c *tc.C, charmUUID corecharm.ID, appName string) coreapplication.UUID {
	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.query(c, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid)
VALUES (?, ?, ?, ?, ?)
`, appUUID, appName, 0 /* alive */, charmUUID.String(), network.AlphaSpaceId)
	return appUUID
}

// addApplicationEndpoint inserts a new application endpoint into the database
// with the specified UUIDs. Returns the endpoint uuid.
func (s *baseSuite) addApplicationEndpoint(c *tc.C, applicationUUID coreapplication.UUID,
	charmRelationUUID string) string {
	applicationEndpointUUID := internaluuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?, ?, ?, ?)
`, applicationEndpointUUID, applicationUUID, charmRelationUUID, network.AlphaSpaceId)
	return applicationEndpointUUID
}

// addCharm inserts a new charm into the database and returns the UUID.
func (s *baseSuite) addCharm(c *tc.C) corecharm.ID {
	charmUUID := corecharmtesting.GenCharmID(c)
	// The UUID is also used as the reference_name as there is a unique
	// constraint on the reference_name, revision.
	s.query(c, `
INSERT INTO charm (uuid, reference_name, revision)
VALUES (?, ?, 42)
`, charmUUID, charmUUID)
	return charmUUID
}

// addCMRCharm inserts a new charm, where the source is CMR, into the
// database and returns the UUID.
func (s *baseSuite) addCMRCharm(c *tc.C) corecharm.ID {
	charmUUID := corecharmtesting.GenCharmID(c)
	// The UUID is also used as the reference_name as there is a unique
	// constraint on the reference_name, revision.
	s.query(c, `
INSERT INTO charm (uuid, reference_name, source_id, revision)
VALUES (?, ?, 
        (SELECT id FROM charm_source WHERE name = 'cmr'),
        42)
`, charmUUID, charmUUID)
	return charmUUID
}

// addCharm inserts a new charm into the database and returns the UUID.
func (s *baseSuite) addCharmMetadataWithDescription(c *tc.C, charmUUID corecharm.ID, description string) {
	// The UUID is also used as the reference_name as there is a unique
	// constraint on the reference_name, revision and source_id.
	s.query(c, `
INSERT INTO charm_metadata (charm_uuid, name, subordinate, description)
VALUES (?, ?, false, ?)
`, charmUUID, charmUUID, description)
}

func (s *baseSuite) addCharmMetadata(c *tc.C, charmUUID corecharm.ID, subordinate bool) {
	s.query(c, `
INSERT INTO charm_metadata (charm_uuid, name, subordinate)
VALUES (?, ?, ?)
`, charmUUID, charmUUID, subordinate)
}

// addCharmRelation inserts a new charm relation into the database with the
// given UUID and attributes. Returns the relation UUID.
func (s *baseSuite) addCharmRelation(c *tc.C, charmUUID corecharm.ID, r charm.Relation) string {
	charmRelationUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, interface, optional, capacity, scope_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, charmRelationUUID, charmUUID, r.Name, s.encodeRoleID(r.Role), r.Interface, r.Optional, r.Limit, s.encodeScopeID(r.Scope))
	return charmRelationUUID
}

// addRelation inserts a new relation into the database with default relation
// and life IDs. Returns the relation UUID.
func (s *baseSuite) addRelation(c *tc.C) corerelation.UUID {
	relationUUID := tc.Must(c, corerelation.NewUUID)
	s.query(c, `
INSERT INTO relation (uuid, life_id, relation_id, scope_id) 
VALUES (?, 0, ?, 0)
`, relationUUID, s.relationCount)
	s.relationCount++
	return relationUUID
}

func (s *baseSuite) addRelationEndpoint(c *tc.C, relationUUID, endpointUUID string) {
	s.query(c, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, internaluuid.MustNewUUID().String(), relationUUID, endpointUUID)
}

// addOffer inserts a new offer with offer_endpoints into the database. Returns
// the offer uuid.
func (s *baseSuite) addOffer(c *tc.C, offerName string, endpointUUIDs []string) offer.UUID {
	offerUUID := tc.Must(c, offer.NewUUID)

	s.query(c, `
INSERT INTO offer (uuid, name) VALUES (?, ?)`, offerUUID, offerName)
	for _, endpoint := range endpointUUIDs {
		s.query(c, `
INSERT INTO offer_endpoint (offer_uuid, endpoint_uuid) VALUES (?, ?)`, offerUUID, endpoint)
	}

	return offerUUID
}

func (s *baseSuite) addOfferConnection(c *tc.C, offerUUID offer.UUID, statusID status.RelationStatusType) string {
	relUUID := s.addRelation(c)

	connUUID := tc.Must(c, internaluuid.NewUUID).String()
	s.query(c, `
INSERT INTO offer_connection (uuid, offer_uuid, remote_relation_uuid, username)
VALUES (?, ?, ?, "bob")`, connUUID, offerUUID, relUUID)

	s.query(c, `
INSERT INTO relation_status (relation_uuid, relation_status_type_id, updated_at)
VALUES (?, ?, 0)`, relUUID, statusID)
	return relUUID.String()
}

// encodeRoleID returns the ID used in the database for the given charm role. This
// reflects the contents of the charm_relation_role table.
func (s *baseSuite) encodeRoleID(role charm.RelationRole) int {
	return map[charm.RelationRole]int{
		charm.RoleProvider: 0,
		charm.RoleRequirer: 1,
		charm.RolePeer:     2,
	}[role]
}

// encodeScopeID returns the ID used in the database for the given charm scope. This
// reflects the contents of the charm_relation_scope table.
func (s *baseSuite) encodeScopeID(role charm.RelationScope) int {
	return map[charm.RelationScope]int{
		charm.ScopeGlobal:    0,
		charm.ScopeContainer: 1,
	}[role]
}

func (s *baseSuite) assertUnitNames(c *tc.C, applicationUUID coreapplication.UUID, expectedNames []string) {
	c.Helper()

	var names []string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT name
FROM unit
WHERE application_uuid = ?`, applicationUUID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			names = append(names, name)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(names, tc.SameContents, expectedNames)
}

func (s *baseSuite) assertApplicationRemoteConsumer(c *tc.C, applicationUUID string) {
	c.Helper()

	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT COUNT(*)
FROM application_remote_consumer
WHERE consumer_application_uuid = ?
`, applicationUUID).Scan(&count)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)
}

func (s *baseSuite) createOffer(c *tc.C, offerUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Create an offer record
		_, err := tx.Exec(`
INSERT INTO offer (uuid, name)
VALUES (?, 'test-offer')
`, offerUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) createApplication(c *tc.C, applicationUUID coreapplication.UUID, charmUUID string, offerUUID string) {
	s.createApplicationWithLife(c, applicationUUID, charmUUID, offerUUID, life.Alive)
}

func (s *baseSuite) createDeadApplication(c *tc.C, applicationUUID coreapplication.UUID, charmUUID string, offerUUID string) {
	s.createApplicationWithLife(c, applicationUUID, charmUUID, offerUUID, life.Dead)
}

func (s *baseSuite) createDyingApplication(c *tc.C, applicationUUID coreapplication.UUID, charmUUID string, offerUUID string) {
	s.createApplicationWithLife(c, applicationUUID, charmUUID, offerUUID, life.Dying)
}

func (s *baseSuite) createApplicationWithLife(c *tc.C, applicationUUID coreapplication.UUID, charmUUID string, offerUUID string, l life.Life) {
	c.Helper()

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Create an application record
		_, err := tx.Exec(`
INSERT INTO application (uuid, name, charm_uuid, life_id, space_uuid)
VALUES (?, ?, ?, ?, ?)
`, applicationUUID, applicationUUID, charmUUID, l, network.AlphaSpaceId)
		if err != nil {
			return err
		}

		charmRelationUUID := tc.Must(c, internaluuid.NewUUID).String()
		charmEndpointUUID := tc.Must(c, internaluuid.NewUUID).String()

		// Insert charm_relation and application_endpoint records
		insertCharmRelation := `INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name) VALUES (?, ?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertCharmRelation, charmRelationUUID, charmUUID, "0", "0", "endpoint0")
		if err != nil {
			return err
		}
		insertEndpoint := `INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?, ?, ?, ?)`
		_, err = tx.ExecContext(ctx, insertEndpoint, charmEndpointUUID, applicationUUID, network.AlphaSpaceId, charmRelationUUID)
		if err != nil {
			return err
		}
		// Insert an offer endpoint record if it's not empty.
		if offerUUID == "" {
			return nil
		}
		insertOfferEndpoint := `INSERT INTO offer_endpoint (offer_uuid, endpoint_uuid) VALUES (?, ?)`
		_, err = tx.ExecContext(ctx, insertOfferEndpoint, offerUUID, charmEndpointUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) createCharm(c *tc.C, charmUUID string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Create a charm record
		_, err := tx.Exec(`
INSERT INTO charm (uuid, reference_name, source_id)
VALUES (?, ?, 1)
`, charmUUID, charmUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *baseSuite) createCharmRelation(c *tc.C, charmUUID, endpointName string) string {
	charmRelUUID1 := tc.Must(c, internaluuid.NewUUID).String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO charm_relation (uuid, charm_uuid, scope_id, role_id, name) VALUES (?, ?, 0, 0, ?)`,
			charmRelUUID1, charmUUID, endpointName)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return charmRelUUID1
}

func (s *baseSuite) assertApplicationRemoteOfferer(c *tc.C, uuid string) {
	c.Helper()

	var (
		gotLifeID   int
		gotMacaroon string
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT aro.life_id, aro.macaroon 
FROM application_remote_offerer AS aro
JOIN application AS a ON aro.application_uuid = a.uuid
WHERE a.uuid=?`, uuid).
			Scan(&gotLifeID, &gotMacaroon)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotLifeID, tc.Equals, 0)
	c.Check(gotMacaroon, tc.Equals, "encoded macaroon")
}

func (s *baseSuite) assertApplicationRemoteOffererStatus(c *tc.C, uuid string) {
	c.Helper()

	var gotStatusID int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT aros.status_id 
FROM application_remote_offerer_status AS aros
JOIN application_remote_offerer AS aro ON aros.application_remote_offerer_uuid = aro.uuid
JOIN application AS a ON aro.application_uuid = a.uuid
WHERE a.uuid=?`, uuid).
			Scan(&gotStatusID)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotStatusID, tc.Equals, 1)
}

func (s *baseSuite) assertApplication(c *tc.C, uuid string) {
	c.Helper()

	var gotName string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM application WHERE uuid=?", uuid).
			Scan(&gotName)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotName, tc.Equals, "foo")
}

func (s *baseSuite) assertRelation(c *tc.C, relationUUID string, relationID int) {
	c.Helper()

	var (
		gotUUID            string
		gotID              int
		gotLifID           int
		gotScopeID         int
		gotSuspended       bool
		gotSuspendedReason sql.Null[string]
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT uuid, relation_id, life_id, scope_id, suspended, suspended_reason
FROM relation WHERE relation_id=?
`, relationID).
			Scan(&gotUUID, &gotID, &gotLifID, &gotScopeID, &gotSuspended, &gotSuspendedReason)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotUUID, tc.Equals, relationUUID)
	c.Check(gotID, tc.Equals, relationID)
	c.Check(gotLifID, tc.Equals, 0)   // life.Alive
	c.Check(gotScopeID, tc.Equals, 0) // scope.Global
	c.Check(gotSuspended, tc.Equals, false)
	c.Check(gotSuspendedReason.V, tc.Equals, "")
}

func (s *baseSuite) assertRelationEndpoints(c *tc.C, relationUUID, app1UUID, app2UUID string) {
	c.Helper()

	appUUIDs := set.NewStrings(app1UUID, app2UUID)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT ae.application_uuid
FROM   relation_endpoint AS re
JOIN   application_endpoint AS ae ON re.endpoint_uuid = ae.uuid
WHERE  relation_uuid = ?
`, relationUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var appUUID string

			if err := rows.Scan(&appUUID); err != nil {
				return err
			}
			if c.Check(appUUIDs.Contains(appUUID), tc.Equals, true) {
				appUUIDs.Remove(appUUID)
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(appUUIDs.IsEmpty(), tc.Equals, true, tc.Commentf("relation_endpoint with app %q, not found", appUUIDs.SortedValues()))
}

func (s *baseSuite) assertRelationStatusJoining(c *tc.C, relationUUID string) {
	c.Helper()

	var statusID int

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT relation_status_type_id
FROM   relation_status
WHERE  relation_uuid = ?
`, relationUUID).Scan(&statusID)

		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	expectedRelationID := tc.Must1(c, status.EncodeRelationStatus, status.RelationStatusTypeJoining)
	c.Check(statusID, tc.Equals, expectedRelationID)
}

func (s *baseSuite) assertCharmMetadata(c *tc.C, appUUID, charmUUID string, expected appcharm.Charm) {
	c.Helper()

	var (
		gotReferenceName string
		gotSourceName    string
		gotCharmName     string

		gotProvides = make(map[string]appcharm.Relation)
		gotRequires = make(map[string]appcharm.Relation)
	)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT ch.reference_name, cs.name, cm.name
FROM application
JOIN charm AS ch ON application.charm_uuid = ch.uuid
JOIN charm_metadata AS cm ON ch.uuid = cm.charm_uuid
JOIN charm_source AS cs ON ch.source_id = cs.id
WHERE application.uuid=?`, appUUID).
			Scan(&gotReferenceName, &gotSourceName, &gotCharmName)
		if err != nil {
			return err
		}

		rows, err := tx.QueryContext(ctx, `
SELECT name, role_id, interface, capacity, scope_id
FROM charm_relation
WHERE charm_uuid = ?`, charmUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var (
				relName  string
				roleID   int
				iface    string
				capacity int
				scopeID  int
			)
			if err := rows.Scan(&relName, &roleID, &iface, &capacity, &scopeID); err != nil {
				return err
			}
			rel := appcharm.Relation{
				Name:      relName,
				Interface: iface,
				Limit:     capacity,
			}
			switch roleID {
			case 0:
				rel.Role = appcharm.RoleProvider
			case 1:
				rel.Role = appcharm.RoleRequirer
			default:
				return internalerrors.Errorf("unknown role ID %d", roleID)
			}
			switch scopeID {
			case 0:
				rel.Scope = appcharm.ScopeGlobal
			default:
				return internalerrors.Errorf("unknown scope ID %d", scopeID)
			}
			switch rel.Role {
			case appcharm.RoleProvider:
				gotProvides[rel.Name] = rel
			case appcharm.RoleRequirer:
				gotRequires[rel.Name] = rel
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(gotReferenceName, tc.Equals, expected.ReferenceName)
	c.Check(gotSourceName, tc.Equals, "cmr")
	c.Check(gotCharmName, tc.Equals, expected.Metadata.Name)

	// Every remote application will automatically get a "juju-info" provider
	// relation.
	// Check that it has been added correctly.
	provides := make(map[string]appcharm.Relation)
	maps.Copy(provides, expected.Metadata.Provides)
	provides["juju-info"] = appcharm.Relation{
		Name:      "juju-info",
		Role:      appcharm.RoleProvider,
		Interface: "juju-info",
		Limit:     0,
		Scope:     appcharm.ScopeGlobal,
	}

	c.Check(gotProvides, tc.DeepEquals, provides)
	c.Check(gotRequires, tc.DeepEquals, expected.Metadata.Requires)
}

type applicationEndpoint struct {
	charmRelationUUID string
	charmRelationName string
	spaceName         string
}

func (s *baseSuite) fetchApplicationEndpoints(c *tc.C, appID string) []applicationEndpoint {
	c.Helper()

	var endpoints []applicationEndpoint
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.Query(`
SELECT ae.charm_relation_uuid, s.name, cr.name AS charm_relation_name
FROM application_endpoint ae
JOIN charm_relation cr ON cr.uuid=ae.charm_relation_uuid
LEFT JOIN space s ON s.uuid=ae.space_uuid
WHERE ae.application_uuid=?
ORDER BY cr.name`, appID)
		defer func() { _ = rows.Close() }()
		if err != nil {
			return err
		}
		for rows.Next() {
			var (
				uuid              string
				spaceName         *string
				charmRelationName string
			)
			if err := rows.Scan(&uuid, &spaceName, &charmRelationName); err != nil {
				return err
			}
			endpoints = append(endpoints, applicationEndpoint{
				charmRelationUUID: uuid,
				charmRelationName: charmRelationName,
				spaceName:         nilEmpty(spaceName),
			})
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return endpoints
}

func newMacaroon(c *tc.C, id string) *macaroon.Macaroon {
	mac, err := macaroon.New(nil, []byte(id), "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	return mac
}

func nilEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// buildTestSyntheticCharm creates a synthetic charm from remote endpoints for testing.
func buildTestSyntheticCharm(appName string, endpoints []crossmodelrelation.RemoteApplicationEndpoint) appcharm.Charm {
	provides := make(map[string]appcharm.Relation)
	requires := make(map[string]appcharm.Relation)

	for _, ep := range endpoints {
		rel := appcharm.Relation{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Scope:     appcharm.ScopeGlobal,
		}
		switch ep.Role {
		case appcharm.RoleProvider:
			provides[ep.Name] = rel
		case appcharm.RoleRequirer:
			requires[ep.Name] = rel
		}
	}

	return appcharm.Charm{
		Metadata: appcharm.Metadata{
			Name:     appName,
			Provides: provides,
			Requires: requires,
		},
		Source:        appcharm.CMRSource,
		ReferenceName: appName,
	}
}
