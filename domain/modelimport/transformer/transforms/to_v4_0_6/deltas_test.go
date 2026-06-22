// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package to_v4_0_6

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/export/types/v4_0_4"
	"github.com/juju/juju/domain/export/types/v4_0_6"
)

type deltasSuite struct{}

func TestDeltasSuite(t *testing.T) {
	tc.Run(t, &deltasSuite{})
}

// --- BlockDevice ---

func (s *deltasSuite) TestBlockDeviceEmptyInput(c *tc.C) {
	d := NewDeltas()
	got, err := d.BlockDevice(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got, tc.HasLen, 0)
}

func (s *deltasSuite) TestBlockDeviceCopiesAllFields(c *tc.C) {
	src := []v4_0_4.BlockDevice{{
		UUID:               "bd-uuid",
		MachineUUID:        "machine-uuid",
		Name:               new("sda"),
		HardwareID:         new("hw-id"),
		Wwn:                new("wwn-value"),
		SerialID:           new("serial-1"),
		BusAddress:         new("pci:0000"),
		SizeMib:            new(int64(100)),
		MountPoint:         new("/mnt/data"),
		InUse:              new(true),
		FilesystemLabel:    new("data"),
		HostFilesystemUUID: new("fs-uuid"),
		FilesystemType:     new("ext4"),
	}}

	d := NewDeltas()
	got, err := d.BlockDevice(c.Context(), src)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 1)

	bd := got[0]
	c.Check(bd.UUID, tc.Equals, "bd-uuid")
	c.Check(bd.MachineUUID, tc.Equals, "machine-uuid")
	c.Check(bd.Name, tc.DeepEquals, new("sda"))
	c.Check(bd.HardwareID, tc.DeepEquals, new("hw-id"))
	c.Check(bd.Wwn, tc.DeepEquals, new("wwn-value"))
	c.Check(bd.SerialID, tc.DeepEquals, new("serial-1"))
	c.Check(bd.BusAddress, tc.DeepEquals, new("pci:0000"))
	c.Check(bd.SizeMib, tc.DeepEquals, new(int64(100)))
	c.Check(bd.MountPoint, tc.DeepEquals, new("/mnt/data"))
	c.Check(bd.InUse, tc.DeepEquals, new(true))
	c.Check(bd.FilesystemLabel, tc.DeepEquals, new("data"))
	c.Check(bd.HostFilesystemUUID, tc.DeepEquals, new("fs-uuid"))
	c.Check(bd.FilesystemType, tc.DeepEquals, new("ext4"))
}

func (s *deltasSuite) TestBlockDeviceProvenanceIDIsAlwaysZero(c *tc.C) {
	src := []v4_0_4.BlockDevice{
		{UUID: "bd-1", MachineUUID: "m-uuid"},
		{UUID: "bd-2", MachineUUID: "m-uuid", Name: new("vdb")},
	}

	d := NewDeltas()
	got, err := d.BlockDevice(c.Context(), src)
	c.Assert(err, tc.ErrorIsNil)
	for _, bd := range got {
		c.Check(bd.ProvenanceID, tc.Equals, int64(0),
			tc.Commentf("uuid=%s", bd.UUID))
	}
}

func (s *deltasSuite) TestBlockDeviceNilOptionalFieldsPreserved(c *tc.C) {
	src := []v4_0_4.BlockDevice{{UUID: "bd-min", MachineUUID: "m-uuid"}}

	d := NewDeltas()
	got, err := d.BlockDevice(c.Context(), src)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 1)

	bd := got[0]
	c.Check(bd.Name, tc.IsNil)
	c.Check(bd.HardwareID, tc.IsNil)
	c.Check(bd.Wwn, tc.IsNil)
	c.Check(bd.SerialID, tc.IsNil)
	c.Check(bd.BusAddress, tc.IsNil)
	c.Check(bd.SizeMib, tc.IsNil)
	c.Check(bd.MountPoint, tc.IsNil)
	c.Check(bd.InUse, tc.IsNil)
	c.Check(bd.FilesystemLabel, tc.IsNil)
	c.Check(bd.HostFilesystemUUID, tc.IsNil)
	c.Check(bd.FilesystemType, tc.IsNil)
}

func (s *deltasSuite) TestBlockDevicePreservesOrder(c *tc.C) {
	src := []v4_0_4.BlockDevice{
		{UUID: "uuid-a", MachineUUID: "m-1"},
		{UUID: "uuid-b", MachineUUID: "m-1"},
		{UUID: "uuid-c", MachineUUID: "m-2"},
	}

	d := NewDeltas()
	got, err := d.BlockDevice(c.Context(), src)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 3)
	c.Check(got[0].UUID, tc.Equals, "uuid-a")
	c.Check(got[1].UUID, tc.Equals, "uuid-b")
	c.Check(got[2].UUID, tc.Equals, "uuid-c")
}

// --- BlockDeviceProvenance ---

func (s *deltasSuite) TestBlockDeviceProvenanceReturnsBothStaticRows(c *tc.C) {
	d := NewDeltas()
	got, err := d.BlockDeviceProvenance(c.Context(), &v4_0_4.ModelExport{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 2)
	c.Check(got[0], tc.DeepEquals, v4_0_6.BlockDeviceProvenance{ID: new(int64(0)), Value: "provider"})
	c.Check(got[1], tc.DeepEquals, v4_0_6.BlockDeviceProvenance{ID: new(int64(1)), Value: "machine"})
}

func (s *deltasSuite) TestBlockDeviceProvenanceIsStaticRegardlessOfSrc(c *tc.C) {
	d := NewDeltas()

	// nil src
	got1, err := d.BlockDeviceProvenance(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)

	// non-empty src
	got2, err := d.BlockDeviceProvenance(c.Context(), &v4_0_4.ModelExport{
		BlockDevice: []v4_0_4.BlockDevice{
			{UUID: "bd-x", MachineUUID: "m-uuid"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(got1, tc.DeepEquals, got2)
}

// --- Integration via NewTransform ---

func (s *deltasSuite) TestNewTransformAppliesBothDeltas(c *tc.C) {
	transform := NewTransform(NewDeltas())

	src := v4_0_4.ModelExport{
		BlockDevice: []v4_0_4.BlockDevice{
			{UUID: "bd-uuid", MachineUUID: "m-uuid", Name: new("vda"), SizeMib: new(int64(512))},
		},
	}

	dst, err := transform(c.Context(), src)
	c.Assert(err, tc.ErrorIsNil)

	// BlockDevice: field copied, ProvenanceID defaulted to 0.
	c.Assert(dst.BlockDevice, tc.HasLen, 1)
	c.Check(dst.BlockDevice[0].UUID, tc.Equals, "bd-uuid")
	c.Check(dst.BlockDevice[0].Name, tc.DeepEquals, new("vda"))
	c.Check(dst.BlockDevice[0].SizeMib, tc.DeepEquals, new(int64(512)))
	c.Check(dst.BlockDevice[0].ProvenanceID, tc.Equals, int64(0))

	// BlockDeviceProvenance: the two static lookup rows.
	c.Assert(dst.BlockDeviceProvenance, tc.HasLen, 2)
	c.Check(dst.BlockDeviceProvenance[0].Value, tc.Equals, "provider")
	c.Check(dst.BlockDeviceProvenance[1].Value, tc.Equals, "machine")
}

func (s *deltasSuite) TestNewTransformEmptyBlockDevices(c *tc.C) {
	transform := NewTransform(NewDeltas())

	dst, err := transform(c.Context(), v4_0_4.ModelExport{})
	c.Assert(err, tc.ErrorIsNil)

	// Empty source → empty result; provenance lookup still synthesised.
	c.Check(dst.BlockDevice, tc.HasLen, 0)
	c.Assert(dst.BlockDeviceProvenance, tc.HasLen, 2)
}
