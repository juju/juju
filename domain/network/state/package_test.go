// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	machinetesting "github.com/juju/juju/core/machine/testing"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/ipaddress"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type linkLayerBaseSuite struct {
	schematesting.ModelSuite
	state *State
}

func (s *linkLayerBaseSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	c.Cleanup(func() {
		s.state = nil
	})
}

// txn executes a transactional function within a database context,
// ensuring proper error handling and assertion.
func (s *linkLayerBaseSuite) txn(c *tc.C, fn func(ctx context.Context, tx *sqlair.TX) error) error {
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)
	return db.Txn(c.Context(), fn)
}

// query executes a given SQL query with optional arguments within a
// transactional context using the test database.
func (s *linkLayerBaseSuite) query(c *tc.C, query string, args ...any) {
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

func (s *linkLayerBaseSuite) addNetNode(c *tc.C) string {
	netNodeUUID := uuid.MustNewUUID().String()
	s.query(c, "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
	return netNodeUUID
}

func (s *linkLayerBaseSuite) addLinkLayerDevice(c *tc.C, netNodeUUID string) string {
	lldUUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO link_layer_device (uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		lldUUUID, netNodeUUID, lldUUUID, 1500, "00:11:22:33:44:55", 0, 0)
	return lldUUUID
}

func (s *linkLayerBaseSuite) addMachine(c *tc.C, name, netNodeUUID string) {
	machineUUID := machinetesting.GenUUID(c).String()
	s.query(c, "INSERT INTO machine (uuid, net_node_uuid, name, life_id) VALUES (?, ?, ? ,?)",
		machineUUID, netNodeUUID, name, 0)
}

func (s *linkLayerBaseSuite) addSpace(c *tc.C) string {
	spaceUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO space (uuid, name) VALUES (?, ?)`,
		spaceUUID, spaceUUID)
	return spaceUUID
}

func (s *linkLayerBaseSuite) addsubnet(c *tc.C, spaceUUID string) (string, string) {
	subnetUUID := uuid.MustNewUUID().String()
	cidr := "10.0.0.0/24"
	s.query(c, `INSERT INTO subnet (uuid, cidr, space_uuid) VALUES (?, ?, ?)`,
		subnetUUID, cidr, spaceUUID)
	return subnetUUID, cidr
}

func (s *linkLayerBaseSuite) addIPAddress(c *tc.C, nodeUUID, deviceUUID, subnetUUID string, scope corenetwork.Scope, origin corenetwork.Origin) string {
	ipAddrUUID := uuid.MustNewUUID().String()
	scopeID := int(ipaddress.MarshallScope(scope))
	originID := int(ipaddress.MarshallOrigin(origin))
	addr := "10.0.0.1"
	s.query(c, `INSERT INTO ip_address (uuid, net_node_uuid, device_uuid, address_value, type_id, scope_id, origin_id, config_type_id, subnet_uuid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ipAddrUUID, nodeUUID, deviceUUID, addr+"/24", 0, scopeID, originID, 1, subnetUUID)
	return addr
}

func (s *linkLayerBaseSuite) addCharm(c *tc.C) string {
	charmUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO charm (uuid, reference_name, create_time) VALUES (?, ?, ?)`,
		charmUUID, charmUUID, time.Now())
	return charmUUID
}

func (s *linkLayerBaseSuite) addApplication(c *tc.C, charmUUID, spaceUUID string) string {
	appUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)`,
		appUUID, appUUID, life.Alive, charmUUID, spaceUUID)
	return appUUID
}

func (s *linkLayerBaseSuite) addUnit(c *tc.C, appUUID, charmUUID, nodeUUID string) unit.UUID {
	unitUUID := testing.GenUnitUUID(c)
	s.query(c, `INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)`,
		unitUUID, unitUUID, life.Alive, appUUID, charmUUID, nodeUUID)
	return unitUUID
}

func (s *linkLayerBaseSuite) addk8sService(c *tc.C, nodeUUID, appUUID string) {
	svcUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO k8s_service (uuid, net_node_uuid, application_uuid, provider_id) VALUES (?, ?, ?, ?)`,
		svcUUID, nodeUUID, appUUID, "provider-id")
}

// checkRowCount checks that the given table has the expected number of rows.
func (s *linkLayerBaseSuite) checkRowCount(c *tc.C, table string, expected int) {
	obtained := -1
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		return tx.QueryRowContext(ctx, query).Scan(&obtained)
	})
	c.Assert(err, tc.IsNil, tc.Commentf("counting rows in table %q", table))
	c.Check(obtained, tc.Equals, expected, tc.Commentf("count of %q rows", table))
}
