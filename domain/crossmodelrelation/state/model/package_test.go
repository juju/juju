// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	corecharmtesting "github.com/juju/juju/core/charm/testing"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/offer"
	corerelation "github.com/juju/juju/core/relation"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
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
			return errors.Errorf("%w: query: %s (args: %s)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) populate DB: %v",
		errors.ErrorStack(err)))
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

func (s *baseSuite) addRelationIngressNetwork(c *tc.C, relationUUID, ingress string) {
	s.query(c, `
INSERT INTO relation_network_ingress (relation_uuid, cidr)
VALUES (?, ?)`, relationUUID, ingress)
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
