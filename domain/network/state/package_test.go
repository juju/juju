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

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	corenetwork "github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/charm"
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

func (s *linkLayerBaseSuite) addApplication(c *tc.C, charmUUID, spaceUUID string) string {
	appUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)`,
		appUUID, appUUID, life.Alive, charmUUID, spaceUUID)
	return appUUID
}

// addApplicationEndpoint inserts a new application endpoint into the
// database with the specified UUIDs. Returns the endpoint uuid.
func (s *linkLayerBaseSuite) addApplicationEndpoint(
	c *tc.C, applicationUUID coreapplication.ID, charmRelationUUID string, boundSpaceUUID string) string {
	applicationEndpointUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO application_endpoint (uuid, application_uuid, charm_relation_uuid,space_uuid)
VALUES (?, ?, ?, ?)
`, applicationEndpointUUID, applicationUUID, charmRelationUUID, nilZeroPtr(boundSpaceUUID))
	return applicationEndpointUUID
}

// addCharm inserts a new charm record into the database and returns its UUID as a string.
func (s *linkLayerBaseSuite) addCharm(c *tc.C) string {
	charmUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO charm (uuid, reference_name, create_time) VALUES (?, ?, ?)`,
		charmUUID, charmUUID, time.Now())
	return charmUUID
}

// addCharmRelation inserts a new charm relation into the database with the
// given UUID and attributes. Returns the relation UUID.
func (s *linkLayerBaseSuite) addCharmRelation(c *tc.C, charmUUID corecharm.ID, r charm.Relation) string {
	charmRelationUUID := uuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO charm_relation (uuid, charm_uuid, name, role_id, interface, optional, capacity, scope_id) 
VALUES (?, ?, ?,
       (SELECT id FROM charm_relation_role WHERE name = ?),
       ?, ?, ?,
       (SELECT id FROM charm_relation_scope WHERE name = ?))
`, charmRelationUUID, charmUUID, r.Name, r.Role, r.Interface, r.Optional, r.Limit, r.Scope)
	return charmRelationUUID
}

func (s *linkLayerBaseSuite) addNetNode(c *tc.C) string {
	netNodeUUID := uuid.MustNewUUID().String()
	s.query(c, "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
	return netNodeUUID
}

func (s *linkLayerBaseSuite) addMachine(c *tc.C, name, netNodeUUID string) machine.UUID {
	machineUUID := machinetesting.GenUUID(c)
	s.query(c, "INSERT INTO machine (uuid, net_node_uuid, name, life_id) VALUES (?, ?, ? ,?)",
		machineUUID.String(), netNodeUUID, name, 0)
	return machineUUID
}

func (s *linkLayerBaseSuite) addSpace(c *tc.C) string {
	spaceUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO space (uuid, name) VALUES (?, ?)`,
		spaceUUID, spaceUUID)
	return spaceUUID
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

// addLinkLayerDevice adds a link layer device to the database and returns its UUID.
func (s *linkLayerBaseSuite) addLinkLayerDevice(
	c *tc.C, netNodeUUID, name, macAddress string, deviceType corenetwork.LinkLayerDeviceType,
) string {
	deviceUUID := "device-" + name + "-uuid"

	mtu := int64(1500)

	s.query(c, `
INSERT INTO link_layer_device (
	uuid, net_node_uuid, name, mtu, mac_address, device_type_id, virtual_port_type_id, 
    is_auto_start, is_enabled, is_default_gateway, gateway_address, vlan_tag)
VALUES (?, ?, ?, ?, ?,
       (SELECT id FROM link_layer_device_type WHERE name = ?),
       (SELECT id FROM virtual_port_type WHERE name = ""),
       ?, ?, ?, ?, ?)
	`, deviceUUID, netNodeUUID, name, mtu, macAddress, deviceType, true,
		true, false, nil, 0)

	return deviceUUID
}

func (s *linkLayerBaseSuite) addDNSDomains(c *tc.C, deviceUUID string, dnsDomains ...string) {
	for _, dnsDomain := range dnsDomains {
		s.query(c, `
INSERT INTO link_layer_device_dns_domain (device_uuid, search_domain) 
VALUES (?, ?)`, deviceUUID, dnsDomain)
	}
}

func (s *linkLayerBaseSuite) addDNSAddresses(c *tc.C, deviceUUID string, dnsAddresses ...string) {
	for _, dnsAddress := range dnsAddresses {
		s.query(c, `
INSERT INTO link_layer_device_dns_address (device_uuid, dns_address) 
VALUES (?, ?)`, deviceUUID, dnsAddress)
	}
}

func (s *linkLayerBaseSuite) addSubnet(c *tc.C, cidr string, spaceUUID string) string {
	uuid := "subnet-" + cidr + "-uuid"
	s.query(c, `
INSERT INTO subnet (uuid, cidr, space_uuid) 
VALUES (?, ?, ?)`, uuid, cidr, spaceUUID)
	return uuid
}

// addProviderLinkLayerDevice adds a provider link layer device to the database.
func (s *linkLayerBaseSuite) addProviderLinkLayerDevice(
	c *tc.C, providerID, deviceUUID string,
) {
	s.query(c, `
INSERT INTO provider_link_layer_device (provider_id, device_uuid)
VALUES (?, ?)
	`, providerID, deviceUUID)
}

// addIPAddress adds an IP address to the database and returns its UUID.
func (s *linkLayerBaseSuite) addIPAddress(
	c *tc.C, deviceUUID, netNodeUUID, addressValue string,
) string {
	addressUUID := "address-" + addressValue + "-uuid"

	s.query(c, `
INSERT INTO ip_address (
	uuid, device_uuid, address_value, net_node_uuid, subnet_uuid, type_id, 
	config_type_id, origin_id, scope_id, is_secondary, is_shadow
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, addressUUID, deviceUUID, addressValue, netNodeUUID, nil, 0, 4, 0, 0,
		false, false)

	return addressUUID
}

// addIPAddressWithSubnet adds an IP address to the database and returns its UUID.
func (s *linkLayerBaseSuite) addIPAddressWithSubnet(c *tc.C, deviceUUID, netNodeUUID,
	subnetUUID, addressValue string) string {
	addressUUID := "address-" + addressValue + "-uuid"

	s.query(c, `
		INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, subnet_uuid, type_id, config_type_id, origin_id, scope_id, is_secondary, is_shadow)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, addressUUID, deviceUUID, addressValue, netNodeUUID, subnetUUID, 0, 4, 1, 0,
		false, false)

	return addressUUID
}

// addIPAddressWithSubnet adds an IP address to the database and returns its UUID.
func (s *linkLayerBaseSuite) addIPAddressWithSubnetAndScope(c *tc.C, deviceUUID, netNodeUUID,
	subnetUUID, addressValue string, scope corenetwork.Scope) string {
	addressUUID := "address-" + addressValue + "-uuid"

	s.query(c, `
		INSERT INTO ip_address (uuid, device_uuid, address_value, net_node_uuid, subnet_uuid, type_id, config_type_id, origin_id, scope_id)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, scope.id
		FROM ip_address_scope AS scope
		WHERE scope.name = ?
	`, addressUUID, deviceUUID, addressValue, netNodeUUID, subnetUUID, 0, 4, 1, string(scope))

	return addressUUID
}

// addProviderIPAddress adds a provider IP address to the database.
func (s *mergeLinkLayerSuite) addProviderIPAddress(
	c *tc.C, addressUUID, providerID string,
) {
	s.query(c, `
INSERT INTO provider_ip_address (provider_id, address_uuid)
VALUES (?, ?)
	`, providerID, addressUUID)

	s.query(c, `
UPDATE ip_address
SET origin_id = (SELECT id FROM ip_address_origin WHERE name = 'provider')
WHERE uuid = ?
	`, addressUUID)
}

func (s *linkLayerBaseSuite) setLinkLayerDeviceParent(c *tc.C, childUUID string, parentUUID string) {
	s.query(c, `INSERT INTO link_layer_device_parent (parent_uuid, device_uuid) VALUES (?, ?)`, parentUUID, childUUID)
}

// addUnit inserts a new unit record into the database and returns the generated unit UUID.
func (s *linkLayerBaseSuite) addUnit(c *tc.C, appUUID, charmUUID, nodeUUID string) coreunit.UUID {
	unitUUID := unittesting.GenUnitUUID(c)
	s.query(c, `INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)`,
		unitUUID, unitUUID, life.Alive, appUUID, charmUUID, nodeUUID)
	return unitUUID
}

// addK8sService inserts a new Kubernetes service into the database with the associated node, application, and
// provider ID.
func (s *linkLayerBaseSuite) addK8sService(c *tc.C, nodeUUID, appUUID string) {
	svcUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO k8s_service (uuid, net_node_uuid, application_uuid, provider_id) VALUES (?, ?, ?, ?)`,
		svcUUID, nodeUUID, appUUID, "provider-id")
}
