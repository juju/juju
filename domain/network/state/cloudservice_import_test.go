// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
)

type k8sServiceImportSuite struct {
	linkLayerBaseSuite
}

func TestK8sServiceImportSuite(t *testing.T) {
	tc.Run(t, &k8sServiceImportSuite{})
}

// TestCreateK8sServices tests the happy path for cloud service creation and deletion.
// It verifies that multiple cloud services are correctly inserted into the database
// and then properly deleted.
func (s *k8sServiceImportSuite) TestCreateK8sServices(c *tc.C) {
	// Arrange: Set up application for the cloud services
	charmUUID := s.addCharm(c)
	spaceUUID := s.addSpace(c)
	appUUID1 := s.addApplicationWithName(c, charmUUID, spaceUUID, "super-app-1")
	appUUID2 := s.addApplicationWithName(c, charmUUID, spaceUUID, "super-app-2")

	args := []internal.ImportK8sService{
		{
			UUID:            "service-uuid-1",
			NetNodeUUID:     "net-node-uuid-1",
			ApplicationName: "super-app-1",
			ProviderID:      "provider-id-1",
		},
		{
			UUID:            "service-uuid-2",
			NetNodeUUID:     "net-node-uuid-2",
			ApplicationName: "super-app-2",
			ProviderID:      "provider-id-2",
		},
	}

	// Act
	err := s.state.CreateK8sServices(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.fetchNetNodeUUIDs(c), tc.SameContents, []string{"net-node-uuid-1", "net-node-uuid-2"})
	type k8sService struct {
		UUID            string
		NetNodeUUID     string
		ApplicationUUID string
		ProviderID      string
	}
	var k8sServices []k8sService
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		k8sServices = nil

		rows, err := tx.QueryContext(ctx, `SELECT uuid, net_node_uuid, application_uuid, provider_id FROM k8s_service`)
		if err != nil {
			return errors.Errorf("querying net nodes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var get k8sService
			err := rows.Scan(&get.UUID, &get.NetNodeUUID, &get.ApplicationUUID, &get.ProviderID)
			if err != nil {
				return errors.Errorf("scanning net nodes: %w", err)
			}
			k8sServices = append(k8sServices, get)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(k8sServices, tc.SameContents, []k8sService{
		{
			UUID:            "service-uuid-1",
			NetNodeUUID:     "net-node-uuid-1",
			ApplicationUUID: appUUID1,
			ProviderID:      "provider-id-1",
		},
		{
			UUID:            "service-uuid-2",
			NetNodeUUID:     "net-node-uuid-2",
			ApplicationUUID: appUUID2,
			ProviderID:      "provider-id-2",
		}})
}

func (s *k8sServiceImportSuite) TestCreateK8sServicesDuplicateApplication(c *tc.C) {
	charmUUID := s.addCharm(c)
	spaceUUID := s.addSpace(c)
	s.addApplicationWithName(c, charmUUID, spaceUUID, "app")
	args := []internal.ImportK8sService{
		{
			UUID:            "service-uuid-1",
			NetNodeUUID:     "net-node-uuid-1",
			ApplicationName: "app",
			ProviderID:      "provider-id-1",
		},
		{
			UUID:            "service-uuid-2",
			NetNodeUUID:     "net-node-uuid-2",
			ApplicationName: "app",
			ProviderID:      "provider-id-2",
		},
	}

	err := s.state.CreateK8sServices(c.Context(), args)
	c.Check(err, tc.ErrorMatches, `inserting services: .*UNIQUE constraint failed: k8s_service.application_uuid.*`)

	// Reject the entire batch, including the first Service and both net nodes.
	var count int
	err = s.DB().QueryRowContext(c.Context(), "SELECT COUNT(*) FROM k8s_service AS ks").Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
	c.Check(s.fetchNetNodeUUIDs(c), tc.HasLen, 0)
}

func (s *k8sServiceImportSuite) fetchNetNodeUUIDs(c *tc.C) []string {
	var nodes []string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		nodes = nil

		rows, err := tx.QueryContext(ctx, `SELECT uuid FROM net_node`)
		if err != nil {
			return errors.Errorf("querying net nodes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var uuid string
			err := rows.Scan(&uuid)
			if err != nil {
				return errors.Errorf("scanning net nodes: %w", err)
			}
			nodes = append(nodes, uuid)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Assert) failed to check DB: %v"))
	return nodes
}

// TestImportNetNodeAddresses covers the Kubernetes import path: addresses are
// written against their net node with a NULL device reference, and no link
// layer devices are created.
func (s *k8sServiceImportSuite) TestImportNetNodeAddresses(c *tc.C) {
	// Arrange
	nodeUUID1 := s.addNetNode(c)
	nodeUUID2 := s.addNetNode(c)
	spaceUUID := s.addSpace(c)
	subnetUUID := s.addSubnet(c, "10.0.0.0/24", spaceUUID)

	input := []internal.ImportNetNodeAddresses{
		{
			NetNodeUUID: nodeUUID1,
			Addresses: []internal.ImportIPAddress{
				{
					UUID:         "address-uuid-1",
					Type:         corenetwork.IPv4Address,
					Scope:        corenetwork.ScopePublic,
					ConfigType:   corenetwork.ConfigStatic,
					Origin:       corenetwork.OriginProvider,
					ProviderID:   new("provider-ip-1"),
					AddressValue: "10.0.0.1/24",
					SubnetUUID:   subnetUUID,
				},
				{
					UUID:         "address-uuid-2",
					Type:         corenetwork.IPv6Address,
					Scope:        corenetwork.ScopeCloudLocal,
					ConfigType:   corenetwork.ConfigDHCP,
					Origin:       corenetwork.OriginProvider,
					AddressValue: "fd42::1/64",
				},
			},
		},
		{
			NetNodeUUID: nodeUUID2,
			Addresses: []internal.ImportIPAddress{
				{
					UUID:         "address-uuid-3",
					Type:         corenetwork.IPv4Address,
					Scope:        corenetwork.ScopePublic,
					ConfigType:   corenetwork.ConfigStatic,
					Origin:       corenetwork.OriginProvider,
					ProviderID:   new("provider-ip-3"),
					AddressValue: "10.0.0.3/24",
					SubnetUUID:   subnetUUID,
				},
			},
		},
	}

	// Act
	err := s.state.ImportNetNodeAddresses(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	s.checkRowCount(c, "ip_address", 3)
	s.checkRowCount(c, "provider_ip_address", 2)
	// No placeholder link layer devices are created for the addresses.
	s.checkRowCount(c, "link_layer_device", 0)

	type ipAddress struct {
		UUID         string
		NetNodeUUID  string
		DeviceUUID   *string
		AddressValue string
		SubnetUUID   *string
	}
	var addresses []ipAddress
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT uuid, net_node_uuid, device_uuid, address_value, subnet_uuid FROM ip_address`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var addr ipAddress
			if err := rows.Scan(&addr.UUID, &addr.NetNodeUUID, &addr.DeviceUUID,
				&addr.AddressValue, &addr.SubnetUUID); err != nil {
				return err
			}
			addresses = append(addresses, addr)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresses, tc.SameContents, []ipAddress{
		{
			UUID:         "address-uuid-1",
			NetNodeUUID:  nodeUUID1,
			DeviceUUID:   nil,
			AddressValue: "10.0.0.1/24",
			SubnetUUID:   &subnetUUID,
		},
		{
			UUID:         "address-uuid-2",
			NetNodeUUID:  nodeUUID1,
			DeviceUUID:   nil,
			AddressValue: "fd42::1/64",
			SubnetUUID:   nil,
		},
		{
			UUID:         "address-uuid-3",
			NetNodeUUID:  nodeUUID2,
			DeviceUUID:   nil,
			AddressValue: "10.0.0.3/24",
			SubnetUUID:   &subnetUUID,
		},
	})

	type providerAddress struct {
		ProviderID  string
		AddressUUID string
	}
	var providerAddresses []providerAddress
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT provider_id, address_uuid FROM provider_ip_address`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var addr providerAddress
			if err := rows.Scan(&addr.ProviderID, &addr.AddressUUID); err != nil {
				return err
			}
			providerAddresses = append(providerAddresses, addr)
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(providerAddresses, tc.SameContents, []providerAddress{
		{ProviderID: "provider-ip-1", AddressUUID: "address-uuid-1"},
		{ProviderID: "provider-ip-3", AddressUUID: "address-uuid-3"},
	})
}

// TestImportNetNodeAddressesLookupErrors verifies that addresses with values
// that cannot be resolved against the network lookup tables are rejected.
func (s *k8sServiceImportSuite) TestImportNetNodeAddressesLookupErrors(c *tc.C) {
	validAddress := internal.ImportIPAddress{
		UUID:         "address-uuid",
		Type:         corenetwork.IPv4Address,
		Scope:        corenetwork.ScopePublic,
		ConfigType:   corenetwork.ConfigStatic,
		Origin:       corenetwork.OriginProvider,
		AddressValue: "10.0.0.1/24",
	}

	testCases := []struct {
		summary string
		mutate  func(*internal.ImportIPAddress)
		expect  string
	}{
		{
			summary: "unknown address type",
			mutate:  func(a *internal.ImportIPAddress) { a.Type = "bogus" },
			expect:  `unknown address type "bogus"`,
		},
		{
			summary: "unknown address config type",
			mutate:  func(a *internal.ImportIPAddress) { a.ConfigType = "bogus" },
			expect:  `unknown address config type "bogus"`,
		},
		{
			summary: "unknown address origin",
			mutate:  func(a *internal.ImportIPAddress) { a.Origin = "bogus" },
			expect:  `unknown address origin "bogus"`,
		},
		{
			summary: "unknown address scope",
			mutate:  func(a *internal.ImportIPAddress) { a.Scope = "bogus" },
			expect:  `unknown address scope "bogus"`,
		},
	}

	for _, test := range testCases {
		c.Logf("test: %s", test.summary)
		addr := validAddress
		test.mutate(&addr)

		nodeUUID := s.addNetNode(c)
		err := s.state.ImportNetNodeAddresses(c.Context(), []internal.ImportNetNodeAddresses{
			{NetNodeUUID: nodeUUID, Addresses: []internal.ImportIPAddress{addr}},
		})
		c.Check(err, tc.ErrorMatches, test.expect)
	}

	// Nothing was persisted.
	s.checkRowCount(c, "ip_address", 0)
}
