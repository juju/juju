// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/blockdevice"
	schematesting "github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/testing"
)

type stateSuite struct {
	schematesting.ModelSuite

	state *State
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), jujutesting.NewCheckLogger(c))
}

func (s *stateSuite) TestCreateMachine(c *gc.C) {
	err := s.state.UpsertMachine(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	var (
		machineID string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT machine_id FROM machine").Scan(&machineID)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineID, gc.Equals, "666")
}

func (s *stateSuite) TestUpdateMachine(c *gc.C) {
	err := s.state.UpsertMachine(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.UpsertMachine(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	var machineID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT machine_id FROM machine").Scan(&machineID)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineID, gc.Equals, "666")
}

func (s *stateSuite) TestDeleteMachine(c *gc.C) {
	err := s.state.UpsertMachine(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	bd := blockdevice.BlockDevice{
		DeviceName:     "name-666",
		Label:          "label-666",
		UUID:           "device-666",
		HardwareId:     "hardware-666",
		WWN:            "wwn-666",
		BusAddress:     "bus-666",
		SizeMiB:        666,
		FilesystemType: "btrfs",
		InUse:          true,
		MountPoint:     "mount-666",
		SerialId:       "serial-666",
	}
	bdUUID := utils.MustNewUUID().String()
	s.insertBlockDevice(c, bd, bdUUID, "666")

	err = s.state.DeleteMachine(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)

	var machineCount int
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT count(*) FROM machine WHERE machine_id=?", "666").Scan(&machineCount)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineCount, gc.Equals, 0)
}

func (s *stateSuite) insertBlockDevice(c *gc.C, bd blockdevice.BlockDevice, blockDeviceUUID, machineId string) {
	db := s.DB()

	inUse := 0
	if bd.InUse {
		inUse = 1
	}
	_, err := db.ExecContext(context.Background(), `
INSERT INTO block_device (uuid, name, label, device_uuid, hardware_id, wwn, bus_address, serial_id, mount_point, filesystem_type_id, Size_mib, in_use, machine_uuid)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 2, ?, ?, (SELECT uuid FROM machine WHERE machine_id=?))
`, blockDeviceUUID, bd.DeviceName, bd.Label, bd.UUID, bd.HardwareId, bd.WWN, bd.BusAddress, bd.SerialId, bd.MountPoint, bd.SizeMiB, inUse, machineId)
	c.Assert(err, jc.ErrorIsNil)

	for _, link := range bd.DeviceLinks {
		_, err = db.ExecContext(context.Background(), `
INSERT INTO block_device_link_device (block_device_uuid, name)
VALUES (?, ?)
`, blockDeviceUUID, link)
		c.Assert(err, jc.ErrorIsNil)
	}
	c.Assert(err, jc.ErrorIsNil)
}
