// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/network/ipfamily"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/uuid"
)

// TestGetProvisioningInfoMachineNotFound verifies that querying a
// non-existent machine returns a MachineNotFound error.
func (s *modelStateSuite) TestGetProvisioningInfoMachineNotFound(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")

	_, err := s.state.GetMachineProvisioningInfo(c.Context(), "nonexistent", false)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetProvisioningInfoMinimal verifies a machine with only platform
// info returns a valid result with no units, no bindings, no volumes.
func (s *modelStateSuite) TestGetProvisioningInfoMinimal(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	s.addMachineWithPlatform(c, "0", "ubuntu", "22.04/stable")

	info, err := s.state.GetMachineProvisioningInfo(c.Context(), "0", false)
	c.Assert(err, tc.ErrorIsNil)

	shared, err := s.state.GetPreludeProvisioningInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.Base.OS, tc.Equals, "ubuntu")
	c.Check(info.Base.Channel.Track, tc.Equals, "22.04")
	c.Check(shared.ModelName, tc.Equals, "mymodel")
	c.Check(shared.CloudType, tc.Equals, "ec2")
	c.Check(shared.CloudRegion, tc.Equals, "us-east-1")
	c.Check(info.IsController, tc.IsFalse)
	c.Check(info.PlacementDirective, tc.IsNil)
	c.Check(info.UnitNames, tc.HasLen, 0)
	c.Check(info.EndpointBindings, tc.HasLen, 0)
}

// TestGetProvisioningInfoWithPlacement verifies placement directives are
// returned.
func (s *modelStateSuite) TestGetProvisioningInfoWithPlacement(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	machineUUID := s.addMachineWithPlatform(c, "1", "ubuntu", "24.04/stable")
	s.addMachinePlacement(c, machineUUID, "zone=us-east-1a")

	info, err := s.state.GetMachineProvisioningInfo(c.Context(), "1", false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info.PlacementDirective, tc.Not(tc.IsNil))
	c.Check(*info.PlacementDirective, tc.Equals, "zone=us-east-1a")
}

// TestGetProvisioningInfoWithConstraints verifies constraints are decoded
// from the database correctly.
func (s *modelStateSuite) TestGetProvisioningInfoWithConstraints(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	machineUUID := s.addMachineWithPlatform(c, "2", "ubuntu", "22.04/stable")
	s.addConstraint(c, machineUUID, "amd64", 4096)

	info, err := s.state.GetMachineProvisioningInfo(c.Context(), "2", false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info.Constraints.HasArch(), tc.IsTrue)
	c.Check(*info.Constraints.Arch, tc.Equals, "amd64")
	c.Assert(info.Constraints.HasMem(), tc.IsTrue)
	c.Check(*info.Constraints.Mem, tc.Equals, uint64(4096))
}

// TestGetProvisioningInfoWithUnits verifies unit names are returned.
func (s *modelStateSuite) TestGetProvisioningInfoWithUnits(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")

	// Create machine with a net_node we can reuse for units.
	netNodeUUID := s.addNetNode(c)
	machineUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?,?,?,?)`,
		machineUUID, "3", life.Alive, netNodeUUID)
	s.runQuery(c, `INSERT INTO machine_platform (machine_uuid, os_id, channel, architecture_id) VALUES (?,0,?,0)`,
		machineUUID, "22.04/stable")

	// Add an app and unit.
	spaceUUID := s.addSpace(c, "default")
	charmUUID := s.addCharm(c, "ubuntu")
	appUUID := s.addApplication(c, "ubuntu", charmUUID, spaceUUID)
	s.addUnit(c, "ubuntu/0", appUUID, charmUUID, netNodeUUID)

	info, err := s.state.GetMachineProvisioningInfo(c.Context(), "3", false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info.UnitNames, tc.HasLen, 1)
	c.Check(info.UnitNames[0].Name.String(), tc.Equals, "ubuntu/0")
	c.Check(info.UnitNames[0].IsSubordinate(), tc.IsFalse)
}

// TestGetProvisioningInfoWithEndpointBindings verifies that endpoint
// bindings for an app's units on the machine are returned.
func (s *modelStateSuite) TestGetProvisioningInfoWithEndpointBindings(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")

	netNodeUUID := s.addNetNode(c)
	machineUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?,?,?,?)`,
		machineUUID, "4", life.Alive, netNodeUUID)
	s.runQuery(c, `INSERT INTO machine_platform (machine_uuid, os_id, channel, architecture_id) VALUES (?,0,?,0)`,
		machineUUID, "22.04/stable")

	defaultSpace := s.addSpace(c, "default")
	dataSpace := s.addSpace(c, "data")

	charmUUID := s.addCharm(c, "wordpress")
	appUUID := s.addApplication(c, "wordpress", charmUUID, defaultSpace)
	s.addUnit(c, "wordpress/0", appUUID, charmUUID, netNodeUUID)

	// Add a charm relation with a specific space binding.
	relUUID := s.addCharmRelation(c, charmUUID, "db")
	s.addApplicationEndpoint(c, appUUID, relUUID, &dataSpace)

	info, err := s.state.GetMachineProvisioningInfo(c.Context(), "4", false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info.EndpointBindings, tc.HasLen, 1)
	bindings, ok := info.EndpointBindings["wordpress"]
	c.Assert(ok, tc.IsTrue)

	// Default binding.
	c.Check(string(bindings[""]), tc.Equals, defaultSpace)
	// db endpoint bound to data space.
	c.Check(string(bindings["db"]), tc.Equals, dataSpace)
}

// TestGetProvisioningInfoWithSpaces verifies spaces and subnets are
// returned.
func (s *modelStateSuite) TestGetProvisioningInfoWithSpaces(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	s.addMachineWithPlatform(c, "5", "ubuntu", "22.04/stable")

	spaceUUID := s.addSpace(c, "production")
	s.addProviderSpace(c, spaceUUID, "provider-space-1")
	subnetUUID := s.addSubnet(c, spaceUUID, "10.0.0.0/24")
	s.addProviderSubnet(c, subnetUUID, "subnet-abc123")
	s.addAvailabilityZone(c, subnetUUID, "us-east-1a")

	shared, err := s.state.GetPreludeProvisioningInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Should have at least the "production" space.
	var found bool
	for _, sp := range shared.Spaces {
		if string(sp.Name) == "production" {
			found = true
			c.Check(string(sp.ProviderId), tc.Equals, "provider-space-1")
			c.Assert(sp.Subnets, tc.HasLen, 1)
			c.Check(sp.Subnets[0].CIDR, tc.Equals, "10.0.0.0/24")
			c.Check(string(sp.Subnets[0].ProviderId), tc.Equals, "subnet-abc123")
			c.Assert(sp.Subnets[0].AvailabilityZones, tc.HasLen, 1)
			c.Check(sp.Subnets[0].AvailabilityZones[0], tc.Equals, "us-east-1a")
			break
		}
	}
	c.Assert(found, tc.IsTrue)
}

// TestGetProvisioningInfoRootDiskStoragePool verifies that the root disk
// storage pool is returned when root-disk-source constraint is set.
func (s *modelStateSuite) TestGetProvisioningInfoRootDiskStoragePool(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	machineUUID := s.addMachineWithPlatform(c, "6", "ubuntu", "22.04/stable")

	// Add constraint with root-disk-source.
	consUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO "constraint" (uuid, root_disk_source) VALUES (?,?)`,
		consUUID, "ebs-fast")
	s.runQuery(c, `INSERT INTO machine_constraint (machine_uuid, constraint_uuid) VALUES (?,?)`,
		machineUUID, consUUID)

	// Add storage pool.
	s.addStoragePool(c, "ebs-fast", "ebs", map[string]string{
		"volume-type": "gp3",
		"iops":        "3000",
	})

	info, err := s.state.GetMachineProvisioningInfo(c.Context(), "6", false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info.RootDiskStoragePool, tc.Not(tc.IsNil))
	c.Check(info.RootDiskStoragePool.Provider, tc.Equals, "ebs")
	c.Check(info.RootDiskStoragePool.Attrs["volume-type"], tc.Equals, "gp3")
	c.Check(info.RootDiskStoragePool.Attrs["iops"], tc.Equals, "3000")
}

// TestGetProvisioningInfoRootDiskNoConstraint verifies that no storage
// pool is returned when there is no root-disk-source constraint.
func (s *modelStateSuite) TestGetProvisioningInfoRootDiskNoConstraint(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	s.addMachineWithPlatform(c, "7", "ubuntu", "22.04/stable")

	info, err := s.state.GetMachineProvisioningInfo(c.Context(), "7", false)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.RootDiskStoragePool, tc.IsNil)
}

// TestGetProvisioningInfoControllerMachine verifies that a machine which
// is running a controller application is flagged as a controller.
func (s *modelStateSuite) TestGetProvisioningInfoControllerMachine(c *tc.C) {
	s.addModelInfo(c, "controller", "ec2", "us-east-1")

	netNodeUUID := s.addNetNode(c)
	machineUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?,?,?,?)`,
		machineUUID, "0", life.Alive, netNodeUUID)
	s.runQuery(c, `INSERT INTO machine_platform (machine_uuid, os_id, channel, architecture_id) VALUES (?,0,?,0)`,
		machineUUID, "22.04/stable")

	// Create a controller application with a unit on this machine.
	spaceUUID := s.addSpace(c, "default")
	charmUUID := s.addCharm(c, "juju-controller")
	appUUID := s.addApplication(c, "controller", charmUUID, spaceUUID)
	s.addApplicationController(c, appUUID)
	s.addUnit(c, "controller/0", appUUID, charmUUID, netNodeUUID)

	info, err := s.state.GetMachineProvisioningInfo(c.Context(), "0", true)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.IsController, tc.IsTrue)
}

// TestGetProvisioningInfoNotControllerMachineInControllerModel verifies
// that a non-controller machine in a controller model is not flagged.
func (s *modelStateSuite) TestGetProvisioningInfoNotControllerMachineInControllerModel(c *tc.C) {
	s.addModelInfo(c, "controller", "ec2", "us-east-1")
	s.addMachineWithPlatform(c, "1", "ubuntu", "22.04/stable")

	info, err := s.state.GetMachineProvisioningInfo(c.Context(), "1", true)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.IsController, tc.IsFalse)
}

// TestGetProvisioningInfoNotControllerModel verifies that even a controller
// machine is not flagged when isControllerModel=false.
func (s *modelStateSuite) TestGetProvisioningInfoNotControllerModel(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")

	netNodeUUID := s.addNetNode(c)
	machineUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?,?,?,?)`,
		machineUUID, "0", life.Alive, netNodeUUID)
	s.runQuery(c, `INSERT INTO machine_platform (machine_uuid, os_id, channel, architecture_id) VALUES (?,0,?,0)`,
		machineUUID, "22.04/stable")

	spaceUUID := s.addSpace(c, "default")
	charmUUID := s.addCharm(c, "juju-controller")
	appUUID := s.addApplication(c, "controller", charmUUID, spaceUUID)
	s.addApplicationController(c, appUUID)
	s.addUnit(c, "controller/0", appUUID, charmUUID, netNodeUUID)

	// isControllerModel=false, so we don't even check.
	info, err := s.state.GetMachineProvisioningInfo(c.Context(), "0", false)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(info.IsController, tc.IsFalse)
}

// TestGetProvisioningInfoImageStream verifies that image stream is
// returned as-is from the database (empty when not configured).
func (s *modelStateSuite) TestGetProvisioningInfoImageStream(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	s.addMachineWithPlatform(c, "8", "ubuntu", "22.04/stable")

	shared, err := s.state.GetPreludeProvisioningInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// State returns raw value; empty string when model_config has no
	// image-stream key. The service layer applies the "released" default.
	c.Check(shared.ImageStream, tc.Equals, "")
}

// TestGetProvisioningInfoSpaceWithMultipleSubnets verifies spaces
// aggregate multiple subnets correctly.
func (s *modelStateSuite) TestGetProvisioningInfoSpaceWithMultipleSubnets(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	s.addMachineWithPlatform(c, "9", "ubuntu", "22.04/stable")

	spaceUUID := s.addSpace(c, "internal")
	s.addProviderSpace(c, spaceUUID, "")
	sub1 := s.addSubnet(c, spaceUUID, "10.0.1.0/24")
	sub2 := s.addSubnet(c, spaceUUID, "10.0.2.0/24")
	s.addProviderSubnet(c, sub1, "subnet-1")
	s.addProviderSubnet(c, sub2, "subnet-2")

	shared, err := s.state.GetPreludeProvisioningInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	var found bool
	for _, sp := range shared.Spaces {
		if string(sp.Name) == "internal" {
			found = true
			c.Check(sp.Subnets, tc.HasLen, 2)
			break
		}
	}
	c.Assert(found, tc.IsTrue)
}

// TestGetProvisioningInfoSubordinateUnit verifies that subordinate units
// with principals have their principal names resolved.
func (s *modelStateSuite) TestGetProvisioningInfoSubordinateUnit(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")

	netNodeUUID := s.addNetNode(c)
	machineUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?,?,?,?)`,
		machineUUID, "10", life.Alive, netNodeUUID)
	s.runQuery(c, `INSERT INTO machine_platform (machine_uuid, os_id, channel, architecture_id) VALUES (?,0,?,0)`,
		machineUUID, "22.04/stable")

	spaceUUID := s.addSpace(c, "default")
	charmUUID := s.addCharm(c, "wordpress")
	appUUID := s.addApplication(c, "wordpress", charmUUID, spaceUUID)

	// Principal unit on the machine.
	principalUUID := s.addUnit(c, "wordpress/0", appUUID, charmUUID, netNodeUUID)

	// Subordinate charm and app.
	subCharmUUID := s.addCharm(c, "nrpe")
	subAppUUID := s.addApplication(c, "nrpe", subCharmUUID, spaceUUID)

	// Subordinate unit — also on the same net_node.
	subUnitUUID := s.addUnit(c, "nrpe/0", subAppUUID, subCharmUUID, netNodeUUID)

	// Link subordinate to principal.
	s.runQuery(c, `INSERT INTO unit_principal (unit_uuid, principal_uuid) VALUES (?,?)`,
		subUnitUUID, principalUUID)

	info, err := s.state.GetMachineProvisioningInfo(c.Context(), "10", false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(info.UnitNames, tc.HasLen, 2)

	// Find the subordinate unit and verify its principal.
	var foundSub bool
	for _, u := range info.UnitNames {
		if u.Name.String() == "nrpe/0" {
			foundSub = true
			c.Assert(u.Principal, tc.Not(tc.IsNil))
			c.Check(u.Principal.String(), tc.Equals, "wordpress/0")
		}
	}
	c.Assert(foundSub, tc.IsTrue)
}

// TestGetProvisioningInfoConstraintsAllFields verifies decoding of
// all constraint fields including tags, spaces and zones.
func (s *modelStateSuite) TestGetProvisioningInfoConstraintsAllFields(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	machineUUID := s.addMachineWithPlatform(c, "11", "ubuntu", "22.04/stable")

	// Add space for space constraint (constraint_space references space.name).
	s.addSpace(c, "myspace")

	consUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO "constraint" (uuid, arch, cpu_cores, cpu_power, mem, root_disk, root_disk_source, instance_role, instance_type, virt_type, allocate_public_ip, image_id, ip_family) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		consUUID, "arm64", 8, 2000, 16384, 51200, "ebs", "worker", "m5.large", "hvm", 1, "ami-custom", "ipv4")
	s.runQuery(c, `INSERT INTO machine_constraint (machine_uuid, constraint_uuid) VALUES (?,?)`,
		machineUUID, consUUID)
	s.runQuery(c, `INSERT INTO constraint_tag (constraint_uuid, tag) VALUES (?,?)`,
		consUUID, "department=engineering")
	s.runQuery(c, `INSERT INTO constraint_tag (constraint_uuid, tag) VALUES (?,?)`,
		consUUID, "team=platform")
	s.runQuery(c, `INSERT INTO constraint_space (constraint_uuid, space, "exclude") VALUES (?,?,?)`,
		consUUID, "myspace", false)
	s.runQuery(c, `INSERT INTO constraint_zone (constraint_uuid, zone) VALUES (?,?)`,
		consUUID, "us-east-1a")
	s.runQuery(c, `INSERT INTO constraint_zone (constraint_uuid, zone) VALUES (?,?)`,
		consUUID, "us-east-1b")

	info, err := s.state.GetMachineProvisioningInfo(c.Context(), "11", false)
	c.Assert(err, tc.ErrorIsNil)

	cons := info.Constraints
	c.Assert(cons.HasArch(), tc.IsTrue)
	c.Check(*cons.Arch, tc.Equals, "arm64")
	c.Check(*cons.CpuCores, tc.Equals, uint64(8))
	c.Check(*cons.CpuPower, tc.Equals, uint64(2000))
	c.Check(*cons.Mem, tc.Equals, uint64(16384))
	c.Check(*cons.RootDisk, tc.Equals, uint64(51200))
	c.Check(*cons.RootDiskSource, tc.Equals, "ebs")
	c.Check(*cons.InstanceRole, tc.Equals, "worker")
	c.Check(*cons.InstanceType, tc.Equals, "m5.large")
	c.Check(*cons.VirtType, tc.Equals, "hvm")
	c.Assert(cons.HasAllocatePublicIP(), tc.IsTrue)
	c.Check(*cons.AllocatePublicIP, tc.IsTrue)
	c.Check(*cons.ImageID, tc.Equals, "ami-custom")
	c.Assert(cons.IPFamily, tc.Not(tc.IsNil))
	c.Check(*cons.IPFamily, tc.Equals, ipfamily.IPv4)
	c.Assert(cons.Tags, tc.Not(tc.IsNil))
	c.Check(*cons.Tags, tc.HasLen, 2)
	c.Assert(cons.Zones, tc.Not(tc.IsNil))
	c.Check(*cons.Zones, tc.HasLen, 2)
	c.Assert(cons.HasSpaces(), tc.IsTrue)
}

// TestGetProvisioningInfoEndpointNullSpace verifies that an endpoint with
// NULL space_uuid falls back to the app's default space.
func (s *modelStateSuite) TestGetProvisioningInfoEndpointNullSpace(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")

	netNodeUUID := s.addNetNode(c)
	machineUUID := uuid.MustNewUUID().String()
	s.runQuery(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?,?,?,?)`,
		machineUUID, "12", life.Alive, netNodeUUID)
	s.runQuery(c, `INSERT INTO machine_platform (machine_uuid, os_id, channel, architecture_id) VALUES (?,0,?,0)`,
		machineUUID, "22.04/stable")

	defaultSpace := s.addSpace(c, "default")
	charmUUID := s.addCharm(c, "mysql")
	appUUID := s.addApplication(c, "mysql", charmUUID, defaultSpace)
	s.addUnit(c, "mysql/0", appUUID, charmUUID, netNodeUUID)

	// Add endpoint with NULL space (should inherit app default).
	relUUID := s.addCharmRelation(c, charmUUID, "server")
	s.addApplicationEndpoint(c, appUUID, relUUID, nil)

	info, err := s.state.GetMachineProvisioningInfo(c.Context(), "12", false)
	c.Assert(err, tc.ErrorIsNil)

	bindings := info.EndpointBindings["mysql"]
	c.Assert(bindings, tc.Not(tc.IsNil))
	// "server" endpoint should fall back to default space.
	c.Check(string(bindings["server"]), tc.Equals, defaultSpace)
}

// TestGetProvisioningInfoSubnetMultiAZ verifies that a subnet with
// multiple availability zones aggregates them correctly.
func (s *modelStateSuite) TestGetProvisioningInfoSubnetMultiAZ(c *tc.C) {
	s.addModelInfo(c, "mymodel", "ec2", "us-east-1")
	s.addMachineWithPlatform(c, "13", "ubuntu", "22.04/stable")

	spaceUUID := s.addSpace(c, "multiaz")
	s.addProviderSpace(c, spaceUUID, "")
	subnetUUID := s.addSubnet(c, spaceUUID, "10.0.5.0/24")
	s.addProviderSubnet(c, subnetUUID, "subnet-multi")
	s.addAvailabilityZone(c, subnetUUID, "us-east-1a")
	s.addAvailabilityZone(c, subnetUUID, "us-east-1b")

	shared, err := s.state.GetPreludeProvisioningInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	var found bool
	for _, sp := range shared.Spaces {
		if string(sp.Name) == "multiaz" {
			found = true
			c.Assert(sp.Subnets, tc.HasLen, 1)
			c.Check(sp.Subnets[0].AvailabilityZones, tc.HasLen, 2)
			break
		}
	}
	c.Assert(found, tc.IsTrue)
}
