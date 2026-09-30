// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/uuid"
)

type controllerNetworkSuite struct{ linkLayerBaseSuite }

func TestControllerNetworkSuite(t *testing.T) { tc.Run(t, &controllerNetworkSuite{}) }

func (s *controllerNetworkSuite) TestPodAddressesExcludeService(c *tc.C) {
	node, app := s.controllerUnit(c, model.CAAS)
	device := s.addLinkLayerDevice(c, node, "eth0", "", network.EthernetDevice)
	s.query(c, `INSERT INTO ip_address (uuid, net_node_uuid, device_uuid, address_value, type_id, config_type_id, origin_id, scope_id)
VALUES ('ipv4', ?, ?, '10.0.0.2/24', 0, 4, 1, 3), ('ipv6', ?, ?, '2001:db8::2/64', 1, 4, 1, 3)`, node, device, node, device)
	s.addFQDNAddress(c, node, "controller-0.controller-service-endpoints.controller-test.svc.cluster.local")
	serviceNode := s.addNetNode(c)
	s.addK8sService(c, serviceNode, app)
	s.addFQDNAddress(c, serviceNode, "shared.example.com")

	facts, err := s.state.GetControllerUnitNetwork(c.Context(), "controller/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(facts.ModelType, tc.Equals, model.CAAS)
	c.Assert(facts.Addresses, tc.HasLen, 3)
	values := make(map[string]network.AddressType)
	for _, address := range facts.Addresses {
		values[address.Value] = address.Type
		if address.Type != network.HostName {
			c.Check(address.Scope, tc.Equals, network.ScopeMachineLocal)
		}
	}
	c.Check(values, tc.DeepEquals, map[string]network.AddressType{
		"10.0.0.2":    network.IPv4Address,
		"2001:db8::2": network.IPv6Address,
		"controller-0.controller-service-endpoints.controller-test.svc.cluster.local": network.HostName,
	})
}

func (s *controllerNetworkSuite) TestMachineAddresses(c *tc.C) {
	node, _ := s.controllerUnit(c, model.IAAS)
	s.addMachine(c, "0", node)
	space := s.addSpace(c)
	s.query(c, `INSERT INTO subnet (uuid, cidr, space_uuid) VALUES ('subnet', '10.0.0.0/24', ?)`, space)
	device := s.addLinkLayerDevice(c, node, "eth0", "", network.EthernetDevice)
	s.addIPAddressWithSubnetAndScope(c, device, node, "subnet", "10.0.0.2/24", network.ScopeCloudLocal)
	facts, err := s.state.GetControllerUnitNetwork(c.Context(), "controller/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(facts.Addresses, tc.HasLen, 1)
	c.Check(facts.ModelType, tc.Equals, model.IAAS)
	c.Check(facts.Addresses[0].SpaceID, tc.Equals, network.SpaceUUID(space))
	c.Check(facts.Addresses[0].Value, tc.Equals, "10.0.0.2")
}

func (s *controllerNetworkSuite) TestLifecycle(c *tc.C) {
	s.controllerUnit(c, model.IAAS)
	facts, err := s.state.GetControllerUnitNetwork(c.Context(), "controller/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(facts.Addresses, tc.HasLen, 0)
	s.query(c, `UPDATE unit SET life_id = 1`)
	s.query(c, `UPDATE application SET life_id = 1`)
	_, err = s.state.GetControllerUnitNetwork(c.Context(), "controller/0")
	c.Assert(err, tc.ErrorIsNil)
	s.query(c, `UPDATE unit SET life_id = 2`)
	_, err = s.state.GetControllerUnitNetwork(c.Context(), "controller/0")
	c.Check(err, tc.ErrorIs, applicationerrors.UnitIsDead)
	s.query(c, `DELETE FROM unit`)
	_, err = s.state.GetControllerUnitNetwork(c.Context(), "controller/0")
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *controllerNetworkSuite) TestDeadApplication(c *tc.C) {
	s.controllerUnit(c, model.CAAS)
	s.query(c, `UPDATE application SET life_id = 2`)
	_, err := s.state.GetControllerUnitNetwork(c.Context(), "controller/0")
	c.Check(err, tc.ErrorIs, applicationerrors.UnitIsDead)
}

func (s *controllerNetworkSuite) TestControllerApplicationRequired(c *tc.C) {
	s.controllerUnit(c, model.IAAS)
	s.query(c, `DELETE FROM application_controller`)
	_, err := s.state.GetControllerUnitNetwork(c.Context(), "controller/0")
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *controllerNetworkSuite) TestControllerModelRequired(c *tc.C) {
	s.insertModel(c, model.IAAS, false)
	_, err := s.state.GetControllerUnitNetwork(c.Context(), "controller/0")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *controllerNetworkSuite) TestUninitialisedModelIsNotEmptySnapshot(c *tc.C) {
	_, err := s.state.GetControllerUnitNetwork(c.Context(), "controller/0")
	c.Check(err, tc.NotNil)
}

func (s *controllerNetworkSuite) TestReassociatedNode(c *tc.C) {
	old, _ := s.controllerUnit(c, model.CAAS)
	s.addFQDNAddress(c, old, "old.example.com")
	replacement := s.addNetNode(c)
	s.addFQDNAddress(c, replacement, "new.example.com")
	s.query(c, `UPDATE unit SET net_node_uuid = ?`, replacement)
	facts, err := s.state.GetControllerUnitNetwork(c.Context(), "controller/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(facts.Addresses, tc.HasLen, 1)
	c.Check(facts.Addresses[0].Value, tc.Equals, "new.example.com")
}

func (s *controllerNetworkSuite) controllerUnit(c *tc.C, modelType model.ModelType) (string, string) {
	s.insertModel(c, modelType, true)
	node := s.addNetNode(c)
	charm := s.addCharm(c)
	app := s.addApplicationWithName(c, charm, network.AlphaSpaceId.String(), "controller")
	s.query(c, `INSERT INTO application_controller (application_uuid) VALUES (?)`, app)
	id := s.addUnit(c, app, charm, node)
	s.query(c, `UPDATE unit SET name = 'controller/0' WHERE uuid = ?`, id)
	return node, app
}

func (s *controllerNetworkSuite) insertModel(c *tc.C, modelType model.ModelType, controller bool) {
	s.query(c, `INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type, is_controller_model)
VALUES (?, ?, 'controller', 'admin', ?, 'test', 'test', ?)`, uuid.MustNewUUID().String(), uuid.MustNewUUID().String(), modelType, controller)
}
