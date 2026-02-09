// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/description/v11"
	"github.com/juju/tc"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	machineservice "github.com/juju/juju/domain/machine/service"
	machinestate "github.com/juju/juju/domain/machine/state"
	domainnetwork "github.com/juju/juju/domain/network"
	networkmodelmigration "github.com/juju/juju/domain/network/modelmigration"
	"github.com/juju/juju/domain/network/service"
	"github.com/juju/juju/domain/network/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type importSuite struct {
	schematesting.ModelSuite

	coordinator *modelmigration.Coordinator
	scope       modelmigration.Scope
	svc         *service.Service
}

func TestImportSuite(t *stdtesting.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.coordinator = modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	s.scope = modelmigration.NewScope(nil, s.TxnRunnerFactory(), nil, model.UUID(s.ModelUUID()))

	s.svc = service.NewService(
		state.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c)),
		loggertesting.WrapCheckLog(c),
	)

	c.Cleanup(func() {
		s.coordinator = nil
		s.svc = nil
		s.scope = modelmigration.Scope{}
	})
}

func (s *importSuite) TestImportSpaces36(c *tc.C) {
	s.setModel(c, "ec2", model.IAAS.String())

	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	// No uuids are exported in 36, only ids
	desc.AddSpace(description.SpaceArgs{
		Id:         "1",
		Name:       "space-one",
		ProviderID: "space-provider-id-1",
	})
	desc.AddSpace(description.SpaceArgs{
		Id:         "2",
		Name:       "space-two",
		ProviderID: "space-provider-id-2",
	})

	spaces, err := s.svc.GetAllSpaces(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spaces, tc.HasLen, 1)
	c.Assert(spaces[0].Name, tc.Equals, network.AlphaSpaceName)

	networkmodelmigration.RegisterImportSubnets(s.coordinator, loggertesting.WrapCheckLog(c))
	err = s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	spaces, err = s.svc.GetAllSpaces(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spaces, tc.HasLen, 3)

	// space uuids are generated, don't bother checking them
	space1, err := s.svc.SpaceByName(c.Context(), "space-one")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(space1.ProviderId, tc.Equals, network.Id("space-provider-id-1"))
	c.Check(space1.Subnets, tc.HasLen, 0)

	space2, err := s.svc.SpaceByName(c.Context(), "space-two")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(space2.ProviderId, tc.Equals, network.Id("space-provider-id-2"))
	c.Check(space2.Subnets, tc.HasLen, 0)
}

func (s *importSuite) TestImportSpaces40(c *tc.C) {
	s.setModel(c, "ec2", model.IAAS.String())

	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	space1UUID := tc.Must(c, network.NewSpaceUUID)
	space2UUID := tc.Must(c, network.NewSpaceUUID)

	desc.AddSpace(description.SpaceArgs{
		UUID:       space1UUID.String(),
		Name:       "space-one",
		ProviderID: "space-provider-id-1",
	})
	desc.AddSpace(description.SpaceArgs{
		UUID:       space2UUID.String(),
		Name:       "space-two",
		ProviderID: "space-provider-id-2",
	})

	spaces, err := s.svc.GetAllSpaces(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spaces, tc.HasLen, 1)
	c.Assert(spaces[0].Name, tc.Equals, network.AlphaSpaceName)

	networkmodelmigration.RegisterImportSubnets(s.coordinator, loggertesting.WrapCheckLog(c))
	err = s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	spaces, err = s.svc.GetAllSpaces(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spaces, tc.HasLen, 3)

	space1, err := s.svc.Space(c.Context(), space1UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(space1.Name, tc.Equals, network.SpaceName("space-one"))
	c.Check(space1.ProviderId, tc.Equals, network.Id("space-provider-id-1"))
	c.Check(space1.Subnets, tc.HasLen, 0)

	space2, err := s.svc.Space(c.Context(), space2UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(space2.Name, tc.Equals, network.SpaceName("space-two"))
	c.Check(space2.ProviderId, tc.Equals, network.Id("space-provider-id-2"))
	c.Check(space2.Subnets, tc.HasLen, 0)
}

func (s *importSuite) TestImportSpacesWithSubnets(c *tc.C) {
	s.setModel(c, "ec2", model.IAAS.String())

	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	space1UUID := tc.Must(c, network.NewSpaceUUID)
	space2UUID := tc.Must(c, network.NewSpaceUUID)

	subnet1UUID := tc.Must(c, domainnetwork.NewSubnetUUID)
	subnet2UUID := tc.Must(c, domainnetwork.NewSubnetUUID)
	subnet3UUID := tc.Must(c, domainnetwork.NewSubnetUUID)

	desc.AddSpace(description.SpaceArgs{
		UUID:       space1UUID.String(),
		Name:       "space-one",
		ProviderID: "space-provider-id-1",
	})
	desc.AddSpace(description.SpaceArgs{
		UUID:       space2UUID.String(),
		Name:       "space-two",
		ProviderID: "space-provider-id-2",
	})

	desc.AddSubnet(description.SubnetArgs{
		UUID:              subnet1UUID.String(),
		CIDR:              "192.0.2.0/24",
		ProviderId:        "subnet-provider-id",
		ProviderNetworkId: "subnet-provider-network-id",
		VLANTag:           42,
		AvailabilityZones: []string{"az1"},
		SpaceID:           space1UUID.String(),
		SpaceName:         "space-one",
		ProviderSpaceId:   "space-provider-id",
	})
	desc.AddSubnet(description.SubnetArgs{
		UUID:              subnet2UUID.String(),
		CIDR:              "192.0.3.0/24",
		ProviderId:        "subnet2-provider-id",
		ProviderNetworkId: "subnet2-provider-network-id",
		VLANTag:           84,
		AvailabilityZones: []string{"az2"},
		SpaceID:           space2UUID.String(),
		SpaceName:         "space-two",
		ProviderSpaceId:   "space-provider-id-2",
	})
	desc.AddSubnet(description.SubnetArgs{
		UUID:              subnet3UUID.String(),
		CIDR:              "192.0.4.0/24",
		ProviderId:        "subnet3-provider-id",
		ProviderNetworkId: "subnet3-provider-network-id",
		VLANTag:           168,
		AvailabilityZones: []string{"az3"},
	})

	networkmodelmigration.RegisterImportSubnets(s.coordinator, loggertesting.WrapCheckLog(c))
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	subnet1Info := network.SubnetInfo{
		ID:                network.Id(subnet1UUID.String()),
		CIDR:              "192.0.2.0/24",
		ProviderId:        network.Id("subnet-provider-id"),
		ProviderNetworkId: network.Id("subnet-provider-network-id"),
		ProviderSpaceId:   network.Id("space-provider-id-1"),
		VLANTag:           42,
		AvailabilityZones: []string{"az1"},
		SpaceID:           space1UUID,
		SpaceName:         network.SpaceName("space-one"),
	}
	subnet2Info := network.SubnetInfo{
		ID:                network.Id(subnet2UUID.String()),
		CIDR:              "192.0.3.0/24",
		ProviderId:        network.Id("subnet2-provider-id"),
		ProviderNetworkId: network.Id("subnet2-provider-network-id"),
		ProviderSpaceId:   network.Id("space-provider-id-2"),
		VLANTag:           84,
		AvailabilityZones: []string{"az2"},
		SpaceID:           space2UUID,
		SpaceName:         network.SpaceName("space-two"),
	}
	subnet3Info := network.SubnetInfo{
		ID:                network.Id(subnet3UUID.String()),
		CIDR:              "192.0.4.0/24",
		ProviderId:        network.Id("subnet3-provider-id"),
		ProviderNetworkId: network.Id("subnet3-provider-network-id"),
		VLANTag:           168,
		AvailabilityZones: []string{"az3"},
		SpaceID:           network.AlphaSpaceId,
		SpaceName:         network.AlphaSpaceName,
	}

	space1, err := s.svc.Space(c.Context(), space1UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(space1.Name, tc.Equals, network.SpaceName("space-one"))
	c.Check(space1.ProviderId, tc.Equals, network.Id("space-provider-id-1"))
	c.Check(space1.Subnets, tc.HasLen, 1)
	c.Check(space1.Subnets[0], tc.DeepEquals, subnet1Info)

	space2, err := s.svc.Space(c.Context(), space2UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(space2.Name, tc.Equals, network.SpaceName("space-two"))
	c.Check(space2.ProviderId, tc.Equals, network.Id("space-provider-id-2"))
	c.Check(space2.Subnets, tc.HasLen, 1)
	c.Check(space2.Subnets[0], tc.DeepEquals, subnet2Info)

	subnets, err := s.svc.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, 3)

	subnet1, err := s.svc.Subnet(c.Context(), subnet1UUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnet1, tc.DeepEquals, &subnet1Info)

	subnet2, err := s.svc.Subnet(c.Context(), subnet2UUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnet2, tc.DeepEquals, &subnet2Info)

	subnet3, err := s.svc.Subnet(c.Context(), subnet3UUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnet3, tc.DeepEquals, &subnet3Info)
}
func (s *importSuite) TestImportSpacesWithSubnetsLXD(c *tc.C) {
	s.setModel(c, "lxd", model.IAAS.String())

	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	space1UUID := tc.Must(c, network.NewSpaceUUID)
	space2UUID := tc.Must(c, network.NewSpaceUUID)

	subnet1UUID := tc.Must(c, domainnetwork.NewSubnetUUID)
	subnet2UUID := tc.Must(c, domainnetwork.NewSubnetUUID)

	desc.AddSpace(description.SpaceArgs{
		UUID:       space1UUID.String(),
		Name:       "space-one",
		ProviderID: "space-provider-id-1",
	})
	desc.AddSpace(description.SpaceArgs{
		UUID:       space2UUID.String(),
		Name:       "space-two",
		ProviderID: "space-provider-id-2",
	})

	desc.AddSubnet(description.SubnetArgs{
		UUID:              subnet1UUID.String(),
		CIDR:              "192.0.2.0/24",
		ProviderNetworkId: "net-docker0",
		ProviderId:        "subnet-docker0-192.0.2.0/24",
		VLANTag:           42,
		AvailabilityZones: []string{"az1"},
		SpaceID:           space1UUID.String(),
		SpaceName:         "space-one",
		ProviderSpaceId:   "space-provider-id",
	})
	desc.AddSubnet(description.SubnetArgs{
		UUID:              subnet2UUID.String(),
		CIDR:              "192.0.3.0/24",
		ProviderNetworkId: "net-lxdbr0",
		ProviderId:        "subnet-lxdbr0-192.0.3.0/24",
		VLANTag:           84,
		AvailabilityZones: []string{"az2"},
		SpaceID:           space2UUID.String(),
		SpaceName:         "space-two",
		ProviderSpaceId:   "space-provider-id-2",
	})

	networkmodelmigration.RegisterImportSubnets(s.coordinator, loggertesting.WrapCheckLog(c))
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	subnet1Info := network.SubnetInfo{
		ID:                network.Id(subnet1UUID.String()),
		CIDR:              "192.0.2.0/24",
		ProviderNetworkId: "docker0",
		ProviderSpaceId:   network.Id("space-provider-id-1"),
		VLANTag:           42,
		AvailabilityZones: []string{"az1"},
		SpaceID:           space1UUID,
		SpaceName:         network.SpaceName("space-one"),
	}
	subnet2Info := network.SubnetInfo{
		ID:                network.Id(subnet2UUID.String()),
		CIDR:              "192.0.3.0/24",
		ProviderNetworkId: "lxdbr0",
		ProviderSpaceId:   network.Id("space-provider-id-2"),
		VLANTag:           84,
		AvailabilityZones: []string{"az2"},
		SpaceID:           space2UUID,
		SpaceName:         network.SpaceName("space-two"),
	}

	space1, err := s.svc.Space(c.Context(), space1UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(space1.Name, tc.Equals, network.SpaceName("space-one"))
	c.Check(space1.ProviderId, tc.Equals, network.Id("space-provider-id-1"))
	c.Check(space1.ProviderId, tc.Equals, network.Id("space-provider-id-1"))
	c.Check(space1.Subnets, tc.HasLen, 1)
	c.Check(space1.Subnets[0], tc.DeepEquals, subnet1Info)

	space2, err := s.svc.Space(c.Context(), space2UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(space2.Name, tc.Equals, network.SpaceName("space-two"))
	c.Check(space2.ProviderId, tc.Equals, network.Id("space-provider-id-2"))
	c.Check(space2.ProviderId, tc.Equals, network.Id("space-provider-id-2"))
	c.Check(space2.Subnets, tc.HasLen, 1)
	c.Check(space2.Subnets[0], tc.DeepEquals, subnet2Info)

	subnets, err := s.svc.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subnets, tc.HasLen, 2)

	subnet1, err := s.svc.Subnet(c.Context(), subnet1UUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnet1, tc.DeepEquals, &subnet1Info)

	subnet2, err := s.svc.Subnet(c.Context(), subnet2UUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subnet2, tc.DeepEquals, &subnet2Info)
}

func (s *importSuite) TestImportLinkLayerDevices(c *tc.C) {
	s.setModel(c, "ec2", model.IAAS.String())

	machineSvc := s.setupMachineService(c)
	res, err := machineSvc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	desc.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
		Name:        "test-device",
		MTU:         1500,
		ProviderID:  "net-lxdbr0",
		MachineID:   res.MachineName.String(),
		Type:        "ethernet",
		MACAddress:  "00:16:3e:ad:4e:01",
		IsAutoStart: true,
		IsUp:        true,
	})
	desc.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
		Name:        "another-device",
		MTU:         900,
		ProviderID:  "net-bridge0",
		MachineID:   res.MachineName.String(),
		Type:        "bridge",
		MACAddress:  "00:16:3e:ad:4e:02",
		IsAutoStart: false,
		IsUp:        false,
	})

	networkmodelmigration.RegisterLinkLayerDevicesImport(s.coordinator, loggertesting.WrapCheckLog(c))
	err = s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	s.checkLinkLayerDeviceExistsOnMachine(c, res.MachineName, "test-device")
	s.checkLinkLayerDeviceExistsOnMachine(c, res.MachineName, "another-device")
}

func (s *importSuite) TestImportLinkLayerDevicesWithAddresses(c *tc.C) {
	s.setModel(c, "ec2", model.IAAS.String())

	machineSvc := s.setupMachineService(c)
	res, err := machineSvc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.svc.AddSubnet(c.Context(), network.SubnetInfo{
		CIDR: "192.168.0.0/24",
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.svc.AddSubnet(c.Context(), network.SubnetInfo{
		CIDR: "2001:db8::/64",
	})
	c.Assert(err, tc.ErrorIsNil)

	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	desc.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
		Name:        "test-device",
		MTU:         1500,
		ProviderID:  "net-lxdbr0",
		MachineID:   res.MachineName.String(),
		Type:        "ethernet",
		MACAddress:  "00:16:3e:ad:4e:01",
		IsAutoStart: true,
		IsUp:        true,
	})
	desc.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
		Name:        "another-device",
		MTU:         900,
		ProviderID:  "net-bridge0",
		MachineID:   res.MachineName.String(),
		Type:        "bridge",
		MACAddress:  "00:16:3e:ad:4e:02",
		IsAutoStart: false,
		IsUp:        false,
	})

	desc.AddIPAddress(description.IPAddressArgs{
		ProviderID:   "ip-address-1",
		Value:        "192.168.0.1",
		SubnetCIDR:   "192.168.0.0/24",
		Origin:       "machine",
		MachineID:    res.MachineName.String(),
		DeviceName:   "test-device",
		ConfigMethod: string(network.ConfigStatic),
	})
	desc.AddIPAddress(description.IPAddressArgs{
		ProviderID:   "ip-address-2",
		Value:        "2001:db8::1",
		SubnetCIDR:   "2001:db8::/64",
		Origin:       "provider",
		MachineID:    res.MachineName.String(),
		DeviceName:   "another-device",
		ConfigMethod: string(network.ConfigDHCP),
	})

	networkmodelmigration.RegisterLinkLayerDevicesImport(s.coordinator, loggertesting.WrapCheckLog(c))
	err = s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	s.checkLinkLayerDeviceExistsOnMachine(c, res.MachineName, "test-device")
	s.checkLinkLayerDeviceExistsOnMachine(c, res.MachineName, "another-device")

	s.checkAddressExistsForDeviceOnMachine(c, res.MachineName, "test-device", "192.168.0.1/24")
	s.checkAddressExistsForDeviceOnMachine(c, res.MachineName, "another-device", "2001:db8::1/64")
}

func (s *importSuite) TestImportLinkLayerDevicesWithAddressesLXD(c *tc.C) {
	s.setModel(c, "lxd", model.IAAS.String())

	// Arrange
	machineSvc := s.setupMachineService(c)
	res, err := machineSvc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "22.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Arrange: add 2 subnets for 3 devices. Import will create the 3rd
	// subnet
	_, err = s.svc.AddSubnet(c.Context(), network.SubnetInfo{
		CIDR: "192.0.2.0/24",
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.svc.AddSubnet(c.Context(), network.SubnetInfo{
		CIDR: "2001:db8::/64",
	})
	c.Assert(err, tc.ErrorIsNil)

	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	// Arrange: 3 devices, 1 with no subnet in the model
	desc.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
		Name:        "test-device",
		MTU:         1500,
		ProviderID:  "net-lxdbr0",
		MachineID:   res.MachineName.String(),
		Type:        "ethernet",
		MACAddress:  "00:16:3e:ad:4e:01",
		IsAutoStart: true,
		IsUp:        true,
	})
	desc.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
		Name:        "another-device",
		MTU:         900,
		ProviderID:  "net-bridge0",
		MachineID:   res.MachineName.String(),
		Type:        "bridge",
		MACAddress:  "00:16:3e:ad:4e:02",
		IsAutoStart: false,
		IsUp:        false,
	})
	desc.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
		Name:        "cilium_host",
		MTU:         1500,
		MachineID:   res.MachineName.String(),
		Type:        "ethernet",
		MACAddress:  "42:d7:14:1e:32:58",
		ProviderID:  "nic-42:d7:14:1e:32:58",
		IsAutoStart: true,
		IsUp:        true,
	})

	desc.AddIPAddress(description.IPAddressArgs{
		ProviderID:        "ip-address-1",
		Value:             "192.0.2.1",
		SubnetCIDR:        "192.0.2.0/24",
		ProviderNetworkID: "net-lxdbr0",
		ProviderSubnetID:  "subnet--192.0.2.0/24",
		Origin:            "machine",
		MachineID:         res.MachineName.String(),
		DeviceName:        "test-device",
		ConfigMethod:      string(network.ConfigStatic),
	})
	desc.AddIPAddress(description.IPAddressArgs{
		ProviderID:        "ip-address-2",
		Value:             "2001:db8::1",
		SubnetCIDR:        "2001:db8::/64",
		ProviderNetworkID: "net-docker0",
		ProviderSubnetID:  "subnet--2001:db8::/64",
		Origin:            "provider",
		MachineID:         res.MachineName.String(),
		DeviceName:        "another-device",
		ConfigMethod:      string(network.ConfigDHCP),
	})
	desc.AddIPAddress(description.IPAddressArgs{
		Value:            "203.0.113.7",
		SubnetCIDR:       "203.0.113.0/32",
		ProviderSubnetID: "subnet--203.0.113.0/32",
		Origin:           "machine",
		MachineID:        res.MachineName.String(),
		DeviceName:       "cilium_host",
		ConfigMethod:     string(network.ConfigStatic),
	})

	networkmodelmigration.RegisterLinkLayerDevicesImport(s.coordinator, loggertesting.WrapCheckLog(c))

	// Act
	err = s.coordinator.Perform(c.Context(), s.scope, desc)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	s.checkLinkLayerDeviceExistsOnMachine(c, res.MachineName, "test-device")
	s.checkLinkLayerDeviceExistsOnMachine(c, res.MachineName, "another-device")

	// Assert the device with no subnet was added.
	s.checkLinkLayerDeviceExistsOnMachine(c, res.MachineName, "cilium_host")

	s.checkAddressExistsForDeviceOnMachine(c, res.MachineName, "test-device", "192.0.2.1/24")
	s.checkAddressExistsForDeviceOnMachine(c, res.MachineName, "another-device", "2001:db8::1/64")

	// Assert the ip address with no subnet was added.
	s.checkAddressExistsForDeviceOnMachine(c, res.MachineName, "cilium_host", "203.0.113.7/32")

	// Assert a new /32 subnet was created.
	s.checkSubnetExists(c, "203.0.113.0/32")
}

func (s *importSuite) TestImportCloudServices(c *tc.C) {
	s.createCAASApplication(c, "foo")
	s.createCAASApplication(c, "bar")

	for _, subnet := range network.FallbackSubnetInfo {
		_, err := s.svc.AddSubnet(c.Context(), network.SubnetInfo{
			CIDR: subnet.CIDR,
		})
		c.Assert(err, tc.ErrorIsNil)
	}

	desc := description.NewModel(description.ModelArgs{
		Type: string(model.CAAS),
	})
	desc.AddApplication(description.ApplicationArgs{
		Name: "foo",
		CloudService: &description.CloudServiceArgs{
			ProviderId: "provider-service-1",
			Addresses: []description.AddressArgs{
				{
					Value:  "192.0.2.1",
					Type:   "ipv4",
					Scope:  "public",
					Origin: "provider",
				},
				{
					Value:  "2001:db8::1",
					Type:   "ipv6",
					Scope:  "public",
					Origin: "provider",
				},
			},
		},
	})
	desc.AddApplication(description.ApplicationArgs{
		Name: "bar",
		CloudService: &description.CloudServiceArgs{
			ProviderId: "provider-service-2",
			Addresses: []description.AddressArgs{
				{
					Value:  "192.0.2.2",
					Type:   "ipv4",
					Scope:  "public",
					Origin: "provider",
				},
			},
		},
	})

	networkmodelmigration.RegisterImportCloudService(s.coordinator, loggertesting.WrapCheckLog(c))
	err := s.coordinator.Perform(c.Context(), s.scope, desc)
	c.Assert(err, tc.ErrorIsNil)

	s.checkAddressExistsForServiceForApp(c, "foo", "192.0.2.1")
	s.checkAddressExistsForServiceForApp(c, "foo", "2001:db8::1")
	s.checkAddressExistsForServiceForApp(c, "bar", "192.0.2.2")
}

func (s *importSuite) setupMachineService(c *tc.C) *machineservice.ProviderService {
	return machineservice.NewProviderService(
		machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c)),
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		func(ctx context.Context) (machineservice.Provider, error) {
			return machineservice.NewNoopProvider(), nil
		},
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *importSuite) setModel(c *tc.C, cloudType, modelType string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "prod",  ?, "test-model", ?)
		`, s.ModelUUID(), testing.ControllerTag.Id(), modelType, cloudType)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) checkLinkLayerDeviceExistsOnMachine(c *tc.C, machineName machine.Name, deviceName string) {
	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM link_layer_device lld
			JOIN machine m ON m.net_node_uuid = lld.net_node_uuid
			WHERE m.name = ? AND lld.name = ?
		`, machineName.String(), deviceName).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1, tc.Commentf("expected 1 link layer device for machine %q with name %q", machineName, deviceName))
}

func (s *importSuite) checkAddressExistsForDeviceOnMachine(c *tc.C, machineName machine.Name, deviceName, addressValue string) {
	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM ip_address ip
			JOIN link_layer_device lld ON ip.device_uuid = lld.uuid
			JOIN machine m ON m.net_node_uuid = lld.net_node_uuid
			WHERE m.name = ? AND lld.name = ? AND ip.address_value = ?
		`, machineName.String(), deviceName, addressValue).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	if !c.Check(count, tc.Equals, 1, tc.Commentf("expected 1 ip address for machine %q, device %q with value %q", machineName, deviceName, addressValue)) {
		s.DumpTable(c, "ip_address", "link_layer_device", "machine")
	}
}

func (s *importSuite) checkSubnetExists(c *tc.C, subnetCIDR string) {
	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM subnet
			WHERE cidr = ?
		`, subnetCIDR).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1, tc.Commentf("expected 1 subnet for cidr %q, got %d", subnetCIDR, count))
}

func (s *importSuite) checkAddressExistsForServiceForApp(c *tc.C, appName, addressValue string) {
	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM k8s_service AS ks
			JOIN application AS a ON ks.application_uuid = a.uuid
			JOIN ip_address AS ip ON ks.net_node_uuid = ip.net_node_uuid
			WHERE a.name = ? AND ip.address_value = ?
		`, appName, addressValue).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1, tc.Commentf("expected 1 ip address for application %q with value %q", appName, addressValue))
}

func (s *importSuite) createCAASApplication(c *tc.C, name string) {
	state := applicationstate.NewState(s.TxnRunnerFactory(), model.UUID(s.ModelUUID()), clock.WallClock, loggertesting.WrapCheckLog(c))

	platform := deployment.Platform{
		Channel:      "22.04/stable",
		OSType:       deployment.Ubuntu,
		Architecture: architecture.ARM64,
	}
	channel := &deployment.Channel{
		Track:  "track",
		Risk:   "stable",
		Branch: "branch",
	}
	ctx := c.Context()

	_, err := state.CreateCAASApplication(ctx, name, application.AddCAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Platform: platform,
			Channel:  channel,
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name: name,
				},
				Manifest:      s.minimalManifest(c),
				ReferenceName: name,
				Source:        charm.CharmHubSource,
				Revision:      42,
				Hash:          "hash",
			},
			CharmDownloadInfo: &charm.DownloadInfo{
				Provenance:         charm.ProvenanceDownload,
				CharmhubIdentifier: "ident",
				DownloadURL:        "https://example.com",
				DownloadSize:       42,
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) minimalManifest(c *tc.C) charm.Manifest {
	return charm.Manifest{
		Bases: []charm.Base{
			{
				Name: "ubuntu",
				Channel: charm.Channel{
					Risk: charm.RiskStable,
				},
				Architectures: []string{"amd64"},
			},
		},
	}
}
