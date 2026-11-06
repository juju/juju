// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package to_v4_1_0

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/export/types/v4_0_12"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	"github.com/juju/juju/internal/errors"
)

// ChangeLogEditType is target-local schema data, excluded from import. Ignore
// source rows rather than validating or restoring their IDs on the target.
func (d deltas) ChangeLogEditType(_ context.Context, _ []v4_0_12.ChangeLogEditType, _ *v4_0_12.ModelExport) ([]v4_1_0.ChangeLogEditType, error) {
	return nil, nil
}

// ChangeLogNamespace is target-local schema data, excluded from import.
func (d deltas) ChangeLogNamespace(_ context.Context, _ []v4_0_12.ChangeLogNamespace, _ *v4_0_12.ModelExport) ([]v4_1_0.ChangeLogNamespace, error) {
	return nil, nil
}

// AgentStream preserves the legacy row with a required lookup ID.
func (d deltas) AgentStream(_ context.Context, src []v4_0_12.AgentStream, _ *v4_0_12.ModelExport) ([]v4_1_0.AgentStream, error) {
	result := make([]v4_1_0.AgentStream, len(src))
	for i, row := range src {
		id, err := requiredLookupID("agent_stream", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.AgentStream{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// Architecture preserves the legacy row with a required lookup ID.
func (d deltas) Architecture(_ context.Context, src []v4_0_12.Architecture, _ *v4_0_12.ModelExport) ([]v4_1_0.Architecture, error) {
	result := make([]v4_1_0.Architecture, len(src))
	for i, row := range src {
		id, err := requiredLookupID("architecture", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.Architecture{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// BlockCommandType preserves the legacy row with a required lookup ID.
func (d deltas) BlockCommandType(_ context.Context, src []v4_0_12.BlockCommandType, _ *v4_0_12.ModelExport) ([]v4_1_0.BlockCommandType, error) {
	result := make([]v4_1_0.BlockCommandType, len(src))
	for i, row := range src {
		id, err := requiredLookupID("block_command_type", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.BlockCommandType{
			ID:       id,
			NameType: row.NameType,
		}
	}
	return result, nil
}

// BlockDeviceProvenance preserves the legacy row with a required lookup ID.
func (d deltas) BlockDeviceProvenance(_ context.Context, src []v4_0_12.BlockDeviceProvenance, _ *v4_0_12.ModelExport) ([]v4_1_0.BlockDeviceProvenance, error) {
	result := make([]v4_1_0.BlockDeviceProvenance, len(src))
	for i, row := range src {
		id, err := requiredLookupID("block_device_provenance", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.BlockDeviceProvenance{
			ID:    id,
			Value: row.Value,
		}
	}
	return result, nil
}

// CharmConfigType preserves the legacy row with a required lookup ID.
func (d deltas) CharmConfigType(_ context.Context, src []v4_0_12.CharmConfigType, _ *v4_0_12.ModelExport) ([]v4_1_0.CharmConfigType, error) {
	result := make([]v4_1_0.CharmConfigType, len(src))
	for i, row := range src {
		id, err := requiredLookupID("charm_config_type", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.CharmConfigType{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// CharmProvenance preserves the legacy row with a required lookup ID.
func (d deltas) CharmProvenance(_ context.Context, src []v4_0_12.CharmProvenance, _ *v4_0_12.ModelExport) ([]v4_1_0.CharmProvenance, error) {
	result := make([]v4_1_0.CharmProvenance, len(src))
	for i, row := range src {
		id, err := requiredLookupID("charm_provenance", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.CharmProvenance{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// CharmRelationRole preserves the legacy row with a required lookup ID.
func (d deltas) CharmRelationRole(_ context.Context, src []v4_0_12.CharmRelationRole, _ *v4_0_12.ModelExport) ([]v4_1_0.CharmRelationRole, error) {
	result := make([]v4_1_0.CharmRelationRole, len(src))
	for i, row := range src {
		id, err := requiredLookupID("charm_relation_role", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.CharmRelationRole{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// CharmRelationScope preserves the legacy row with a required lookup ID.
func (d deltas) CharmRelationScope(_ context.Context, src []v4_0_12.CharmRelationScope, _ *v4_0_12.ModelExport) ([]v4_1_0.CharmRelationScope, error) {
	result := make([]v4_1_0.CharmRelationScope, len(src))
	for i, row := range src {
		id, err := requiredLookupID("charm_relation_scope", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.CharmRelationScope{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// CharmResourceKind preserves the legacy row with a required lookup ID.
func (d deltas) CharmResourceKind(_ context.Context, src []v4_0_12.CharmResourceKind, _ *v4_0_12.ModelExport) ([]v4_1_0.CharmResourceKind, error) {
	result := make([]v4_1_0.CharmResourceKind, len(src))
	for i, row := range src {
		id, err := requiredLookupID("charm_resource_kind", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.CharmResourceKind{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// CharmRunAsKind preserves the legacy row with a required lookup ID.
func (d deltas) CharmRunAsKind(_ context.Context, src []v4_0_12.CharmRunAsKind, _ *v4_0_12.ModelExport) ([]v4_1_0.CharmRunAsKind, error) {
	result := make([]v4_1_0.CharmRunAsKind, len(src))
	for i, row := range src {
		id, err := requiredLookupID("charm_run_as_kind", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.CharmRunAsKind{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// CharmSource preserves the legacy row with a required lookup ID.
func (d deltas) CharmSource(_ context.Context, src []v4_0_12.CharmSource, _ *v4_0_12.ModelExport) ([]v4_1_0.CharmSource, error) {
	result := make([]v4_1_0.CharmSource, len(src))
	for i, row := range src {
		id, err := requiredLookupID("charm_source", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.CharmSource{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// CharmStorageKind preserves the legacy row with a required lookup ID.
func (d deltas) CharmStorageKind(_ context.Context, src []v4_0_12.CharmStorageKind, _ *v4_0_12.ModelExport) ([]v4_1_0.CharmStorageKind, error) {
	result := make([]v4_1_0.CharmStorageKind, len(src))
	for i, row := range src {
		id, err := requiredLookupID("charm_storage_kind", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.CharmStorageKind{
			ID:   id,
			Kind: row.Kind,
		}
	}
	return result, nil
}

// ContainerType preserves the legacy row with a required lookup ID.
func (d deltas) ContainerType(_ context.Context, src []v4_0_12.ContainerType, _ *v4_0_12.ModelExport) ([]v4_1_0.ContainerType, error) {
	result := make([]v4_1_0.ContainerType, len(src))
	for i, row := range src {
		id, err := requiredLookupID("container_type", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.ContainerType{
			ID:    id,
			Value: row.Value,
		}
	}
	return result, nil
}

// HashKind preserves the legacy row with a required lookup ID.
func (d deltas) HashKind(_ context.Context, src []v4_0_12.HashKind, _ *v4_0_12.ModelExport) ([]v4_1_0.HashKind, error) {
	result := make([]v4_1_0.HashKind, len(src))
	for i, row := range src {
		id, err := requiredLookupID("hash_kind", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.HashKind{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// IpAddressConfigType preserves the legacy row with a required lookup ID.
func (d deltas) IpAddressConfigType(_ context.Context, src []v4_0_12.IpAddressConfigType, _ *v4_0_12.ModelExport) ([]v4_1_0.IpAddressConfigType, error) {
	result := make([]v4_1_0.IpAddressConfigType, len(src))
	for i, row := range src {
		id, err := requiredLookupID("ip_address_config_type", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.IpAddressConfigType{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// IpAddressOrigin preserves the legacy row with a required lookup ID.
func (d deltas) IpAddressOrigin(_ context.Context, src []v4_0_12.IpAddressOrigin, _ *v4_0_12.ModelExport) ([]v4_1_0.IpAddressOrigin, error) {
	result := make([]v4_1_0.IpAddressOrigin, len(src))
	for i, row := range src {
		id, err := requiredLookupID("ip_address_origin", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.IpAddressOrigin{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// IpAddressScope preserves the legacy row with a required lookup ID.
func (d deltas) IpAddressScope(_ context.Context, src []v4_0_12.IpAddressScope, _ *v4_0_12.ModelExport) ([]v4_1_0.IpAddressScope, error) {
	result := make([]v4_1_0.IpAddressScope, len(src))
	for i, row := range src {
		id, err := requiredLookupID("ip_address_scope", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.IpAddressScope{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// IpAddressType preserves the legacy row with a required lookup ID.
func (d deltas) IpAddressType(_ context.Context, src []v4_0_12.IpAddressType, _ *v4_0_12.ModelExport) ([]v4_1_0.IpAddressType, error) {
	result := make([]v4_1_0.IpAddressType, len(src))
	for i, row := range src {
		id, err := requiredLookupID("ip_address_type", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.IpAddressType{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// K8sPodStatusValue preserves the legacy row with a required lookup ID.
func (d deltas) K8sPodStatusValue(_ context.Context, src []v4_0_12.K8sPodStatusValue, _ *v4_0_12.ModelExport) ([]v4_1_0.K8sPodStatusValue, error) {
	result := make([]v4_1_0.K8sPodStatusValue, len(src))
	for i, row := range src {
		id, err := requiredLookupID("k8s_pod_status_value", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.K8sPodStatusValue{
			ID:     id,
			Status: row.Status,
		}
	}
	return result, nil
}

// Life preserves the legacy row with a required lookup ID.
func (d deltas) Life(_ context.Context, src []v4_0_12.Life, _ *v4_0_12.ModelExport) ([]v4_1_0.Life, error) {
	result := make([]v4_1_0.Life, len(src))
	for i, row := range src {
		id, err := requiredLookupID("life", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.Life{
			ID:    id,
			Value: row.Value,
		}
	}
	return result, nil
}

// LinkLayerDeviceType preserves the legacy row with a required lookup ID.
func (d deltas) LinkLayerDeviceType(_ context.Context, src []v4_0_12.LinkLayerDeviceType, _ *v4_0_12.ModelExport) ([]v4_1_0.LinkLayerDeviceType, error) {
	result := make([]v4_1_0.LinkLayerDeviceType, len(src))
	for i, row := range src {
		id, err := requiredLookupID("link_layer_device_type", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.LinkLayerDeviceType{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// MachineCloudInstanceStatusValue preserves the legacy row with a required
// lookup ID.
func (d deltas) MachineCloudInstanceStatusValue(_ context.Context, src []v4_0_12.MachineCloudInstanceStatusValue, _ *v4_0_12.ModelExport) ([]v4_1_0.MachineCloudInstanceStatusValue, error) {
	result := make([]v4_1_0.MachineCloudInstanceStatusValue, len(src))
	for i, row := range src {
		id, err := requiredLookupID("machine_cloud_instance_status_value", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.MachineCloudInstanceStatusValue{
			ID:     id,
			Status: row.Status,
		}
	}
	return result, nil
}

// MachinePlacementScope preserves the legacy row with a required lookup ID.
func (d deltas) MachinePlacementScope(_ context.Context, src []v4_0_12.MachinePlacementScope, _ *v4_0_12.ModelExport) ([]v4_1_0.MachinePlacementScope, error) {
	result := make([]v4_1_0.MachinePlacementScope, len(src))
	for i, row := range src {
		id, err := requiredLookupID("machine_placement_scope", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.MachinePlacementScope{
			ID:    id,
			Value: row.Value,
		}
	}
	return result, nil
}

// MachineStatusValue preserves the legacy row with a required lookup ID.
func (d deltas) MachineStatusValue(_ context.Context, src []v4_0_12.MachineStatusValue, _ *v4_0_12.ModelExport) ([]v4_1_0.MachineStatusValue, error) {
	result := make([]v4_1_0.MachineStatusValue, len(src))
	for i, row := range src {
		id, err := requiredLookupID("machine_status_value", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.MachineStatusValue{
			ID:     id,
			Status: row.Status,
		}
	}
	return result, nil
}

// NetworkAddressScope preserves the legacy row with a required lookup ID.
func (d deltas) NetworkAddressScope(_ context.Context, src []v4_0_12.NetworkAddressScope, _ *v4_0_12.ModelExport) ([]v4_1_0.NetworkAddressScope, error) {
	result := make([]v4_1_0.NetworkAddressScope, len(src))
	for i, row := range src {
		id, err := requiredLookupID("network_address_scope", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.NetworkAddressScope{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// OperationTaskStatusValue preserves the legacy row with a required lookup ID.
func (d deltas) OperationTaskStatusValue(_ context.Context, src []v4_0_12.OperationTaskStatusValue, _ *v4_0_12.ModelExport) ([]v4_1_0.OperationTaskStatusValue, error) {
	result := make([]v4_1_0.OperationTaskStatusValue, len(src))
	for i, row := range src {
		id, err := requiredLookupID("operation_task_status_value", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.OperationTaskStatusValue{
			ID:     id,
			Status: row.Status,
		}
	}
	return result, nil
}

// Os preserves the legacy row with a required lookup ID.
func (d deltas) Os(_ context.Context, src []v4_0_12.Os, _ *v4_0_12.ModelExport) ([]v4_1_0.Os, error) {
	result := make([]v4_1_0.Os, len(src))
	for i, row := range src {
		id, err := requiredLookupID("os", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.Os{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// PasswordHashAlgorithm preserves the legacy row with a required lookup ID.
func (d deltas) PasswordHashAlgorithm(_ context.Context, src []v4_0_12.PasswordHashAlgorithm, _ *v4_0_12.ModelExport) ([]v4_1_0.PasswordHashAlgorithm, error) {
	result := make([]v4_1_0.PasswordHashAlgorithm, len(src))
	for i, row := range src {
		id, err := requiredLookupID("password_hash_algorithm", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.PasswordHashAlgorithm{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// Protocol preserves the legacy row with a required lookup ID.
func (d deltas) Protocol(_ context.Context, src []v4_0_12.Protocol, _ *v4_0_12.ModelExport) ([]v4_1_0.Protocol, error) {
	result := make([]v4_1_0.Protocol, len(src))
	for i, row := range src {
		id, err := requiredLookupID("protocol", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.Protocol{
			ID:       id,
			Protocol: row.Protocol,
		}
	}
	return result, nil
}

// ResolveMode preserves the legacy row with a required lookup ID.
func (d deltas) ResolveMode(_ context.Context, src []v4_0_12.ResolveMode, _ *v4_0_12.ModelExport) ([]v4_1_0.ResolveMode, error) {
	result := make([]v4_1_0.ResolveMode, len(src))
	for i, row := range src {
		id, err := requiredLookupID("resolve_mode", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.ResolveMode{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// ResourceOriginType preserves the legacy row with a required lookup ID.
func (d deltas) ResourceOriginType(_ context.Context, src []v4_0_12.ResourceOriginType, _ *v4_0_12.ModelExport) ([]v4_1_0.ResourceOriginType, error) {
	result := make([]v4_1_0.ResourceOriginType, len(src))
	for i, row := range src {
		id, err := requiredLookupID("resource_origin_type", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.ResourceOriginType{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// ResourceRetrievedByType preserves the legacy row with a required lookup ID.
func (d deltas) ResourceRetrievedByType(_ context.Context, src []v4_0_12.ResourceRetrievedByType, _ *v4_0_12.ModelExport) ([]v4_1_0.ResourceRetrievedByType, error) {
	result := make([]v4_1_0.ResourceRetrievedByType, len(src))
	for i, row := range src {
		id, err := requiredLookupID("resource_retrieved_by_type", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.ResourceRetrievedByType{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// ResourceState preserves the legacy row with a required lookup ID.
func (d deltas) ResourceState(_ context.Context, src []v4_0_12.ResourceState, _ *v4_0_12.ModelExport) ([]v4_1_0.ResourceState, error) {
	result := make([]v4_1_0.ResourceState, len(src))
	for i, row := range src {
		id, err := requiredLookupID("resource_state", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.ResourceState{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// SecretGrantScopeType preserves the legacy row with a required lookup ID.
func (d deltas) SecretGrantScopeType(_ context.Context, src []v4_0_12.SecretGrantScopeType, _ *v4_0_12.ModelExport) ([]v4_1_0.SecretGrantScopeType, error) {
	result := make([]v4_1_0.SecretGrantScopeType, len(src))
	for i, row := range src {
		id, err := requiredLookupID("secret_grant_scope_type", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.SecretGrantScopeType{
			ID:   id,
			Type: row.Type,
		}
	}
	return result, nil
}

// SecretGrantSubjectType preserves the legacy row with a required lookup ID.
func (d deltas) SecretGrantSubjectType(_ context.Context, src []v4_0_12.SecretGrantSubjectType, _ *v4_0_12.ModelExport) ([]v4_1_0.SecretGrantSubjectType, error) {
	result := make([]v4_1_0.SecretGrantSubjectType, len(src))
	for i, row := range src {
		id, err := requiredLookupID("secret_grant_subject_type", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.SecretGrantSubjectType{
			ID:   id,
			Type: row.Type,
		}
	}
	return result, nil
}

// SecretRole preserves the legacy row with a required lookup ID.
func (d deltas) SecretRole(_ context.Context, src []v4_0_12.SecretRole, _ *v4_0_12.ModelExport) ([]v4_1_0.SecretRole, error) {
	result := make([]v4_1_0.SecretRole, len(src))
	for i, row := range src {
		id, err := requiredLookupID("secret_role", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.SecretRole{
			ID:   id,
			Role: row.Role,
		}
	}
	return result, nil
}

// SecretRotatePolicy preserves the legacy row with a required lookup ID.
func (d deltas) SecretRotatePolicy(_ context.Context, src []v4_0_12.SecretRotatePolicy, _ *v4_0_12.ModelExport) ([]v4_1_0.SecretRotatePolicy, error) {
	result := make([]v4_1_0.SecretRotatePolicy, len(src))
	for i, row := range src {
		id, err := requiredLookupID("secret_rotate_policy", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.SecretRotatePolicy{
			ID:     id,
			Policy: row.Policy,
		}
	}
	return result, nil
}

// StorageFilesystemStatusValue preserves the legacy row with a required
// lookup ID.
func (d deltas) StorageFilesystemStatusValue(_ context.Context, src []v4_0_12.StorageFilesystemStatusValue, _ *v4_0_12.ModelExport) ([]v4_1_0.StorageFilesystemStatusValue, error) {
	result := make([]v4_1_0.StorageFilesystemStatusValue, len(src))
	for i, row := range src {
		id, err := requiredLookupID("storage_filesystem_status_value", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.StorageFilesystemStatusValue{
			ID:     id,
			Status: row.Status,
		}
	}
	return result, nil
}

// StorageKind preserves the legacy row with a required lookup ID.
func (d deltas) StorageKind(_ context.Context, src []v4_0_12.StorageKind, _ *v4_0_12.ModelExport) ([]v4_1_0.StorageKind, error) {
	result := make([]v4_1_0.StorageKind, len(src))
	for i, row := range src {
		id, err := requiredLookupID("storage_kind", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.StorageKind{
			ID:   id,
			Kind: row.Kind,
		}
	}
	return result, nil
}

// StorageProvisionScope preserves the legacy row with a required lookup ID.
func (d deltas) StorageProvisionScope(_ context.Context, src []v4_0_12.StorageProvisionScope, _ *v4_0_12.ModelExport) ([]v4_1_0.StorageProvisionScope, error) {
	result := make([]v4_1_0.StorageProvisionScope, len(src))
	for i, row := range src {
		id, err := requiredLookupID("storage_provision_scope", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.StorageProvisionScope{
			ID:    id,
			Scope: row.Scope,
		}
	}
	return result, nil
}

// StorageVolumeDeviceType preserves the legacy row with a required lookup ID.
func (d deltas) StorageVolumeDeviceType(_ context.Context, src []v4_0_12.StorageVolumeDeviceType, _ *v4_0_12.ModelExport) ([]v4_1_0.StorageVolumeDeviceType, error) {
	result := make([]v4_1_0.StorageVolumeDeviceType, len(src))
	for i, row := range src {
		id, err := requiredLookupID("storage_volume_device_type", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.StorageVolumeDeviceType{
			ID:          id,
			Name:        row.Name,
			Description: row.Description,
		}
	}
	return result, nil
}

// StorageVolumeStatusValue preserves the legacy row with a required lookup ID.
func (d deltas) StorageVolumeStatusValue(_ context.Context, src []v4_0_12.StorageVolumeStatusValue, _ *v4_0_12.ModelExport) ([]v4_1_0.StorageVolumeStatusValue, error) {
	result := make([]v4_1_0.StorageVolumeStatusValue, len(src))
	for i, row := range src {
		id, err := requiredLookupID("storage_volume_status_value", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.StorageVolumeStatusValue{
			ID:     id,
			Status: row.Status,
		}
	}
	return result, nil
}

// UnitAgentStatusValue preserves the legacy row with a required lookup ID.
func (d deltas) UnitAgentStatusValue(_ context.Context, src []v4_0_12.UnitAgentStatusValue, _ *v4_0_12.ModelExport) ([]v4_1_0.UnitAgentStatusValue, error) {
	result := make([]v4_1_0.UnitAgentStatusValue, len(src))
	for i, row := range src {
		id, err := requiredLookupID("unit_agent_status_value", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.UnitAgentStatusValue{
			ID:     id,
			Status: row.Status,
		}
	}
	return result, nil
}

// VirtualPortType preserves the legacy row with a required lookup ID.
func (d deltas) VirtualPortType(_ context.Context, src []v4_0_12.VirtualPortType, _ *v4_0_12.ModelExport) ([]v4_1_0.VirtualPortType, error) {
	result := make([]v4_1_0.VirtualPortType, len(src))
	for i, row := range src {
		id, err := requiredLookupID("virtual_port_type", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.VirtualPortType{
			ID:   id,
			Name: row.Name,
		}
	}
	return result, nil
}

// WorkloadStatusValue preserves the legacy row with a required lookup ID.
func (d deltas) WorkloadStatusValue(_ context.Context, src []v4_0_12.WorkloadStatusValue, _ *v4_0_12.ModelExport) ([]v4_1_0.WorkloadStatusValue, error) {
	result := make([]v4_1_0.WorkloadStatusValue, len(src))
	for i, row := range src {
		id, err := requiredLookupID("workload_status_value", i, row.ID)
		if err != nil {
			return nil, err
		}
		result[i] = v4_1_0.WorkloadStatusValue{
			ID:     id,
			Status: row.Status,
		}
	}
	return result, nil
}

// Lookup IDs become non-nullable in 4.1.0. Keep zero as a valid ID and
// report the source table and zero-based row index when an ID is absent.
func requiredLookupID(table string, row int, value *int64) (int64, error) {
	if value == nil {
		return 0, errors.Errorf("invalid %s row %d: id is NULL", table, row).Add(coreerrors.NotValid)
	}
	return *value, nil
}
