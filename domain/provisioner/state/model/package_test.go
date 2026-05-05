// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type modelStateSuite struct {
	schematesting.ModelSuite

	state *State
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &modelStateSuite{})
}

func (s *modelStateSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
}

// runQuery executes an SQL statement for test setup.
func (s *modelStateSuite) runQuery(c *tc.C, query string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %v)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}

// addNetNode inserts a net_node and returns its UUID.
func (s *modelStateSuite) addNetNode(c *tc.C) string {
	netNodeUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID)
	return netNodeUUID
}

// addMachine inserts a machine and returns its UUID.
func (s *modelStateSuite) addMachine(c *tc.C, name string) string {
	netNodeUUID := s.addNetNode(c)
	machineUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?,?,?,?)`,
		machineUUID, name, life.Alive, netNodeUUID)
	return machineUUID
}

// addMachineWithPlatform inserts a machine with platform (os + channel).
func (s *modelStateSuite) addMachineWithPlatform(c *tc.C, name, osName, channel string) string {
	netNodeUUID := s.addNetNode(c)
	machineUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?,?,?,?)`,
		machineUUID, name, life.Alive, netNodeUUID)

	// Get OS ID.
	var osID int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT id FROM os WHERE name = ?`, osName).Scan(&osID)
	})
	c.Assert(err, tc.ErrorIsNil)

	// Architecture amd64 = 0.
	s.runQuery(c, `INSERT INTO machine_platform (machine_uuid, os_id, channel, architecture_id) VALUES (?,?,?,?)`,
		machineUUID, osID, channel, 0)

	return machineUUID
}

// addMachinePlacement inserts a placement directive for a machine.
func (s *modelStateSuite) addMachinePlacement(c *tc.C, machineUUID, directive string) {
	s.runQuery(c, `INSERT INTO machine_placement (machine_uuid, scope_id, directive) VALUES (?,?,?)`,
		machineUUID, 0, directive)
}

// addConstraint inserts a constraint and links it to a machine. Returns the
// constraint UUID.
func (s *modelStateSuite) addConstraint(c *tc.C, machineUUID string, arch string, mem int64) string {
	consUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO "constraint" (uuid, arch, mem) VALUES (?,?,?)`,
		consUUID, arch, mem)
	s.runQuery(c, `INSERT INTO machine_constraint (machine_uuid, constraint_uuid) VALUES (?,?)`,
		machineUUID, consUUID)
	return consUUID
}

// addSpace inserts a space and returns its UUID.
func (s *modelStateSuite) addSpace(c *tc.C, name string) string {
	spaceUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO space (uuid, name) VALUES (?,?)`, spaceUUID, name)
	return spaceUUID
}

// addSubnet inserts a subnet in a space.
func (s *modelStateSuite) addSubnet(c *tc.C, spaceUUID, cidr string) string {
	subnetUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO subnet (uuid, cidr, space_uuid) VALUES (?,?,?)`,
		subnetUUID, cidr, spaceUUID)
	return subnetUUID
}

// addProviderSubnet inserts a provider subnet mapping.
func (s *modelStateSuite) addProviderSubnet(c *tc.C, subnetUUID, providerID string) {
	s.runQuery(c, `INSERT INTO provider_subnet (subnet_uuid, provider_id) VALUES (?,?)`,
		subnetUUID, providerID)
}

// addProviderSpace inserts a provider space mapping.
func (s *modelStateSuite) addProviderSpace(c *tc.C, spaceUUID, providerID string) {
	s.runQuery(c, `INSERT INTO provider_space (space_uuid, provider_id) VALUES (?,?)`,
		spaceUUID, providerID)
}

// addAvailabilityZone inserts an AZ and links it to a subnet.
func (s *modelStateSuite) addAvailabilityZone(c *tc.C, subnetUUID, azName string) {
	azUUID := uuid.MustNewUUID().String()
	// Insert AZ if not exists.
	s.runQuery(c, `INSERT OR IGNORE INTO availability_zone (uuid, name) VALUES (?,?)`, azUUID, azName)
	// Get actual UUID (might already exist).
	var actualUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT uuid FROM availability_zone WHERE name = ?`, azName).Scan(&actualUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	s.runQuery(c, `INSERT INTO availability_zone_subnet (availability_zone_uuid, subnet_uuid) VALUES (?,?)`,
		actualUUID, subnetUUID)
}

// addStoragePool inserts a storage pool with attributes.
func (s *modelStateSuite) addStoragePool(c *tc.C, name, provider string, attrs map[string]string) {
	poolUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO storage_pool (uuid, name, type) VALUES (?,?,?)`,
		poolUUID, name, provider)
	for k, v := range attrs {
		s.runQuery(c, `INSERT INTO storage_pool_attribute (storage_pool_uuid, "key", value) VALUES (?,?,?)`,
			poolUUID, k, v)
	}
}

// addCharm inserts a minimal charm. Returns the charm UUID.
func (s *modelStateSuite) addCharm(c *tc.C, name string) string {
	charmUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO charm (uuid, source_id, reference_name, revision) VALUES (?,1,?,0)`,
		charmUUID, name)
	return charmUUID
}

// addApplication inserts an application. Returns the app UUID.
func (s *modelStateSuite) addApplication(c *tc.C, name, charmUUID, spaceUUID string) string {
	appUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?,?,?,?,?)`,
		appUUID, name, life.Alive, charmUUID, spaceUUID)
	return appUUID
}

// addUnit inserts a unit assigned to the given machine's net_node. Returns
// the unit UUID.
func (s *modelStateSuite) addUnit(c *tc.C, name, appUUID, charmUUID, netNodeUUID string) string {
	unitUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid) VALUES (?,?,?,?,?,?)`,
		unitUUID, name, life.Alive, appUUID, netNodeUUID, charmUUID)
	return unitUUID
}

// addCharmRelation inserts a charm relation (endpoint). Returns the relation
// UUID.
func (s *modelStateSuite) addCharmRelation(c *tc.C, charmUUID, name string) string {
	relUUID := uuid.MustNewUUID().String()
	// role_id=0 (provider), scope_id=0 (global)
	s.runQuery(c, `INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, scope_id) VALUES (?,?,?,0,0)`,
		relUUID, charmUUID, name)
	return relUUID
}

// addApplicationEndpoint inserts an application endpoint binding.
func (s *modelStateSuite) addApplicationEndpoint(c *tc.C, appUUID, charmRelUUID string, spaceUUID *string) {
	epUUID := uuid.MustNewUUID().String()
	if spaceUUID != nil {
		s.runQuery(c, `INSERT INTO application_endpoint (uuid, application_uuid, space_uuid, charm_relation_uuid) VALUES (?,?,?,?)`,
			epUUID, appUUID, *spaceUUID, charmRelUUID)
	} else {
		s.runQuery(c, `INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid) VALUES (?,?,?)`,
			epUUID, appUUID, charmRelUUID)
	}
}

// addModelInfo inserts a minimal model row.
func (s *modelStateSuite) addModelInfo(c *tc.C, name, cloudType, cloudRegion string) {
	modelUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type, cloud_region) VALUES (?,?,?,?,?,?,?,?)`,
		modelUUID, "ctrl-uuid", name, "", "iaas", "aws", cloudType, cloudRegion)
}

// addApplicationController marks an application as a controller app.
func (s *modelStateSuite) addApplicationController(c *tc.C, appUUID string) {
	s.runQuery(c, `INSERT INTO application_controller (application_uuid) VALUES (?)`, appUUID)
}
