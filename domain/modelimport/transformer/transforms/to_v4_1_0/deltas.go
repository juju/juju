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

// parseID converts a 4.0.12 TEXT identifier column to the INTEGER
// representation used in 4.1.0. A non-numeric value indicates corruption,
// so the transform halts rather than dropping the row.
func parseID(column, value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, errors.Errorf("invalid %s %q: %w", column, value, err)
	}
	return id, nil
}

// parseOptionalID is the nullable form of parseID.
func parseOptionalID(column string, value *string) (*int64, error) {
	if value == nil {
		return nil, nil
	}
	id, err := parseID(column, *value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// ApplicationAgent parses the password_hash_algorithm_id column, which
// changed from TEXT to INTEGER in 4.1.0.
func (d deltas) ApplicationAgent(_ context.Context, src []v4_0_12.ApplicationAgent, _ *v4_0_12.ModelExport) ([]v4_1_0.ApplicationAgent, error) {
	result := make([]v4_1_0.ApplicationAgent, len(src))
	for i, a := range src {
		algorithm, err := parseOptionalID("password_hash_algorithm_id", a.PasswordHashAlgorithmID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.ApplicationAgent{
			ApplicationUUID:         a.ApplicationUUID,
			PasswordHashAlgorithmID: algorithm,
			PasswordHash:            a.PasswordHash,
		}
	}
	return result, nil
}

// ApplicationPlatform parses the os_id column, which changed from TEXT to
// INTEGER in 4.1.0.
func (d deltas) ApplicationPlatform(_ context.Context, src []v4_0_12.ApplicationPlatform, _ *v4_0_12.ModelExport) ([]v4_1_0.ApplicationPlatform, error) {
	result := make([]v4_1_0.ApplicationPlatform, len(src))
	for i, p := range src {
		os, err := parseID("os_id", p.OsID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.ApplicationPlatform{
			ApplicationUUID: p.ApplicationUUID,
			OsID:            os,
			Channel:         p.Channel,
			ArchitectureID:  p.ArchitectureID,
		}
	}
	return result, nil
}

// CharmConfig parses the type_id column, which changed from TEXT to INTEGER
// in 4.1.0.
func (d deltas) CharmConfig(_ context.Context, src []v4_0_12.CharmConfig, _ *v4_0_12.ModelExport) ([]v4_1_0.CharmConfig, error) {
	result := make([]v4_1_0.CharmConfig, len(src))
	for i, c := range src {
		typeID, err := parseOptionalID("type_id", c.TypeID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.CharmConfig{
			CharmUUID:    c.CharmUUID,
			Key:          c.Key,
			TypeID:       typeID,
			DefaultValue: c.DefaultValue,
			Description:  c.Description,
		}
	}
	return result, nil
}

// Machine parses the password_hash_algorithm_id column, which changed from
// TEXT to INTEGER in 4.1.0.
func (d deltas) Machine(_ context.Context, src []v4_0_12.Machine, _ *v4_0_12.ModelExport) ([]v4_1_0.Machine, error) {
	result := make([]v4_1_0.Machine, len(src))
	for i, m := range src {
		algorithm, err := parseOptionalID("password_hash_algorithm_id", m.PasswordHashAlgorithmID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.Machine{
			UUID:                    m.UUID,
			Name:                    m.Name,
			NetNodeUUID:             m.NetNodeUUID,
			LifeID:                  m.LifeID,
			Nonce:                   m.Nonce,
			PasswordHashAlgorithmID: algorithm,
			PasswordHash:            m.PasswordHash,
			ForceDestroyed:          m.ForceDestroyed,
			AgentStartedAt:          m.AgentStartedAt,
			Hostname:                m.Hostname,
			KeepInstance:            m.KeepInstance,
		}
	}
	return result, nil
}

// MachinePlatform parses the os_id column, which changed from TEXT to
// INTEGER in 4.1.0.
func (d deltas) MachinePlatform(_ context.Context, src []v4_0_12.MachinePlatform, _ *v4_0_12.ModelExport) ([]v4_1_0.MachinePlatform, error) {
	result := make([]v4_1_0.MachinePlatform, len(src))
	for i, p := range src {
		os, err := parseID("os_id", p.OsID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.MachinePlatform{
			MachineUUID:    p.MachineUUID,
			OsID:           os,
			Channel:        p.Channel,
			ArchitectureID: p.ArchitectureID,
		}
	}
	return result, nil
}

// ModelLife parses the life_id column, which changed from TEXT to INTEGER in
// 4.1.0.
func (d deltas) ModelLife(_ context.Context, src []v4_0_12.ModelLife, _ *v4_0_12.ModelExport) ([]v4_1_0.ModelLife, error) {
	result := make([]v4_1_0.ModelLife, len(src))
	for i, l := range src {
		life, err := parseID("life_id", l.LifeID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.ModelLife{ModelUUID: l.ModelUUID, LifeID: life}
	}
	return result, nil
}

// SecretPermission parses the scope_type_id column, which changed from TEXT
// to INTEGER in 4.1.0.
func (d deltas) SecretPermission(_ context.Context, src []v4_0_12.SecretPermission, _ *v4_0_12.ModelExport) ([]v4_1_0.SecretPermission, error) {
	result := make([]v4_1_0.SecretPermission, len(src))
	for i, p := range src {
		scope, err := parseID("scope_type_id", p.ScopeTypeID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.SecretPermission{
			SecretID:      p.SecretID,
			RoleID:        p.RoleID,
			SubjectUUID:   p.SubjectUUID,
			SubjectTypeID: p.SubjectTypeID,
			ScopeUUID:     p.ScopeUUID,
			ScopeTypeID:   scope,
		}
	}
	return result, nil
}

// Unit parses the password_hash_algorithm_id column, which changed from TEXT
// to INTEGER in 4.1.0.
func (d deltas) Unit(_ context.Context, src []v4_0_12.Unit, _ *v4_0_12.ModelExport) ([]v4_1_0.Unit, error) {
	result := make([]v4_1_0.Unit, len(src))
	for i, u := range src {
		algorithm, err := parseOptionalID("password_hash_algorithm_id", u.PasswordHashAlgorithmID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.Unit{
			UUID:                    u.UUID,
			Name:                    u.Name,
			LifeID:                  u.LifeID,
			ApplicationUUID:         u.ApplicationUUID,
			NetNodeUUID:             u.NetNodeUUID,
			CharmUUID:               u.CharmUUID,
			PasswordHashAlgorithmID: algorithm,
			PasswordHash:            u.PasswordHash,
		}
	}
	return result, nil
}
