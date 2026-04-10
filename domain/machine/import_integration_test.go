// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	machinemodelmigration "github.com/juju/juju/domain/machine/modelmigration"
	"github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/domain/machine/state"
	networkstate "github.com/juju/juju/domain/network/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type importSuite struct {
	schematesting.ModelSuite

	spaceUUID network.SpaceUUID
	subnetID  network.Id
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.spaceUUID = tc.Must(c, network.NewSpaceUUID)
	s.subnetID = network.Id(tc.Must(c, uuid.NewUUID).String())

	networkState := networkstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := networkState.AddSpace(c.Context(), s.spaceUUID, "test-space", "provider-space-id", []string{})
	c.Assert(err, tc.ErrorIsNil)
	err = networkState.AddSubnet(c.Context(), network.SubnetInfo{
		ID:                s.subnetID,
		SpaceID:           s.spaceUUID,
		CIDR:              "10.0.0.0/24",
		AvailabilityZones: []string{"zone1", "zone2"},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportMachine(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{})
	m0 := desc.AddMachine(description.MachineArgs{
		Id:                  "0",
		Nonce:               "nonce-0",
		PasswordHash:        "h@sh",
		Base:                "ubuntu@24.04",
		ContainerType:       "lxd",
		SupportedContainers: &[]string{"lxd"},
	})
	m0.SetAddresses([]description.AddressArgs{{
		Value:   "192.168.0.1",
		Type:    "ipv4",
		Scope:   "machine",
		Origin:  "manual",
		SpaceID: s.spaceUUID.String(),
	}}, []description.AddressArgs{{
		Value:   "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		Type:    "ipv6",
		Scope:   "model",
		Origin:  "manual",
		SpaceID: s.spaceUUID.String(),
	}})
	m0.SetAnnotations(map[string]string{
		"annotation-key": "annotation-value",
	})
	m0.SetConstraints(description.ConstraintsArgs{
		CpuCores: 4,
		Memory:   8192,
		RootDisk: 1024,
		Tags:     []string{"tag1", "tag2"},
	})
	m0.SetInstance(description.CloudInstanceArgs{
		InstanceId:       "inst-0",
		DisplayName:      "test instance",
		Architecture:     "amd64",
		Memory:           16384,
		RootDisk:         20480,
		RootDiskSource:   "/dev/sda1",
		CpuCores:         8,
		CpuPower:         32,
		Tags:             []string{"cloud-tag1", "cloud-tag2"},
		AvailabilityZone: "zone1",
		VirtType:         "kvm",
	})
	m0.SetTools(description.AgentToolsArgs{
		Version: "1.2.3",
		URL:     "http://example.com/tools",
		SHA256:  "abc123",
		Size:    123,
	})

	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	machinemodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := coordinator.Perform(c.Context(), modelmigration.NewScope(nil, s.TxnRunnerFactory(),
		nil, nil, model.UUID(s.ModelUUID())), desc)
	c.Assert(err, tc.ErrorIsNil)

	svc := s.setupService(c)
	machineNames, err := svc.AllMachineNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineNames, tc.DeepEquals, []machine.Name{"0"})
	machineName := machineNames[0]

	machineUUID, err := svc.GetMachineUUID(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)

	instanceID, err := svc.GetInstanceID(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceID, tc.Equals, instance.Id("inst-0"))

	base, err := svc.GetMachineBase(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(base, tc.Equals, corebase.MustParseBaseFromString("ubuntu@24.04"))

	cons, err := svc.GetMachineConstraints(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, constraints.Value{
		CpuCores: new(uint64(4)),
		Mem:      new(uint64(8192)),
		RootDisk: new(uint64(1024)),
		Tags:     &[]string{"tag1", "tag2"},
	})

	placement, err := svc.GetMachinePlacementDirective(c.Context(), machineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(placement, tc.IsNil)

	containerTypes, err := svc.GetSupportedContainersTypes(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(containerTypes, tc.DeepEquals, []instance.ContainerType{"lxd"})

	machineHardwareCharacteristics, err := svc.GetHardwareCharacteristics(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineHardwareCharacteristics, tc.DeepEquals, instance.HardwareCharacteristics{
		Arch:             new("amd64"),
		Mem:              new(uint64(16384)),
		RootDisk:         new(uint64(20480)),
		RootDiskSource:   new("/dev/sda1"),
		CpuCores:         new(uint64(8)),
		CpuPower:         new(uint64(32)),
		Tags:             &[]string{"cloud-tag1", "cloud-tag2"},
		AvailabilityZone: new("zone1"),
		VirtType:         new("kvm"),
	})
}

func (s *importSuite) TestImportMachineParentSubordinate(c *tc.C) {
	desc := description.NewModel(description.ModelArgs{})
	m0 := desc.AddMachine(description.MachineArgs{
		Id:            "0",
		ContainerType: "lxd",
		Base:          "ubuntu@24.04",
	})

	m0.AddContainer(description.MachineArgs{
		Id:        "0/lxd/0",
		Placement: "lxd:0",
		Base:      "ubuntu@24.04",
	})

	coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
	machinemodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
	err := coordinator.Perform(c.Context(), modelmigration.NewScope(nil, s.TxnRunnerFactory(),
		nil, nil, model.UUID(s.ModelUUID())), desc)
	c.Assert(err, tc.ErrorIsNil)

	svc := s.setupService(c)
	machineNames, err := svc.AllMachineNames(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machineNames, tc.DeepEquals, []machine.Name{"0", "0/lxd/0"})

	parentUUID, err := svc.GetMachineUUID(c.Context(), machine.Name("0"))
	c.Assert(err, tc.ErrorIsNil)

	subordinateUUID, err := svc.GetMachineUUID(c.Context(), machine.Name("0/lxd/0"))
	c.Assert(err, tc.ErrorIsNil)

	subordinateParentUUID, err := svc.GetMachineParentUUID(c.Context(), subordinateUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subordinateParentUUID, tc.Equals, parentUUID)

	parentSubordinates, err := svc.GetMachineContainers(c.Context(), parentUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(parentSubordinates, tc.DeepEquals, []machine.Name{"0/lxd/0"})
}

func (s *importSuite) setupService(c *tc.C) *service.Service {
	return service.NewService(
		state.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c)),
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}
