// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
)

func (s *stateSuite) TestGetHardwareCharacteristics(c *gc.C) {
	db := s.DB()

	// Create a reference machine.
	err := s.state.CreateMachine(context.Background(), "42", "", "")
	c.Assert(err, jc.ErrorIsNil)
	var machineUUID string
	row := db.QueryRowContext(context.Background(), "SELECT uuid FROM machine WHERE machine_id='42'")
	c.Assert(row.Err(), jc.ErrorIsNil)
	err = row.Scan(&machineUUID)
	c.Assert(err, jc.ErrorIsNil)
	// Add a reference AZ.
	_, err = db.ExecContext(context.Background(), "INSERT INTO availability_zone VALUES('az-1', 'az1')")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		context.Background(),
		machineUUID,
		instance.Id("1"),
		instance.HardwareCharacteristics{
			Arch:             strptr("arm64"),
			Mem:              uintptr(1024),
			RootDisk:         uintptr(256),
			RootDiskSource:   strptr("/test"),
			CpuCores:         uintptr(4),
			CpuPower:         uintptr(75),
			Tags:             strsliceptr([]string{"tag1", "tag2"}),
			AvailabilityZone: strptr("az-1"),
			VirtType:         strptr("virtual-machine"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	hc, err := s.state.HardwareCharacteristics(context.Background(), machineUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(*hc.Arch, gc.Equals, "arm64")
	c.Check(*hc.Mem, gc.Equals, uint64(1024))
	c.Check(*hc.RootDisk, gc.Equals, uint64(256))
	c.Check(*hc.RootDiskSource, gc.Equals, "/test")
	c.Check(*hc.CpuCores, gc.Equals, uint64(4))
	c.Check(*hc.CpuPower, gc.Equals, uint64(75))
	c.Check(*hc.AvailabilityZone, gc.Equals, "az-1")
	c.Check(*hc.VirtType, gc.Equals, "virtual-machine")
}

func (s *stateSuite) TestSetInstanceData(c *gc.C) {
	db := s.DB()

	// Create a reference machine.
	err := s.state.CreateMachine(context.Background(), "42", "", "")
	c.Assert(err, jc.ErrorIsNil)
	var machineUUID string
	row := db.QueryRowContext(context.Background(), "SELECT uuid FROM machine WHERE machine_id='42'")
	c.Assert(row.Err(), jc.ErrorIsNil)
	err = row.Scan(&machineUUID)
	c.Assert(err, jc.ErrorIsNil)
	// Add a reference AZ.
	_, err = db.ExecContext(context.Background(), "INSERT INTO availability_zone VALUES('az-1', 'az1')")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		context.Background(),
		machineUUID,
		instance.Id("1"),
		instance.HardwareCharacteristics{
			Arch:             strptr("arm64"),
			Mem:              uintptr(1024),
			RootDisk:         uintptr(256),
			CpuCores:         uintptr(4),
			CpuPower:         uintptr(75),
			Tags:             strsliceptr([]string{"tag1", "tag2"}),
			AvailabilityZone: strptr("az-1"),
			VirtType:         strptr("virtual-machine"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	var instanceData instanceData
	row = db.QueryRowContext(context.Background(), "SELECT * FROM machine_cloud_instance WHERE instance_id='1'")
	c.Assert(row.Err(), jc.ErrorIsNil)
	err = row.Scan(
		&instanceData.MachineUUID,
		&instanceData.InstanceID,
		&instanceData.Arch,
		&instanceData.Mem,
		&instanceData.RootDisk,
		&instanceData.RootDiskSource,
		&instanceData.CPUCores,
		&instanceData.CPUPower,
		&instanceData.AvailabilityZoneUUID,
		&instanceData.VirtType,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(instanceData.MachineUUID, gc.Equals, machineUUID)
	c.Check(instanceData.InstanceID, gc.Equals, "1")
	c.Check(*instanceData.Arch, gc.Equals, "arm64")
	c.Check(*instanceData.Mem, gc.Equals, uint64(1024))
	c.Check(*instanceData.RootDisk, gc.Equals, uint64(256))
	// Make sure we also handle correctly NULL values.
	c.Check(instanceData.RootDiskSource, gc.IsNil)
	c.Check(*instanceData.CPUCores, gc.Equals, uint64(4))
	c.Check(*instanceData.CPUPower, gc.Equals, uint64(75))
	c.Check(*instanceData.AvailabilityZoneUUID, gc.Equals, "az-1")
	c.Check(*instanceData.VirtType, gc.Equals, "virtual-machine")

	rows, err := db.QueryContext(context.Background(), "SELECT tag FROM instance_tag WHERE machine_uuid='"+machineUUID+"'")
	defer rows.Close()
	c.Assert(err, jc.ErrorIsNil)
	var instanceTags []string
	for rows.Next() {
		var tag string
		err = rows.Scan(&tag)
		c.Assert(err, jc.ErrorIsNil)
		instanceTags = append(instanceTags, tag)
	}
	c.Check(instanceTags, gc.HasLen, 2)
	c.Check(instanceTags[0], gc.Equals, "tag1")
	c.Check(instanceTags[1], gc.Equals, "tag2")
}

func (s *stateSuite) TestDeleteInstanceData(c *gc.C) {
	db := s.DB()

	// Create a reference machine.
	err := s.state.CreateMachine(context.Background(), "42", "", "")
	c.Assert(err, jc.ErrorIsNil)
	var machineUUID string
	row := db.QueryRowContext(context.Background(), "SELECT uuid FROM machine WHERE machine_id='42'")
	c.Assert(row.Err(), jc.ErrorIsNil)
	err = row.Scan(&machineUUID)
	c.Assert(err, jc.ErrorIsNil)
	// Add a reference AZ.
	_, err = db.ExecContext(context.Background(), "INSERT INTO availability_zone VALUES('az-1', 'az1')")
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetMachineCloudInstance(
		context.Background(),
		machineUUID,
		instance.Id("1"),
		instance.HardwareCharacteristics{
			Arch:             strptr("arm64"),
			Mem:              uintptr(1024),
			RootDisk:         uintptr(256),
			RootDiskSource:   strptr("/test"),
			CpuCores:         uintptr(4),
			CpuPower:         uintptr(75),
			Tags:             strsliceptr([]string{"tag1", "tag2"}),
			AvailabilityZone: strptr("az-1"),
			VirtType:         strptr("virtual-machine"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.DeleteMachineCloudInstance(context.Background(), machineUUID)
	c.Assert(err, jc.ErrorIsNil)

	// Check that all rows've been deleted.
	rows, err := db.QueryContext(context.Background(), "SELECT * FROM machine_cloud_instance WHERE instance_id='1'")
	defer rows.Close()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rows.Err(), jc.ErrorIsNil)
	c.Check(rows.Next(), jc.IsFalse)
	rows, err = db.QueryContext(context.Background(), "SELECT * FROM instance_tag WHERE machine_uuid='"+machineUUID+"'")
	defer rows.Close()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rows.Err(), jc.ErrorIsNil)
	c.Check(rows.Next(), jc.IsFalse)
}

func strptr(s string) *string {
	return &s
}

func uintptr(u uint64) *uint64 {
	return &u
}

func strsliceptr(s []string) *[]string {
	return &s
}
