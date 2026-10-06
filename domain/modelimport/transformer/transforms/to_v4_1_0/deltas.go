// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package to_v4_1_0

import (
	"context"
	"strconv"

	"github.com/juju/juju/domain/export/types/v4_0_12"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/internal/errors"
)

// deltas is the engineer-owned implementation of the Deltas interface
// declared in transform.go. When Deltas has methods, add receivers on
// this type or the package will not compile.
type deltas struct{}

var _ Deltas = deltas{}

// NewDeltas returns the engineer-written delta implementation for the
// 4.0.12 -> 4.1.0 transform.
func NewDeltas() Deltas { return deltas{} }

// Offer copies all v4_0_12 fields and leaves Description nil. Offers exported
// from a 4.0.12 model carry no offer description; on import the offer falls
// back to the charm metadata description.
func (d deltas) Offer(_ context.Context, src []v4_0_12.Offer, _ *v4_0_12.ModelExport) ([]v4_1_0.Offer, error) {
	result := make([]v4_1_0.Offer, len(src))
	for i := range src {
		result[i] = v4_1_0.Offer{
			UUID: src[i].UUID,
			Name: src[i].Name,
		}
	}
	return result, nil
}

// Operation converts v4_0_12 Operation rows to v4_1_0. The operation_id
// column changed from TEXT to INTEGER in 4.1.0; the string value is parsed
// to int64. A non-numeric operation_id indicates data corruption (the
// sequence always produces numeric values), so the transform halts with
// an error rather than silently dropping the row.
func (d deltas) Operation(_ context.Context, src []v4_0_12.Operation, _ *v4_0_12.ModelExport) ([]v4_1_0.Operation, error) {
	result := make([]v4_1_0.Operation, 0, len(src))
	for _, o := range src {
		id, err := strconv.ParseInt(o.OperationID, 10, 64)
		if err != nil {
			return nil, errors.Errorf("invalid operation ID %q: %w", o.OperationID, err)
		}
		result = append(result, v4_1_0.Operation{
			UUID:           o.UUID,
			OperationID:    id,
			Summary:        o.Summary,
			EnqueuedAt:     o.EnqueuedAt,
			StartedAt:      o.StartedAt,
			CompletedAt:    o.CompletedAt,
			Parallel:       o.Parallel,
			ExecutionGroup: o.ExecutionGroup,
		})
	}
	return result, nil
}

// Constraint copies all v4_0_12 fields and leaves IpFamily nil. Constraints
// exported from a 4.0.12 model carry no IP family information.
func (d deltas) Constraint(_ context.Context, src []v4_0_12.Constraint, _ *v4_0_12.ModelExport) ([]v4_1_0.Constraint, error) {
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
func (d deltas) RelationApplicationSetting(_ context.Context, src []v4_0_12.RelationApplicationSetting, _ *v4_0_12.ModelExport) ([]v4_1_0.
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
func (d deltas) RelationUnitSetting(_ context.Context, src []v4_0_12.RelationUnitSetting, _ *v4_0_12.ModelExport) ([]v4_1_0.
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

// ApplicationScale: struct shape changed in 4.1.0. We always default to 0,
// as this is a change in behavior.
func (d deltas) ApplicationScale(ctx context.Context, src []v4_0_12.ApplicationScale, _ *v4_0_12.ModelExport) ([]v4_1_0.ApplicationScale, error) {
	var scales []v4_1_0.ApplicationScale
	for _, s := range src {
		scales = append(scales, v4_1_0.ApplicationScale{
			ApplicationUUID: s.ApplicationUUID,
			Scale:           s.Scale,
			ScaleTarget:     s.ScaleTarget,
			Scaling:         s.Scaling,
			StartOrdinal:    0,
			EndOrdinal:      0,
		})
	}
	return scales, nil
}

// UnitResource adds the resource name used by the new logical unit-resource
// key. Legacy payloads can contain duplicate names for a unit, so prefer the
// resource belonging to the unit's charm and then the most recently added row.
// Equal timestamps are resolved by resource UUID for deterministic output.
func (d deltas) UnitResource(
	_ context.Context,
	src []v4_0_12.UnitResource,
	model *v4_0_12.ModelExport,
) ([]v4_1_0.UnitResource, error) {
	resources := make(map[string]v4_0_12.Resource, len(model.Resource))
	for _, resource := range model.Resource {
		resources[resource.UUID] = resource
	}
	unitCharms := make(map[string]string, len(model.Unit))
	for _, unit := range model.Unit {
		unitCharms[unit.UUID] = unit.CharmUUID
	}

	type unitResourceKey struct {
		unitUUID string
		name     string
	}
	indexes := make(map[unitResourceKey]int, len(src))
	result := make([]v4_1_0.UnitResource, 0, len(src))
	for _, unitResource := range src {
		resource, ok := resources[unitResource.ResourceUUID]
		if !ok {
			return nil, errors.Errorf("resource %q referenced by unit %q not found",
				unitResource.ResourceUUID, unitResource.UnitUUID)
		}
		unitCharm, ok := unitCharms[unitResource.UnitUUID]
		if !ok {
			return nil, errors.Errorf("unit %q referenced by resource %q not found",
				unitResource.UnitUUID, unitResource.ResourceUUID)
		}

		// We require the charm resource name to be present in the resource
		// table, and the only way to get it is to look up the resource by UUID.
		// If the resource is missing, we cannot determine the name and must
		// fail the transform.
		charmResourceName := resource.CharmResourceName
		if charmResourceName == "" {
			return nil, errors.Errorf("resource %q referenced by unit %q has no charm resource name",
				unitResource.ResourceUUID, unitResource.UnitUUID)
		}

		candidate := v4_1_0.UnitResource{
			ResourceUUID:      unitResource.ResourceUUID,
			UnitUUID:          unitResource.UnitUUID,
			CharmResourceName: charmResourceName,
			AddedAt:           unitResource.AddedAt,
		}
		key := unitResourceKey{
			unitUUID: unitResource.UnitUUID,
			name:     charmResourceName,
		}

		index, exists := indexes[key]
		if !exists {
			indexes[key] = len(result)
			result = append(result, candidate)
			continue
		}

		// If the unit already has a resource with the same name, prefer the one
		// that belongs to the unit's charm, and then the most recently added
		// one.
		current := result[index]
		currentResource := resources[current.ResourceUUID]
		currentMatchesCharm := currentResource.CharmUUID == unitCharm
		candidateMatchesCharm := resource.CharmUUID == unitCharm

		// If the candidate resource matches the unit's charm and the current
		// one does not, replace it. If both match or both do not match, prefer
		// the most recently added one. Equal timestamps are resolved by UUID.
		replace := candidateMatchesCharm && !currentMatchesCharm
		if candidateMatchesCharm == currentMatchesCharm {
			replace = candidate.AddedAt.After(current.AddedAt) ||
				(candidate.AddedAt.Equal(current.AddedAt) && candidate.ResourceUUID > current.ResourceUUID)
		}
		if replace {
			result[index] = candidate
		}
	}
	return result, nil
}

// MachineReprovision returns no rows for 4.0.12 payloads. The source schema has
// no machine reprovision table.
func (d deltas) MachineReprovision(_ context.Context, _ *v4_0_12.ModelExport) ([]v4_1_0.MachineReprovision, error) {
	// The machine_reprovision table was added in 4.1.0, so there are no rows to
	// transform from 4.0.12.
	return nil, nil
}

// RelationUnitDeparture returns no rows for 4.0.12 payloads. The source schema
// has no relation unit departure table.
func (d deltas) RelationUnitDeparture(_ context.Context, _ *v4_0_12.ModelExport) ([]v4_1_0.RelationUnitDeparture, error) {
	return nil, nil
}

// MachineVirtualSshHostKey returns no rows for 4.0.12 payloads. The source
// schema has no machine virtual SSH host key table.
func (d deltas) MachineVirtualSshHostKey(_ context.Context, _ *v4_0_12.ModelExport) ([]v4_1_0.MachineVirtualSshHostKey, error) {
	// The machine_virtual_ssh_host_key table was added in 4.1.0, so there
	// are no rows to transform from 4.0.12.
	return nil, nil
}

// UnitVirtualSshHostKey returns no rows for 4.0.12 payloads. The source schema
// has no unit virtual SSH host key table.
func (d deltas) UnitVirtualSshHostKey(_ context.Context, _ *v4_0_12.ModelExport) ([]v4_1_0.UnitVirtualSshHostKey, error) {
	// The unit_virtual_ssh_host_key table was added in 4.1.0, so there
	// are no rows to transform from 4.0.12.
	return nil, nil
}

// SshConnectionRequest returns no rows for 4.0.12 payloads. The source schema
// has no SSH connection request table.
func (d deltas) SshConnectionRequest(_ context.Context, _ *v4_0_12.ModelExport) ([]v4_1_0.SshConnectionRequest, error) {
	// The ssh_connection_request table was added in 4.1.0, so there are no rows
	// to transform from 4.0.12.
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
