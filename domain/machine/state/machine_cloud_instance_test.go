// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"strconv"

	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	coremachinetesting "github.com/juju/juju/core/machine/testing"
	machineerrors "github.com/juju/juju/domain/machine/errors"
)

func (s *stateSuite) TestGetHardwareCharacteristicsWithNoData(c *tc.C) {
	machineUUID := coremachinetesting.GenUUID(c)

	_, err := s.state.GetHardwareCharacteristics(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
}

func (s *stateSuite) TestGetHardwareCharacteristics(c *tc.C) {
	machineUUID := s.ensureInstance(c, "42")

	hc, err := s.state.GetHardwareCharacteristics(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*hc.Arch, tc.Equals, "arm64")
	c.Check(*hc.Mem, tc.Equals, uint64(1024))
	c.Check(*hc.RootDisk, tc.Equals, uint64(256))
	c.Check(*hc.RootDiskSource, tc.Equals, "/test")
	c.Check(*hc.CpuCores, tc.Equals, uint64(4))
	c.Check(*hc.CpuPower, tc.Equals, uint64(75))
	c.Check(*hc.AvailabilityZone, tc.Equals, "az-1")
	c.Check(*hc.VirtType, tc.Equals, "virtual-machine")
}

func (s *stateSuite) TestGetHardwareCharacteristicsWithoutAvailabilityZone(c *tc.C) {
	db := s.DB()
	// Create a reference machine.
	err := s.state.CreateMachine(c.Context(), "42", "", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	var machineUUID machine.UUID
	err = db.QueryRowContext(c.Context(), "SELECT uuid FROM machine WHERE name='42'").Scan(&machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		c.Context(),
		machineUUID,
		instance.Id("123"),
		"",
		"nonce",
		&instance.HardwareCharacteristics{
			Arch:           ptr("arm64"),
			Mem:            ptr[uint64](1024),
			RootDisk:       ptr[uint64](256),
			RootDiskSource: ptr("/test"),
			CpuCores:       ptr[uint64](4),
			CpuPower:       ptr[uint64](75),
			Tags:           ptr([]string{"tag1", "tag2"}),
			VirtType:       ptr("virtual-machine"),
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	hc, err := s.state.GetHardwareCharacteristics(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(*hc.Arch, tc.Equals, "arm64")
	c.Check(*hc.Mem, tc.Equals, uint64(1024))
	c.Check(*hc.RootDisk, tc.Equals, uint64(256))
	c.Check(*hc.RootDiskSource, tc.Equals, "/test")
	c.Check(*hc.CpuCores, tc.Equals, uint64(4))
	c.Check(*hc.CpuPower, tc.Equals, uint64(75))
	c.Check(hc.AvailabilityZone, tc.IsNil)
	c.Check(*hc.VirtType, tc.Equals, "virtual-machine")
}

func (s *stateSuite) TestAvailabilityZoneWithNoMachine(c *tc.C) {
	machineUUID := coremachinetesting.GenUUID(c)

	_, err := s.state.AvailabilityZone(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIs, machineerrors.AvailabilityZoneNotFound)
}

func (s *stateSuite) TestAvailabilityZone(c *tc.C) {
	machineUUID := s.ensureInstance(c, "42")

	az, err := s.state.AvailabilityZone(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(az, tc.Equals, "az-1")
}

func (s *stateSuite) TestSetInstanceData(c *tc.C) {
	db := s.DB()

	// Create a reference machine.
	err := s.state.CreateMachine(c.Context(), "42", "", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	var machineUUID machine.UUID
	row := db.QueryRowContext(c.Context(), "SELECT uuid FROM machine WHERE name='42'")
	err = row.Scan(&machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(row.Err(), tc.ErrorIsNil)
	// Add a reference AZ.
	_, err = db.ExecContext(c.Context(), "INSERT INTO availability_zone VALUES('deadbeef-0bad-400d-8000-4b1d0d06f00d', 'az-1')")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		c.Context(),
		machineUUID,
		instance.Id("1"),
		"one",
		"nonce",
		&instance.HardwareCharacteristics{
			Arch:             ptr("arm64"),
			Mem:              ptr[uint64](1024),
			RootDisk:         ptr[uint64](256),
			CpuCores:         ptr[uint64](4),
			CpuPower:         ptr[uint64](75),
			Tags:             ptr([]string{"tag1", "tag2"}),
			AvailabilityZone: ptr("az-1"),
			VirtType:         ptr("virtual-machine"),
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	var instanceData instanceData
	row = db.QueryRowContext(c.Context(), "SELECT * FROM machine_cloud_instance WHERE instance_id='1'")
	c.Assert(row.Err(), tc.ErrorIsNil)
	err = row.Scan(
		&instanceData.MachineUUID,
		&instanceData.InstanceID,
		&instanceData.DisplayName,
		&instanceData.Arch,
		&instanceData.Mem,
		&instanceData.RootDisk,
		&instanceData.RootDiskSource,
		&instanceData.CPUCores,
		&instanceData.CPUPower,
		&instanceData.AvailabilityZoneUUID,
		&instanceData.VirtType,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceData.MachineUUID, tc.Equals, machineUUID)
	c.Check(instanceData.InstanceID, tc.DeepEquals, sql.Null[string]{V: "1", Valid: true})
	c.Check(instanceData.DisplayName, tc.DeepEquals, sql.Null[string]{V: "one", Valid: true})
	c.Check(*instanceData.Arch, tc.Equals, "arm64")
	c.Check(*instanceData.Mem, tc.Equals, uint64(1024))
	c.Check(*instanceData.RootDisk, tc.Equals, uint64(256))
	// Make sure we also handle correctly NULL values.
	c.Check(instanceData.RootDiskSource, tc.IsNil)
	c.Check(*instanceData.CPUCores, tc.Equals, uint64(4))
	c.Check(*instanceData.CPUPower, tc.Equals, uint64(75))
	c.Check(*instanceData.AvailabilityZoneUUID, tc.Equals, "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Check(*instanceData.VirtType, tc.Equals, "virtual-machine")

	rows, err := db.QueryContext(c.Context(), "SELECT tag FROM instance_tag WHERE machine_uuid='"+machineUUID.String()+"'")
	defer func() { _ = rows.Close() }()
	c.Assert(err, tc.ErrorIsNil)
	var instanceTags []string
	for rows.Next() {
		var tag string
		err = rows.Scan(&tag)
		c.Assert(err, tc.ErrorIsNil)
		instanceTags = append(instanceTags, tag)
	}
	c.Check(instanceTags, tc.HasLen, 2)
	c.Check(instanceTags[0], tc.Equals, "tag1")
	c.Check(instanceTags[1], tc.Equals, "tag2")
}

func (s *stateSuite) TestSetInstanceDataEmptyInstanceID(c *tc.C) {
	db := s.DB()

	// Create a reference machine.
	err := s.state.CreateMachine(c.Context(), "42", "", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	var machineUUID machine.UUID
	row := db.QueryRowContext(c.Context(), "SELECT uuid FROM machine WHERE name='42'")
	err = row.Scan(&machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(row.Err(), tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		c.Context(),
		machineUUID,
		instance.Id(""),
		"one",
		"nonce",
		&instance.HardwareCharacteristics{},
	)
	c.Assert(err, tc.ErrorIsNil)

	var instanceID sql.Null[string]
	row = db.QueryRowContext(c.Context(), "SELECT instance_id FROM machine_cloud_instance WHERE machine_uuid=?", machineUUID)
	err = row.Scan(
		&instanceID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(row.Err(), tc.ErrorIsNil)
	c.Check(instanceID.Valid, tc.IsFalse)
}

func (s *stateSuite) TestSetInstanceDataEmptyDisplayName(c *tc.C) {
	db := s.DB()

	// Create a reference machine.
	err := s.state.CreateMachine(c.Context(), "42", "", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	var machineUUID machine.UUID
	row := db.QueryRowContext(c.Context(), "SELECT uuid FROM machine WHERE name='42'")
	err = row.Scan(&machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(row.Err(), tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		c.Context(),
		machineUUID,
		instance.Id("1"),
		"",
		"nonce",
		&instance.HardwareCharacteristics{},
	)
	c.Assert(err, tc.ErrorIsNil)

	var displayName sql.Null[string]
	row = db.QueryRowContext(c.Context(), "SELECT display_name FROM machine_cloud_instance WHERE machine_uuid=?", machineUUID)
	c.Assert(row.Err(), tc.ErrorIsNil)
	err = row.Scan(
		&displayName,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(displayName.Valid, tc.IsFalse)
}

func (s *stateSuite) TestSetInstanceDataEmptyUniqueIndex(c *tc.C) {
	db := s.DB()

	// Ensure that setting empty instance IDs and display names does not
	// violate the unique index on the machine_cloud_instance table.

	for i := range 10 {
		name := strconv.Itoa(i + 1)

		uuid := coremachinetesting.GenUUID(c)

		// Create a reference machine.
		err := s.state.CreateMachine(c.Context(), machine.Name(name), name, uuid, nil)
		c.Assert(err, tc.ErrorIsNil)
		var machineUUID machine.UUID
		row := db.QueryRowContext(c.Context(), "SELECT uuid FROM machine WHERE name=?", name)
		err = row.Scan(&machineUUID)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(row.Err(), tc.ErrorIsNil)

		err = s.state.SetMachineCloudInstance(
			c.Context(),
			machineUUID,
			instance.Id(""),
			"",
			"nonce",
			&instance.HardwareCharacteristics{},
		)
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *stateSuite) TestSetInstanceDataAlreadyExists(c *tc.C) {
	db := s.DB()

	// Create a reference machine.
	err := s.state.CreateMachine(c.Context(), "42", "", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	var machineUUID machine.UUID
	row := db.QueryRowContext(c.Context(), "SELECT uuid FROM machine WHERE name='42'")
	c.Assert(row.Err(), tc.ErrorIsNil)
	err = row.Scan(&machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		c.Context(),
		machineUUID,
		instance.Id("1"),
		"one",
		"nonce",
		&instance.HardwareCharacteristics{
			Arch: ptr("arm64"),
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// Must fail when we try to add again.
	err = s.state.SetMachineCloudInstance(
		c.Context(),
		machineUUID,
		instance.Id("1"),
		"one",
		"nonce",
		&instance.HardwareCharacteristics{
			Arch: ptr("amd64"),
		},
	)
	c.Assert(err, tc.ErrorMatches, "machine cloud instance already exists.*")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineCloudInstanceAlreadyExists)
}

// TestDeleteInstanceData asserts the happy path of DeleteMachineCloudInstance
// at the state layer.
func (s *stateSuite) TestDeleteInstanceData(c *tc.C) {
	db := s.DB()

	machineUUID := s.ensureInstance(c, "42")

	err := s.state.DeleteMachineCloudInstance(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Check that all rows've been deleted.
	rows, err := db.QueryContext(c.Context(), "SELECT * FROM machine_cloud_instance WHERE instance_id='1'")
	defer func() { _ = rows.Close() }()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rows.Err(), tc.ErrorIsNil)
	c.Check(rows.Next(), tc.IsFalse)
	rows, err = db.QueryContext(c.Context(), "SELECT * FROM instance_tag WHERE machine_uuid='"+machineUUID.String()+"'")
	defer func() { _ = rows.Close() }()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rows.Err(), tc.ErrorIsNil)
	c.Check(rows.Next(), tc.IsFalse)
}

func (s *stateSuite) TestInstanceIdSuccess(c *tc.C) {
	machineUUID := s.ensureInstance(c, "666")

	instanceId, err := s.state.GetInstanceID(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceId, tc.Equals, "123")
}

func (s *stateSuite) TestInstanceIdError(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "", nil)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.state.GetInstanceID(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
}

func (s *stateSuite) TestInstanceNameSuccess(c *tc.C) {
	machineUUID := s.ensureInstance(c, "666")

	instanceID, displayName, err := s.state.GetInstanceIDAndName(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instanceID, tc.Equals, "123")
	c.Assert(displayName, tc.Equals, "one-two-three")
}

func (s *stateSuite) TestInstanceNameError(c *tc.C) {
	err := s.state.CreateMachine(c.Context(), "666", "", "", nil)
	c.Assert(err, tc.ErrorIsNil)

	_, _, err = s.state.GetInstanceIDAndName(c.Context(), "666")
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
}

func (s *stateSuite) ensureInstance(c *tc.C, mName machine.Name) machine.UUID {
	db := s.DB()

	// Create a reference machine.
	machineUUID := coremachinetesting.GenUUID(c)
	err := s.state.CreateMachine(c.Context(), mName, "", machineUUID, nil)
	c.Assert(err, tc.ErrorIsNil)
	// Add a reference AZ.
	_, err = db.ExecContext(c.Context(), "INSERT INTO availability_zone VALUES('deadbeef-0bad-400d-8000-4b1d0d06f00d', 'az-1')")
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		c.Context(),
		machineUUID,
		instance.Id("123"),
		"one-two-three",
		"nonce",
		&instance.HardwareCharacteristics{
			Arch:             ptr("arm64"),
			Mem:              ptr[uint64](1024),
			RootDisk:         ptr[uint64](256),
			RootDiskSource:   ptr("/test"),
			CpuCores:         ptr[uint64](4),
			CpuPower:         ptr[uint64](75),
			Tags:             ptr([]string{"tag1", "tag2"}),
			AvailabilityZone: ptr("az-1"),
			VirtType:         ptr("virtual-machine"),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
}
