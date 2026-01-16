// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"context"
	"testing"

	"github.com/juju/description/v11"
	"github.com/juju/tc"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	corenetwork "github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	"github.com/juju/juju/domain/life"
	networkmodelmigration "github.com/juju/juju/domain/network/modelmigration"
	"github.com/juju/juju/domain/network/service"
	"github.com/juju/juju/domain/network/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type importSuite struct {
	schematesting.ModelSuite
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

// TestImportSpacesAndSubnets tests the import of spaces and subnets during
// model migration.
func (s *importSuite) TestImportSpacesAndSubnets(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	// Add spaces to the model description.
	spaceUUID1 := networktesting.GenSpaceUUID(c)
	desc.AddSpace(description.SpaceArgs{
		Id:         spaceUUID1.String(),
		Name:       "space1",
		ProviderID: "provider-space-1",
	})

	spaceUUID2 := networktesting.GenSpaceUUID(c)
	desc.AddSpace(description.SpaceArgs{
		Id:         spaceUUID2.String(),
		Name:       "space2",
		ProviderID: "provider-space-2",
	})

	// Add subnets linked to spaces.
	subnet1UUID := tc.Must(c, uuid.NewUUID).String()
	desc.AddSubnet(description.SubnetArgs{
		UUID:              subnet1UUID,
		CIDR:              "192.168.1.0/24",
		ProviderId:        "subnet-provider-id-1",
		ProviderNetworkId: "network-provider-id-1",
		VLANTag:           100,
		AvailabilityZones: []string{"az1", "az2"},
		SpaceID:           spaceUUID1.String(),
		SpaceName:         "space1",
		ProviderSpaceId:   "provider-space-1",
	})

	subnet2UUID := tc.Must(c, uuid.NewUUID).String()
	desc.AddSubnet(description.SubnetArgs{
		UUID:              subnet2UUID,
		CIDR:              "10.0.0.0/16",
		ProviderId:        "subnet-provider-id-2",
		ProviderNetworkId: "network-provider-id-2",
		VLANTag:           200,
		AvailabilityZones: []string{"az3"},
		SpaceID:           spaceUUID2.String(),
		SpaceName:         "space2",
		ProviderSpaceId:   "provider-space-2",
	})

	// Setup coordinator and perform import.
	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	networkmodelmigration.RegisterImportSubnets(coordinator, loggertesting.WrapCheckLog(c))
	err := coordinator.Perform(c.Context(), modelmigration.NewScope(nil, s.TxnRunnerFactory(),
		nil, model.UUID(s.ModelUUID())), desc)
	c.Assert(err, tc.ErrorIsNil)

	// Verify spaces were imported correctly.
	svc := s.setupService(c)
	spaces, err := svc.GetAllSpaces(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Should have 2 imported spaces plus the default alpha space.
	c.Assert(len(spaces), tc.Equals, 3)

	space1 := spaces.GetByName("space1")
	c.Assert(space1, tc.NotNil)
	c.Check(space1.Name, tc.Equals, corenetwork.SpaceName("space1"))
	c.Check(space1.ProviderId, tc.Equals, corenetwork.Id("provider-space-1"))

	space2 := spaces.GetByName("space2")
	c.Assert(space2, tc.NotNil)
	c.Check(space2.Name, tc.Equals, corenetwork.SpaceName("space2"))
	c.Check(space2.ProviderId, tc.Equals, corenetwork.Id("provider-space-2"))

	// Verify subnets were imported correctly.
	subnets, err := svc.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(subnets), tc.Equals, 2)

	subnet1, err := subnets.GetByCIDR("192.168.1.0/24")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(subnet1), tc.Equals, 1)
	c.Check(subnet1[0].CIDR, tc.Equals, "192.168.1.0/24")
	c.Check(subnet1[0].ProviderId, tc.Equals, corenetwork.Id("subnet-provider-id-1"))
	c.Check(subnet1[0].ProviderNetworkId, tc.Equals, corenetwork.Id("network-provider-id-1"))
	c.Check(subnet1[0].VLANTag, tc.Equals, 100)
	c.Check(subnet1[0].AvailabilityZones, tc.DeepEquals, []string{"az1", "az2"})
	c.Check(subnet1[0].SpaceName, tc.Equals, corenetwork.SpaceName("space1"))

	subnet2, err := subnets.GetByCIDR("10.0.0.0/16")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(subnet2), tc.Equals, 1)
	c.Check(subnet2[0].CIDR, tc.Equals, "10.0.0.0/16")
	c.Check(subnet2[0].ProviderId, tc.Equals, corenetwork.Id("subnet-provider-id-2"))
	c.Check(subnet2[0].SpaceName, tc.Equals, corenetwork.SpaceName("space2"))
}

// TestImportLinkLayerDevices tests the import of link layer devices and
// IP addresses during model migration.
func (s *importSuite) TestImportLinkLayerDevices(c *tc.C) {
	// First, set up prerequisite data: spaces, subnets, and machines.
	svc := s.setupService(c)

	// Create a space and subnet.
	spaceUUID, err := svc.AddSpace(c.Context(), corenetwork.SpaceInfo{
		Name:       "test-space",
		ProviderId: "provider-space-1",
	})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID := tc.Must(c, uuid.NewUUID).String()
	_, err = svc.AddSubnet(c.Context(), corenetwork.SubnetInfo{
		ID:                corenetwork.Id(subnetUUID),
		CIDR:              "192.168.1.0/24",
		ProviderId:        "subnet-provider-1",
		ProviderNetworkId: "network-provider-1",
		VLANTag:           0,
		AvailabilityZones: []string{"az1"},
		SpaceID:           spaceUUID,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Add a machine.
	machineID := "0"
	machineUUID := s.addMachine(c, machineID)

	// Create the model description with link layer devices.
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	lld := desc.AddLinkLayerDevice(description.LinkLayerDeviceArgs{
		MachineID:       machineID,
		Name:            "eth0",
		MTU:             1500,
		MACAddress:      "aa:bb:cc:dd:ee:ff",
		ProviderID:      "device-provider-1",
		Type:            "ethernet",
		VirtualPortType: "",
		IsAutoStart:     true,
		IsUp:            true,
		ParentName:      "",
	})

	lld.AddIPAddress(description.IPAddressArgs{
		MachineID:        machineID,
		DeviceName:       "eth0",
		Value:            "192.168.1.10/24",
		ConfigMethod:     "static",
		Origin:           "provider",
		SubnetCIDR:       "192.168.1.0/24",
		ProviderSubnetID: "subnet-provider-1",
		IsSecondary:      false,
		IsShadow:         false,
	})

	// Setup coordinator and perform import.
	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	networkmodelmigration.RegisterLinkLayerDevicesImport(coordinator, loggertesting.WrapCheckLog(c))
	err = coordinator.Perform(c.Context(), modelmigration.NewScope(nil, s.TxnRunnerFactory(),
		nil, model.UUID(s.ModelUUID())), desc)
	c.Assert(err, tc.ErrorIsNil)

	// Verify link layer device was imported.
	devices, err := svc.GetAllMachineNetInterfaces(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(devices), tc.Equals, 1)

	device := devices[0]
	c.Check(device.Name, tc.Equals, "eth0")
	c.Check(*device.MTU, tc.Equals, int64(1500))
	c.Check(*device.MACAddress, tc.Equals, "aa:bb:cc:dd:ee:ff")
	c.Check(*device.ProviderID, tc.Equals, corenetwork.Id("device-provider-1"))
	c.Check(device.Type, tc.Equals, corenetwork.EthernetDevice)
	c.Check(device.IsAutoStart, tc.Equals, true)
	c.Check(device.IsEnabled, tc.Equals, true)

	// Verify IP address was imported.
	c.Assert(len(device.Addrs), tc.Equals, 1)
	addr := device.Addrs[0]
	c.Check(addr.AddressValue, tc.Equals, "192.168.1.10/24")
	c.Check(addr.ConfigType, tc.Equals, corenetwork.ConfigStatic)
	c.Check(addr.Origin, tc.Equals, corenetwork.OriginProvider)
	c.Check(addr.IsSecondary, tc.Equals, false)
	c.Check(addr.IsShadow, tc.Equals, false)
}

// TestImportCloudServices tests the import of cloud services for CAAS models
// during model migration.
func (s *importSuite) TestImportCloudServices(c *tc.C) {
	// First, set up prerequisite data: space, subnet, and application.
	svc := s.setupService(c)

	// Create a space and subnet.
	spaceUUID, err := svc.AddSpace(c.Context(), corenetwork.SpaceInfo{
		Name:       "k8s-space",
		ProviderId: "k8s-provider-space",
	})
	c.Assert(err, tc.ErrorIsNil)

	subnetUUID := tc.Must(c, uuid.NewUUID).String()
	_, err = svc.AddSubnet(c.Context(), corenetwork.SubnetInfo{
		ID:                corenetwork.Id(subnetUUID),
		CIDR:              "10.0.1.0/24",
		ProviderId:        "k8s-subnet-provider",
		ProviderNetworkId: "k8s-network-provider",
		VLANTag:           0,
		AvailabilityZones: []string{"k8s-az"},
		SpaceID:           spaceUUID,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Add an application.
	appName := "test-app"
	s.addApplication(c, appName)

	// Create the model description with cloud service.
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.CAAS),
	})

	app := desc.AddApplication(description.ApplicationArgs{
		Name: appName,
	})

	cloudService := app.SetCloudService(description.CloudServiceArgs{
		ProviderId: "k8s-service-provider-1",
	})

	cloudService.AddAddress(description.AddressArgs{
		Value:   "10.0.1.20",
		Type:    "ipv4",
		Scope:   "public",
		Origin:  "provider",
		SpaceID: spaceUUID.String(),
	})

	// Setup coordinator and perform import.
	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	networkmodelmigration.RegisterImportCloudService(coordinator, loggertesting.WrapCheckLog(c))
	err = coordinator.Perform(c.Context(), modelmigration.NewScope(nil, s.TxnRunnerFactory(),
		nil, model.UUID(s.ModelUUID())), desc)
	c.Assert(err, tc.ErrorIsNil)

	// Verify cloud service was imported by checking the k8s_service table.
	s.checkK8sServiceExists(c, appName)
}

// TestImportMinimalSpaceAndSubnet tests importing spaces and subnets with
// minimal fields populated.
func (s *importSuite) TestImportMinimalSpaceAndSubnet(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{
		Type: string(model.IAAS),
	})

	// Add space with only required fields.
	spaceUUID := networktesting.GenSpaceUUID(c)
	desc.AddSpace(description.SpaceArgs{
		Id:   spaceUUID.String(),
		Name: "minimal-space",
	})

	// Add subnet with minimal fields.
	subnetUUID := tc.Must(c, uuid.NewUUID).String()
	desc.AddSubnet(description.SubnetArgs{
		UUID:      subnetUUID,
		CIDR:      "172.16.0.0/16",
		SpaceID:   spaceUUID.String(),
		SpaceName: "minimal-space",
	})

	// Setup coordinator and perform import.
	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	networkmodelmigration.RegisterImportSubnets(coordinator, loggertesting.WrapCheckLog(c))
	err := coordinator.Perform(c.Context(), modelmigration.NewScope(nil, s.TxnRunnerFactory(),
		nil, model.UUID(s.ModelUUID())), desc)
	c.Assert(err, tc.ErrorIsNil)

	// Verify import succeeded.
	svc := s.setupService(c)
	spaces, err := svc.GetAllSpaces(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	space := spaces.GetByName("minimal-space")
	c.Assert(space, tc.NotNil)
	c.Check(space.Name, tc.Equals, corenetwork.SpaceName("minimal-space"))

	subnets, err := svc.GetAllSubnets(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	subnet, err := subnets.GetByCIDR("172.16.0.0/16")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(subnet), tc.Equals, 1)
	c.Check(subnet[0].CIDR, tc.Equals, "172.16.0.0/16")
	c.Check(subnet[0].SpaceName, tc.Equals, corenetwork.SpaceName("minimal-space"))
}

// setupService creates and returns a network service configured for testing.
func (s *importSuite) setupService(c *tc.C) *service.Service {
	modelDB := func(context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	return service.NewService(
		state.NewState(modelDB, loggertesting.WrapCheckLog(c)),
		loggertesting.WrapCheckLog(c),
	)
}

// addMachine adds a machine to the database and returns its UUID.
func (s *importSuite) addMachine(c *tc.C, name string) machine.UUID {
	machineUUID := tc.Must(c, uuid.NewUUID).String()
	netNodeUUID := tc.Must(c, uuid.NewUUID).String()

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *database.StdTx) error {
		// Insert net_node first.
		_, err := tx.ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", netNodeUUID)
		if err != nil {
			return err
		}

		// Insert machine.
		_, err = tx.ExecContext(ctx,
			"INSERT INTO machine (uuid, net_node_uuid, name, life_id) VALUES (?, ?, ?, ?)",
			machineUUID, netNodeUUID, name, life.Alive)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return machine.UUID(machineUUID)
}

// addApplication adds an application to the database.
func (s *importSuite) addApplication(c *tc.C, name string) {
	appUUID := tc.Must(c, uuid.NewUUID).String()
	charmUUID := tc.Must(c, uuid.NewUUID).String()

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *database.StdTx) error {
		// Insert charm first.
		_, err := tx.ExecContext(ctx,
			"INSERT INTO charm (uuid, reference_name) VALUES (?, ?)",
			charmUUID, name)
		if err != nil {
			return err
		}

		// Insert application.
		_, err = tx.ExecContext(ctx,
			"INSERT INTO application (uuid, name, life_id, charm_uuid) VALUES (?, ?, ?, ?)",
			appUUID, name, life.Alive, charmUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// checkK8sServiceExists verifies that a cloud service exists in the database
// for the given application.
func (s *importSuite) checkK8sServiceExists(c *tc.C, appName string) {
	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *database.StdTx) error {
		return tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM k8s_service 
			WHERE application_uuid = (SELECT uuid FROM application WHERE name = ?)
		`, appName).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1, tc.Commentf("expected 1 k8s_service for application %q", appName))
}
