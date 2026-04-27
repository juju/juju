// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package to_v4_0_6

import (
	"context"

	"github.com/juju/juju/domain/export/types/v4_0_4"
	"github.com/juju/juju/domain/export/types/v4_0_6"
)

// deltas is the engineer-owned implementation of the Deltas interface
// declared in transform.go. When Deltas has methods, add receivers on
// this type or the package will not compile.
type deltas struct{}

var _ Deltas = deltas{}

// NewDeltas returns the engineer-written delta implementation for the
// 4.0.4 -> 4.0.6 transform.
func NewDeltas() Deltas { return deltas{} }

// BlockDevice copies all v4_0_4 fields and sets ProvenanceID to 0 ("provider"),
// matching the DEFAULT on the block_device.provenance_id column introduced in
// 4.0.6. Block devices exported from a 4.0.4 model carry no provenance
// information; the column default keeps behaviour consistent with a straight
// controller upgrade where SQLite fills new NOT NULL columns with their
// declared DEFAULT.
func (d deltas) BlockDevice(_ context.Context, src []v4_0_4.BlockDevice) ([]v4_0_6.BlockDevice, error) {
	result := make([]v4_0_6.BlockDevice, len(src))
	for i, bd := range src {
		result[i] = v4_0_6.BlockDevice{
			UUID:               bd.UUID,
			MachineUUID:        bd.MachineUUID,
			Name:               bd.Name,
			HardwareID:         bd.HardwareID,
			Wwn:                bd.Wwn,
			SerialID:           bd.SerialID,
			BusAddress:         bd.BusAddress,
			SizeMib:            bd.SizeMib,
			MountPoint:         bd.MountPoint,
			InUse:              bd.InUse,
			FilesystemLabel:    bd.FilesystemLabel,
			HostFilesystemUUID: bd.HostFilesystemUUID,
			FilesystemType:     bd.FilesystemType,
			ProvenanceID:       0,
		}
	}
	return result, nil
}

// BlockDeviceProvenance synthesises the provenance lookup table introduced in
// 4.0.6. The table is static — it is pre-seeded by the schema migration with
// exactly two rows — so we produce the same two entries unconditionally,
// regardless of what the source payload contains.
func (d deltas) BlockDeviceProvenance(_ context.Context, _ *v4_0_4.ModelExport) ([]v4_0_6.BlockDeviceProvenance, error) {
	p0, p1 := int64(0), int64(1)
	return []v4_0_6.BlockDeviceProvenance{
		{ID: &p0, Value: "provider"},
		{ID: &p1, Value: "machine"},
	}, nil
}
