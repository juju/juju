// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package blockdevice

import (
	"reflect"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/blockdevice"
)

type comparisonSuite struct{}

func TestComparisonSuite(t *testing.T) {
	tc.Run(t, &comparisonSuite{})
}

func (s *comparisonSuite) TestSameDeviceByName(c *tc.C) {
	left := blockdevice.BlockDevice{
		DeviceName: "a",
	}
	right := blockdevice.BlockDevice{
		DeviceName: "a",
	}
	res := SameDevice(left, right)
	c.Assert(res, tc.IsTrue)
}

func (s *comparisonSuite) TestSameDeviceByNameMismatch(c *tc.C) {
	left := blockdevice.BlockDevice{
		DeviceName: "a",
	}
	right := blockdevice.BlockDevice{
		DeviceName: "b",
	}
	res := SameDevice(left, right)
	c.Assert(res, tc.IsFalse)
}

func (s *comparisonSuite) TestSameDeviceByDevLink(c *tc.C) {
	left := blockdevice.BlockDevice{
		DeviceLinks: []string{
			"/something/else",
			"/dev/disk/by-id/xyz",
		},
	}
	right := blockdevice.BlockDevice{
		DeviceLinks: []string{
			"/one/thing",
			"/dev/disk/by-id/xy",
			"/dev/disk/by-id/xyz",
		},
	}
	res := SameDevice(left, right)
	c.Assert(res, tc.IsTrue)
}

func (s *comparisonSuite) TestSameDeviceByDevLinkMismatch(c *tc.C) {
	left := blockdevice.BlockDevice{
		DeviceLinks: []string{
			"/something/else",
			"/dev/disk/by-id/xyz",
		},
	}
	right := blockdevice.BlockDevice{
		DeviceLinks: []string{
			"/one/thing",
			"/dev/disk/by-id/xy",
		},
	}
	res := SameDevice(left, right)
	c.Assert(res, tc.IsFalse)
}

func (s *comparisonSuite) TestSameDeviceByWWN(c *tc.C) {
	left := blockdevice.BlockDevice{
		WWN: "abc",
	}
	right := blockdevice.BlockDevice{
		WWN: "abc",
	}
	res := SameDevice(left, right)
	c.Assert(res, tc.IsTrue)
}

func (s *comparisonSuite) TestSameDeviceByHardwareId(c *tc.C) {
	left := blockdevice.BlockDevice{
		HardwareId: "abc",
	}
	right := blockdevice.BlockDevice{
		HardwareId: "abc",
	}
	res := SameDevice(left, right)
	c.Assert(res, tc.IsTrue)
}

func (s *comparisonSuite) TestSameDeviceBySerialId(c *tc.C) {
	left := blockdevice.BlockDevice{
		SerialId: "abc",
	}
	right := blockdevice.BlockDevice{
		SerialId: "abc",
	}
	res := SameDevice(left, right)
	c.Assert(res, tc.IsTrue)
}

func (s *comparisonSuite) TestSameDeviceByBusAddress(c *tc.C) {
	left := blockdevice.BlockDevice{
		BusAddress: "abc",
	}
	right := blockdevice.BlockDevice{
		BusAddress: "abc",
	}
	res := SameDevice(left, right)
	c.Assert(res, tc.IsTrue)
}

func (s *comparisonSuite) TestSameDeviceByWWNMismatchPartition(c *tc.C) {
	left := blockdevice.BlockDevice{
		WWN: "abc",
		DeviceLinks: []string{
			"/dev/disk/by-partlabel/foo",
		},
	}
	right := blockdevice.BlockDevice{
		WWN: "abc",
	}
	res := SameDevice(left, right)
	c.Assert(res, tc.IsFalse)
}

func (s *comparisonSuite) TestSameDeviceByHardwareIdMismatchPartition(c *tc.C) {
	left := blockdevice.BlockDevice{
		HardwareId: "abc",
		DeviceLinks: []string{
			"/dev/disk/by-partlabel/foo",
		},
	}
	right := blockdevice.BlockDevice{
		HardwareId: "abc",
	}
	res := SameDevice(left, right)
	c.Assert(res, tc.IsFalse)
}

func (s *comparisonSuite) TestSameDeviceBySerialIdMismatchPartition(c *tc.C) {
	left := blockdevice.BlockDevice{
		SerialId: "abc",
	}
	right := blockdevice.BlockDevice{
		SerialId: "abc",
		DeviceLinks: []string{
			"/dev/disk/by-partlabel/foo",
		},
	}
	res := SameDevice(left, right)
	c.Assert(res, tc.IsFalse)
}

func (s *comparisonSuite) TestSameDeviceByBusAddressMismatchPartition(c *tc.C) {
	left := blockdevice.BlockDevice{
		BusAddress: "abc",
		DeviceLinks: []string{
			"/dev/disk/by-partlabel/foo",
		},
	}
	right := blockdevice.BlockDevice{
		BusAddress: "abc",
	}
	res := SameDevice(left, right)
	c.Assert(res, tc.IsFalse)
}

func (s *comparisonSuite) TestIsPartition(c *tc.C) {
	res := IsPartition(blockdevice.BlockDevice{
		DeviceLinks: []string{
			"/dev/disk/by-partuuid/abc",
		},
	})
	c.Check(res, tc.IsTrue)

	res = IsPartition(blockdevice.BlockDevice{
		DeviceLinks: []string{
			"/dev/disk/by-partlabel/abc",
		},
	})
	c.Check(res, tc.IsTrue)

	res = IsPartition(blockdevice.BlockDevice{
		DeviceLinks: []string{
			"/dev/disk/by-uuid/abc",
		},
	})
	c.Check(res, tc.IsTrue)

	res = IsPartition(blockdevice.BlockDevice{
		DeviceLinks: []string{
			"/dev/disk/by-id/abc",
			"/dev/disk/by-uuid/abc",
		},
	})
	c.Check(res, tc.IsTrue)

	res = IsPartition(blockdevice.BlockDevice{
		DeviceLinks: []string{
			"/dev/disk/by-id/abc",
		},
	})
	c.Check(res, tc.IsFalse)
}

func (s *comparisonSuite) TestIsNameOnly(c *tc.C) {
	res := IsNameOnly(blockdevice.BlockDevice{
		DeviceName: "a",
	})
	c.Check(res, tc.IsTrue)

	res = IsNameOnly(blockdevice.BlockDevice{})
	c.Check(res, tc.IsFalse)

	res = IsNameOnly(blockdevice.BlockDevice{
		DeviceName:      "a",
		FilesystemLabel: "b",
	})
	c.Check(res, tc.IsFalse)
}

func (s *comparisonSuite) TestIsEmpty(c *tc.C) {
	res := IsEmpty(blockdevice.BlockDevice{})
	c.Check(res, tc.IsTrue)
	res = IsEmpty(blockdevice.BlockDevice{
		DeviceLinks: []string{},
	})
	c.Check(res, tc.IsTrue)

	examples := map[string]blockdevice.BlockDevice{
		"DeviceName":      {DeviceName: "a"},
		"DeviceLinks":     {DeviceLinks: []string{"a"}},
		"FilesystemLabel": {FilesystemLabel: "a"},
		"FilesystemUUID":  {FilesystemUUID: "a"},
		"HardwareId":      {HardwareId: "a"},
		"WWN":             {WWN: "a"},
		"BusAddress":      {BusAddress: "a"},
		"SizeMiB":         {SizeMiB: 1024},
		"FilesystemType":  {FilesystemType: "a"},
		"InUse":           {InUse: true},
		"MountPoint":      {MountPoint: "a"},
		"SerialId":        {SerialId: "a"},
	}
	t := reflect.TypeFor[blockdevice.BlockDevice]()
	c.Assert(examples, tc.HasLen, t.NumField(),
		tc.Commentf("all fields must have an example"))
	for i := range t.NumField() {
		fname := t.Field(i).Name
		example, ok := examples[fname]
		c.Assert(ok, tc.IsTrue,
			tc.Commentf("field %s missing example", fname))

		// Test the example fails the empty check.
		res := IsEmpty(example)
		c.Check(res, tc.IsFalse,
			tc.Commentf("field %s example is incorrectly empty?", fname))

		// Reset the field the example is for back to a zero value.
		reflect.ValueOf(&example).Elem().Field(i).Set(
			reflect.Zero(t.Field(i).Type))
		res = IsEmpty(example)
		c.Check(res, tc.IsTrue,
			tc.Commentf("field %s example is for the wrong field", fname))
	}
}

func (s *comparisonSuite) TestIDLink(c *tc.C) {
	devLinks := []string{
		"/dev/disk/by-diskseq/9-part2",
		"/dev/disk/by-id/nvme-KINGSTON_SKC3000S1024G_500BBBBBBBBBBBBBB-part2",
		"/dev/disk/by-path/pci-0000:04:00.0-nvme-1-part2",
		"/dev/disk/by-id/nvme-KINGSTON_SKC3000S1024G_500BBBBBBBBBBBBBB_1-part2",
		"/dev/disk/by-id/nvme-eui.000000000000000000bbbbbbbbbbbbbb-part2",
		"/dev/disk/by-partuuid/ad2e094d-7aa0-43c2-9f64-302850cf723e",
		"/dev/disk/by-uuid/5af9dc70-4af6-4885-afea-008002e622d2",
	}
	idLink := IDLink(devLinks)
	c.Assert(idLink, tc.Equals,
		"/dev/disk/by-id/nvme-eui.000000000000000000bbbbbbbbbbbbbb-part2")
}

func (s *comparisonSuite) TestIDLinkNoIDLink(c *tc.C) {
	devLinks := []string{
		"/dev/disk/by-diskseq/9-part2",
		"/dev/disk/by-path/pci-0000:04:00.0-nvme-1-part2",
		"/dev/disk/by-partuuid/ad2e094d-7aa0-43c2-9f64-302850cf723e",
		"/dev/disk/by-uuid/5af9dc70-4af6-4885-afea-008002e622d2",
	}
	idLink := IDLink(devLinks)
	c.Assert(idLink, tc.Equals, "")
}

func (s *comparisonSuite) TestSameDeviceByDevLinkAzure(c *tc.C) {
	left := blockdevice.BlockDevice{
		DeviceLinks: []string{
			"/something/else",
			"/dev/disk/azure/scsi1/lun0",
		},
	}
	right := blockdevice.BlockDevice{
		DeviceLinks: []string{
			"/one/thing",
			"/dev/disk/by-id/xy",
			"/dev/disk/by-id/xyz",
			"/dev/disk/azure/scsi1/lun0",
		},
	}
	res := SameDevice(left, right)
	c.Assert(res, tc.IsTrue)
}
