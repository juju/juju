// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/charm"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type relationNetworkStateSuite struct {
	baseSuite
}

func TestRelationNetworkStateSuite(t *testing.T) {
	tc.Run(t, &relationNetworkStateSuite{})
}

func (s *relationNetworkStateSuite) TestAddRelationNetworkIngress(c *tc.C) {
	// Arrange
	relationUUID := s.createTestRelation(c)
	cidrs := []string{"192.0.2.0/24", "198.51.100.0/24"}

	// Act
	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), cidrs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtainedCIDRs := s.readRelationNetworkIngress(c, relationUUID.String())
	c.Check(obtainedCIDRs, tc.SameContents, cidrs)
}

func (s *relationNetworkStateSuite) TestAddRelationNetworkIngressSingleCIDR(c *tc.C) {
	// Arrange
	relationUUID := s.createTestRelation(c)
	cidr := []string{"192.0.2.0/24"}

	// Act
	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), cidr)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtainedCIDRs := s.readRelationNetworkIngress(c, relationUUID.String())
	c.Check(obtainedCIDRs, tc.DeepEquals, cidr)
}

func (s *relationNetworkStateSuite) TestAddRelationNetworkIngressMultipleCalls(c *tc.C) {
	// Arrange
	relationUUID := s.createTestRelation(c)
	firstCIDRs := []string{"192.0.2.0/24"}
	secondCIDRs := []string{"198.51.100.0/24", "203.0.113.0/24"}

	// Act - First call
	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), firstCIDRs)
	c.Assert(err, tc.ErrorIsNil)

	// Act - Second call
	err = s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), secondCIDRs)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	obtainedCIDRs := s.readRelationNetworkIngress(c, relationUUID.String())
	expectedCIDRs := append(firstCIDRs, secondCIDRs...)
	c.Check(obtainedCIDRs, tc.SameContents, expectedCIDRs)
}

func (s *relationNetworkStateSuite) TestAddRelationNetworkIngressDuplicateCIDR(c *tc.C) {
	// Arrange
	relationUUID := s.createTestRelation(c)
	cidr := []string{"192.0.2.0/24"}

	// Act - First insertion
	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), cidr)
	c.Assert(err, tc.ErrorIsNil)

	// Act - Second insertion of same CIDR
	err = s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), cidr)

	// Assert - Should fail due to primary key constraint
	c.Assert(err, tc.ErrorMatches, `.*inserting relation network ingress for relation.*`)
}

func (s *relationNetworkStateSuite) TestAddRelationNetworkIngressMultipleRelations(c *tc.C) {
	// Arrange
	relationUUID1 := s.createTestRelationWithNames(c, "app1", "app2")
	relationUUID2 := s.createTestRelationWithNames(c, "app3", "app4")
	cidr := []string{"192.0.2.0/24"}

	// Act
	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID1.String(), cidr)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.AddRelationNetworkIngress(c.Context(), relationUUID2.String(), cidr)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	obtainedCIDRs1 := s.readRelationNetworkIngress(c, relationUUID1.String())
	c.Check(obtainedCIDRs1, tc.DeepEquals, cidr)

	obtainedCIDRs2 := s.readRelationNetworkIngress(c, relationUUID2.String())
	c.Check(obtainedCIDRs2, tc.DeepEquals, cidr)
}

func (s *relationNetworkStateSuite) TestAddRelationNetworkIngressInvalidRelation(c *tc.C) {
	// Arrange
	nonExistentRelationUUID := internaluuid.MustNewUUID().String()
	cidr := []string{"192.0.2.0/24"}

	// Act
	err := s.state.AddRelationNetworkIngress(c.Context(), nonExistentRelationUUID, cidr)

	// Assert - Should return RelationNotFound
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *relationNetworkStateSuite) TestAddRelationNetworkIngressMultipleCIDRsInSingleCall(c *tc.C) {
	// Arrange
	relationUUID := s.createTestRelation(c)
	cidrs := []string{
		"192.0.2.0/24",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"2001:db8::/32",
	}

	// Act
	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), cidrs)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtainedCIDRs := s.readRelationNetworkIngress(c, relationUUID.String())
	c.Check(obtainedCIDRs, tc.SameContents, cidrs)
}

func (s *relationNetworkStateSuite) TestAddRelationNetworkIngressTransactional(c *tc.C) {
	// Arrange
	relationUUID := s.createTestRelation(c)
	existingCIDR := []string{"192.0.2.0/24"}

	// Add an existing CIDR
	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), existingCIDR)
	c.Assert(err, tc.ErrorIsNil)

	// Try to add multiple CIDRs where one is a duplicate
	cidrs := []string{"198.51.100.0/24", "203.0.113.0/24"}
	cidrs = append(cidrs, existingCIDR...)

	// Act
	err = s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), cidrs)

	// Assert - Should fail
	c.Assert(err, tc.ErrorMatches, `.*inserting relation network ingress for relation.*`)

	// Verify that the transaction was rolled back and no new CIDRs were added
	obtainedCIDRs := s.readRelationNetworkIngress(c, relationUUID.String())
	c.Check(obtainedCIDRs, tc.DeepEquals, existingCIDR)
}

func (s *relationNetworkStateSuite) createTestRelation(c *tc.C) internaluuid.UUID {
	return s.createTestRelationWithNames(c, "app1", "app2")
}

func (s *relationNetworkStateSuite) createTestRelationWithNames(c *tc.C, appName1, appName2 string) internaluuid.UUID {
	return s.createTestRelationWithNamesAndLife(c, appName1, appName2, 0 /* alive */)
}

func (s *relationNetworkStateSuite) createTestRelationWithNamesAndLife(c *tc.C, appName1, appName2 string, lifeID int) internaluuid.UUID {
	// Create a charm
	charmUUID := s.addCharm(c)
	s.addCharmMetadata(c, charmUUID, false)

	// Add a relation to the charm
	relation := charm.Relation{
		Name:      "db",
		Role:      charm.RoleProvider,
		Interface: "db",
		Scope:     charm.ScopeGlobal,
	}
	relationUUID := s.addCharmRelation(c, charmUUID, relation)

	// Create two applications
	appUUID1 := s.addApplication(c, charmUUID, appName1)
	s.addApplicationEndpoint(c, appUUID1, relationUUID)

	appUUID2 := s.addApplication(c, charmUUID, appName2)
	s.addApplicationEndpoint(c, appUUID2, relationUUID)

	// Create a relation between the two applications
	s.relationCount++
	relUUID := internaluuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var globalScopeID int
		if err := tx.QueryRowContext(ctx, `SELECT id FROM charm_relation_scope WHERE name='global'`).Scan(&globalScopeID); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation (uuid, life_id, relation_id, scope_id)
VALUES (?, ?, ?, ?)`, relUUID.String(), lifeID, s.relationCount, globalScopeID); err != nil {
			return err
		}

		// Get endpoint UUIDs for both apps
		var ep1, ep2 string
		qEP := `
SELECT ae.uuid
FROM   application_endpoint ae
JOIN   charm_relation cr ON cr.uuid = ae.charm_relation_uuid
WHERE  ae.application_uuid = ? AND cr.name = ?`

		if err := tx.QueryRowContext(ctx, qEP, appUUID1.String(), "db").Scan(&ep1); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, qEP, appUUID2.String(), "db").Scan(&ep2); err != nil {
			return err
		}

		// Insert relation endpoints
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, internaluuid.MustNewUUID().String(), relUUID.String(), ep1); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relation_endpoint (uuid, relation_uuid, endpoint_uuid)
VALUES (?, ?, ?)`, internaluuid.MustNewUUID().String(), relUUID.String(), ep2); err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return relUUID
}

func (s *relationNetworkStateSuite) createTestDyingRelation(c *tc.C) internaluuid.UUID {
	return s.createTestRelationWithNamesAndLife(c, "app1", "app2", 1 /* dying */)
}

func (s *relationNetworkStateSuite) createTestDeadRelation(c *tc.C) internaluuid.UUID {
	return s.createTestRelationWithNamesAndLife(c, "app1", "app2", 2 /* dead */)
}

func (s *relationNetworkStateSuite) TestAddRelationNetworkIngressDyingRelation(c *tc.C) {
	// Arrange
	relationUUID := s.createTestDyingRelation(c)
	cidr := []string{"192.0.2.0/24"}

	// Act
	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), cidr)

	// Assert - Should return RelationNotAlive
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotAlive)
}

func (s *relationNetworkStateSuite) TestAddRelationNetworkIngressDeadRelation(c *tc.C) {
	// Arrange
	relationUUID := s.createTestDeadRelation(c)
	cidr := []string{"192.0.2.0/24"}

	// Act
	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), cidr)

	// Assert - Should return RelationNotAlive
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotAlive)
}

func (s *relationNetworkStateSuite) TestGetRelationNetworkIngress(c *tc.C) {
	// Arrange
	relationUUID := s.createTestRelation(c)
	cidrs := []string{"192.0.2.0/24", "198.51.100.0/24"}

	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), cidrs)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	obtainedCIDRs, err := s.state.GetRelationNetworkIngress(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.SameContents, cidrs)
}

func (s *relationNetworkStateSuite) TestGetRelationNetworkIngressSingleCIDR(c *tc.C) {
	// Arrange
	relationUUID := s.createTestRelation(c)
	cidr := []string{"192.0.2.0/24"}

	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), cidr)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	obtainedCIDRs, err := s.state.GetRelationNetworkIngress(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.DeepEquals, cidr)
}

func (s *relationNetworkStateSuite) TestGetRelationNetworkIngressEmpty(c *tc.C) {
	// Arrange
	relationUUID := s.createTestRelation(c)

	// Act - No CIDRs added
	obtainedCIDRs, err := s.state.GetRelationNetworkIngress(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.HasLen, 0)
}

func (s *relationNetworkStateSuite) TestGetRelationNetworkIngressInvalidRelation(c *tc.C) {
	// Arrange
	nonExistentRelationUUID := internaluuid.MustNewUUID().String()

	// Act
	obtainedCIDRs, err := s.state.GetRelationNetworkIngress(c.Context(), nonExistentRelationUUID)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
	c.Check(obtainedCIDRs, tc.IsNil)
}

func (s *relationNetworkStateSuite) TestGetRelationNetworkIngressDyingRelation(c *tc.C) {
	// Arrange
	relationUUID := s.createTestDyingRelation(c)

	// Act
	obtainedCIDRs, err := s.state.GetRelationNetworkIngress(c.Context(), relationUUID.String())

	// Assert - Should succeed and return empty slice (no CIDRs added)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.HasLen, 0)
}

// TestGetRelationNetworkIngressDeadRelation tests that GetRelationNetworkIngress
// successfully retrieves ingress network CIDRs even when the relation is dead.
func (s *relationNetworkStateSuite) TestGetRelationNetworkIngressDeadRelation(c *tc.C) {
	// Arrange
	relationUUID := s.createTestDeadRelation(c)

	// Act
	obtainedCIDRs, err := s.state.GetRelationNetworkIngress(c.Context(), relationUUID.String())

	// Assert - Should succeed and return empty slice (no CIDRs added)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.HasLen, 0)
}

func (s *relationNetworkStateSuite) TestGetRelationNetworkIngressMultipleCIDRs(c *tc.C) {
	// Arrange
	relationUUID := s.createTestRelation(c)
	cidrs := []string{
		"192.0.2.0/24",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"2001:db8::/32",
	}

	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), cidrs)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	obtainedCIDRs, err := s.state.GetRelationNetworkIngress(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedCIDRs, tc.SameContents, cidrs)
}

// TestGetRelationNetworkIngressSortedOrder tests that GetRelationNetworkIngress
// returns CIDRs in sorted order.
func (s *relationNetworkStateSuite) TestGetRelationNetworkIngressSortedOrder(c *tc.C) {
	// Arrange
	relationUUID := s.createTestRelation(c)
	cidrs := []string{"203.0.113.0/24", "192.0.2.0/24", "198.51.100.0/24"}

	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), cidrs)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	obtainedCIDRs, err := s.state.GetRelationNetworkIngress(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	expectedSorted := []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24"}
	c.Check(obtainedCIDRs, tc.DeepEquals, expectedSorted)
}

// TestGetRelationNetworkIngressMultipleRelations tests that GetRelationNetworkIngress
// only returns CIDRs for the specified relation.
func (s *relationNetworkStateSuite) TestGetRelationNetworkIngressMultipleRelations(c *tc.C) {
	// Arrange
	relationUUID1 := s.createTestRelationWithNames(c, "app1", "app2")
	relationUUID2 := s.createTestRelationWithNames(c, "app3", "app4")

	cidrs1 := []string{"192.0.2.0/24"}
	cidrs2 := []string{"198.51.100.0/24", "203.0.113.0/24"}

	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID1.String(), cidrs1)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.AddRelationNetworkIngress(c.Context(), relationUUID2.String(), cidrs2)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	obtainedCIDRs1, err := s.state.GetRelationNetworkIngress(c.Context(), relationUUID1.String())
	c.Assert(err, tc.ErrorIsNil)

	obtainedCIDRs2, err := s.state.GetRelationNetworkIngress(c.Context(), relationUUID2.String())
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	c.Check(obtainedCIDRs1, tc.DeepEquals, cidrs1)
	c.Check(obtainedCIDRs2, tc.SameContents, cidrs2)
}

// TestGetRelationNetworkIngressAfterMultipleAdds tests that GetRelationNetworkIngress
// retrieves all accumulated CIDRs after multiple add operations.
func (s *relationNetworkStateSuite) TestGetRelationNetworkIngressAfterMultipleAdds(c *tc.C) {
	// Arrange
	relationUUID := s.createTestRelation(c)
	firstCIDRs := []string{"192.0.2.0/24"}
	secondCIDRs := []string{"198.51.100.0/24", "203.0.113.0/24"}

	err := s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), firstCIDRs)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.AddRelationNetworkIngress(c.Context(), relationUUID.String(), secondCIDRs)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	obtainedCIDRs, err := s.state.GetRelationNetworkIngress(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	allCIDRs := append(firstCIDRs, secondCIDRs...)
	c.Check(obtainedCIDRs, tc.SameContents, allCIDRs)
}

// readRelationNetworkIngress reads all CIDRs for a given relation from the
// relation_network_ingress table.
func (s *relationNetworkStateSuite) readRelationNetworkIngress(c *tc.C, relationUUID string) []string {
	rows, err := s.DB().QueryContext(c.Context(), `
SELECT cidr FROM relation_network_ingress
WHERE relation_uuid = ?
ORDER BY cidr`, relationUUID)
	c.Assert(err, tc.IsNil)
	defer func() { _ = rows.Close() }()

	var cidrs []string
	for rows.Next() {
		var cidr string
		err = rows.Scan(&cidr)
		c.Assert(err, tc.IsNil)
		cidrs = append(cidrs, cidr)
	}
	return cidrs
}
