// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package to_v4_1_0

import (
	"context"

	"github.com/juju/juju/domain/export/types/v4_0_12"
	"github.com/juju/juju/domain/export/types/v4_1_0"
)

// deltas is the engineer-owned implementation of the Deltas interface
// declared in transform.go. When Deltas has methods, add receivers on
// this type or the package will not compile.
type deltas struct{}

var _ Deltas = deltas{}

// NewDeltas returns the engineer-written delta implementation for the
// 4.0.12 -> 4.1.0 transform.
func NewDeltas() Deltas { return deltas{} }

// Constraint copies all v4_0_12 fields and leaves IpFamily nil. Constraints
// exported from a 4.0.12 model carry no IP family information.
func (d deltas) Constraint(_ context.Context, src []v4_0_12.Constraint) ([]v4_1_0.Constraint, error) {
	result := make([]v4_1_0.Constraint, len(src))
	for i, c := range src {
		result[i] = v4_1_0.Constraint{
			UUID:             c.UUID,
			Arch:             c.Arch,
			CpuCores:         c.CpuCores,
			CpuPower:         c.CpuPower,
			Mem:              c.Mem,
			RootDisk:         c.RootDisk,
			RootDiskSource:   c.RootDiskSource,
			InstanceRole:     c.InstanceRole,
			InstanceType:     c.InstanceType,
			ContainerTypeID:  c.ContainerTypeID,
			VirtType:         c.VirtType,
			AllocatePublicIp: c.AllocatePublicIp,
			ImageID:          c.ImageID,
		}
	}
	return result, nil
}

// RelationApplicationSetting copies all v4_0_12 fields into the 4.1.0 schema,
// where the relation_application_setting.value column is NOT NULL and disallows
// the empty string. A 4.0.12 row whose value is NULL (or empty) has no valid
// representation in 4.1.0, so such rows are dropped rather than coerced to "".
// The result is therefore of variable length.
func (d deltas) RelationApplicationSetting(_ context.Context, src []v4_0_12.RelationApplicationSetting) ([]v4_1_0.
	RelationApplicationSetting, error) {
	result := make([]v4_1_0.RelationApplicationSetting, 0, len(src))
	for _, s := range src {
		if s.Value == nil || *s.Value == "" {
			continue
		}
		result = append(result, v4_1_0.RelationApplicationSetting{
			RelationEndpointUUID: s.RelationEndpointUUID,
			Key:                  s.Key,
			Value:                *s.Value,
		})
	}
	return result, nil
}

// RelationUnitSetting copies all v4_0_12 fields into the 4.1.0 schema, where the
// relation_unit_setting.value column is NOT NULL and disallows the empty string.
// A 4.0.12 row whose value is NULL (or empty) has no valid representation in
// 4.1.0, so such rows are dropped rather than coerced to "". The result is
// therefore of variable length.
func (d deltas) RelationUnitSetting(_ context.Context, src []v4_0_12.RelationUnitSetting) ([]v4_1_0.
	RelationUnitSetting, error) {
	result := make([]v4_1_0.RelationUnitSetting, 0, len(src))
	for _, s := range src {
		if s.Value == nil || *s.Value == "" {
			continue
		}
		result = append(result, v4_1_0.RelationUnitSetting{
			RelationUnitUUID: s.RelationUnitUUID,
			Key:              s.Key,
			Value:            *s.Value,
		})
	}
	return result, nil
}

// MachineVirtualSshHostKey returns no rows for 4.0.12 payloads. The source
// schema has no machine virtual SSH host key table.
func (d deltas) MachineVirtualSshHostKey(_ context.Context,
	_ *v4_0_12.ModelExport) ([]v4_1_0.MachineVirtualSshHostKey, error) {
	return nil, nil
}

// SshKeyAlgorithmType synthesises the static lookup table introduced in
// 4.1.0. The table is schema-owned data, so it is produced unconditionally.
func (d deltas) SshKeyAlgorithmType(_ context.Context, _ *v4_0_12.ModelExport) ([]v4_1_0.SshKeyAlgorithmType, error) {
	rsa, ecdsa, ed25519 := int64(0), int64(1), int64(2)
	return []v4_1_0.SshKeyAlgorithmType{
		{ID: &rsa, Type: "ssh-rsa"},
		{ID: &ecdsa, Type: "ecdsa-sha2-nistp256"},
		{ID: &ed25519, Type: "ssh-ed25519"},
	}, nil
}

// UnitVirtualSshHostKey returns no rows for 4.0.12 payloads. The source schema
// has no unit virtual SSH host key table.
func (d deltas) UnitVirtualSshHostKey(_ context.Context, _ *v4_0_12.ModelExport) ([]v4_1_0.UnitVirtualSshHostKey,
	error) {
	return nil, nil
}

// SshConnectionRequest returns no rows for 4.0.12 payloads. The source schema
// has no SSH connection request table.
func (d deltas) SshConnectionRequest(_ context.Context, _ *v4_0_12.ModelExport) ([]v4_1_0.SshConnectionRequest, error) {
	return nil, nil
}

// SshConnectionRequestAddress returns no rows for 4.0.12 payloads. The source schema
// has no SSH connection request address table.
func (d deltas) SshConnectionRequestAddress(_ context.Context,
	_ *v4_0_12.ModelExport) ([]v4_1_0.SshConnectionRequestAddress, error) {
	return nil, nil
}
