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

// BlockDevice copies all v4_0_4 fields and sets ProvenanceID to 0.
// The provenance_id column was added in 4.0.6, so rows imported from
// a 4.0.4 model have no provenance information.
func (d deltas) BlockDevice(_ context.Context, src []v4_0_4.BlockDevice) ([]v4_0_6.BlockDevice, error) {
	result := make([]v4_0_6.BlockDevice, len(src))
	for i, b := range src {
		result[i] = v4_0_6.BlockDevice{
			UUID:               b.UUID,
			MachineUUID:        b.MachineUUID,
			Name:               b.Name,
			HardwareID:         b.HardwareID,
			Wwn:                b.Wwn,
			SerialID:           b.SerialID,
			BusAddress:         b.BusAddress,
			SizeMib:            b.SizeMib,
			MountPoint:         b.MountPoint,
			InUse:              b.InUse,
			FilesystemLabel:    b.FilesystemLabel,
			HostFilesystemUUID: b.HostFilesystemUUID,
			FilesystemType:     b.FilesystemType,
		}
	}
	return result, nil
}

// BlockDeviceProvenance returns no rows for 4.0.4 payloads. The source
// schema has no block device provenance table.
func (d deltas) BlockDeviceProvenance(_ context.Context, _ *v4_0_4.ModelExport) ([]v4_0_6.BlockDeviceProvenance, error) {
	return nil, nil
}
