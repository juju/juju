// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"encoding/json"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/domain/charm"
	charmerrors "github.com/juju/juju/domain/charm/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/charm/resource"
)

// Conversion code is used to decode charm.Metadata code to non-domain
// charm.Metadata code. The domain charm.Metadata code is used as the
// normalisation layer for charm metadata. The persistence layer will ensure
// that the charm metadata is stored in the correct format.

func decodeMetadata(metadata charm.Metadata) (internalcharm.Meta, error) {
	provides, err := decodeMetadataRelation(metadata.Provides)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("decode provides relation: %w", err)
	}

	requires, err := decodeMetadataRelation(metadata.Requires)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("decode requires relation: %w", err)
	}

	peers, err := decodeMetadataRelation(metadata.Peers)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("decode peers relation: %w", err)
	}

	storage, err := decodeMetadataStorage(metadata.Storage)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("decode storage: %w", err)
	}

	devices, err := decodeMetadataDevices(metadata.Devices)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("decode devices: %w", err)
	}

	payloadClasses, err := decodeMetadataPayloadClasses(metadata.PayloadClasses)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("decode payload classes: %w", err)
	}

	resources, err := decodeMetadataResources(metadata.Resources)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("decode resources: %w", err)
	}

	containers, err := decodeMetadataContainers(metadata.Containers)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("decode containers: %w", err)
	}

	assumes, err := decodeMetadataAssumes(metadata.Assumes)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("parse assumes: %w", err)
	}

	charmUser, err := decodeMetadataRunAs(metadata.RunAs)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("decode charm user: %w", err)
	}

	return internalcharm.Meta{
		Name:           metadata.Name,
		Summary:        metadata.Summary,
		Description:    metadata.Description,
		Subordinate:    metadata.Subordinate,
		Categories:     metadata.Categories,
		Tags:           metadata.Tags,
		MinJujuVersion: metadata.MinJujuVersion,
		Terms:          metadata.Terms,
		Provides:       provides,
		Requires:       requires,
		Peers:          peers,
		ExtraBindings:  decodeMetadataExtraBindings(metadata.ExtraBindings),
		Storage:        storage,
		Devices:        devices,
		PayloadClasses: payloadClasses,
		Resources:      resources,
		Containers:     containers,
		Assumes:        assumes,
		CharmUser:      charmUser,
	}, nil
}

func decodeMetadataRelation(relations map[string]charm.Relation) (map[string]internalcharm.Relation, error) {
	if len(relations) == 0 {
		return nil, nil
	}

	result := make(map[string]internalcharm.Relation, len(relations))
	for k, v := range relations {
		role, err := decodeMetadataRole(v.Role)
		if err != nil {
			return nil, fmt.Errorf("decode role: %w", err)
		}

		scope, err := decodeMetadataScope(v.Scope)
		if err != nil {
			return nil, fmt.Errorf("decode scope: %w", err)
		}

		result[k] = internalcharm.Relation{
			Name:      v.Name,
			Role:      role,
			Scope:     scope,
			Interface: v.Interface,
			Optional:  v.Optional,
			Limit:     v.Limit,
		}
	}
	return result, nil
}

func decodeMetadataRole(role charm.RelationRole) (internalcharm.RelationRole, error) {
	switch role {
	case charm.RoleProvider:
		return internalcharm.RoleProvider, nil
	case charm.RoleRequirer:
		return internalcharm.RoleRequirer, nil
	case charm.RolePeer:
		return internalcharm.RolePeer, nil
	default:
		return "", errors.Errorf("unknown role %q", role)
	}
}

func decodeMetadataScope(scope charm.RelationScope) (internalcharm.RelationScope, error) {
	switch scope {
	case charm.ScopeGlobal:
		return internalcharm.ScopeGlobal, nil
	case charm.ScopeContainer:
		return internalcharm.ScopeContainer, nil
	default:
		return "", errors.Errorf("unknown scope %q", scope)
	}
}

func decodeMetadataExtraBindings(bindings map[string]charm.ExtraBinding) map[string]internalcharm.ExtraBinding {
	if len(bindings) == 0 {
		return nil
	}

	result := make(map[string]internalcharm.ExtraBinding, len(bindings))
	for k, v := range bindings {
		result[k] = internalcharm.ExtraBinding{
			Name: v.Name,
		}
	}
	return result
}

func decodeMetadataStorage(storage map[string]charm.Storage) (map[string]internalcharm.Storage, error) {
	if len(storage) == 0 {
		return nil, nil
	}

	result := make(map[string]internalcharm.Storage, len(storage))
	for k, v := range storage {
		storeType, err := decodeMetadataStorageType(v.Type)
		if err != nil {
			return nil, fmt.Errorf("decode storage type: %w", err)
		}

		result[k] = internalcharm.Storage{
			Name:        v.Name,
			Description: v.Description,
			Type:        storeType,
			Shared:      v.Shared,
			ReadOnly:    v.ReadOnly,
			CountMin:    v.CountMin,
			CountMax:    v.CountMax,
			MinimumSize: v.MinimumSize,
			Location:    v.Location,
			Properties:  v.Properties,
		}
	}
	return result, nil
}

func decodeMetadataStorageType(storeType charm.StorageType) (internalcharm.StorageType, error) {
	switch storeType {
	case charm.StorageBlock:
		return internalcharm.StorageBlock, nil
	case charm.StorageFilesystem:
		return internalcharm.StorageFilesystem, nil
	default:
		return "", errors.Errorf("unknown storage type %q", storeType)
	}
}

func decodeMetadataDevices(devices map[string]charm.Device) (map[string]internalcharm.Device, error) {
	if len(devices) == 0 {
		return nil, nil
	}

	result := make(map[string]internalcharm.Device, len(devices))
	for k, v := range devices {
		result[k] = internalcharm.Device{
			Name:        v.Name,
			Description: v.Description,
			Type:        internalcharm.DeviceType(v.Type),
			CountMin:    v.CountMin,
			CountMax:    v.CountMax,
		}
	}
	return result, nil
}

func decodeMetadataPayloadClasses(payloadClasses map[string]charm.PayloadClass) (map[string]internalcharm.PayloadClass, error) {
	if len(payloadClasses) == 0 {
		return nil, nil
	}

	result := make(map[string]internalcharm.PayloadClass, len(payloadClasses))
	for k, v := range payloadClasses {
		result[k] = internalcharm.PayloadClass{
			Name: v.Name,
			Type: v.Type,
		}
	}
	return result, nil
}

func decodeMetadataResources(resources map[string]charm.Resource) (map[string]resource.Meta, error) {
	if len(resources) == 0 {
		return nil, nil
	}

	result := make(map[string]resource.Meta, len(resources))
	for k, v := range resources {
		resourceType, err := decodeMetadataResourceType(v.Type)
		if err != nil {
			return nil, fmt.Errorf("decode resource type: %w", err)
		}

		result[k] = resource.Meta{
			Name:        v.Name,
			Description: v.Description,
			Path:        v.Path,
			Type:        resourceType,
		}
	}

	return result, nil
}

func decodeMetadataResourceType(resourceType charm.ResourceType) (resource.Type, error) {
	switch resourceType {
	case charm.ResourceTypeFile:
		return resource.TypeFile, nil
	case charm.ResourceTypeContainerImage:
		return resource.TypeContainerImage, nil
	default:
		// Zero is the unknown resource type.
		return resource.Type(0), errors.Errorf("unknown resource type %q", resourceType)
	}
}

func decodeMetadataContainers(containers map[string]charm.Container) (map[string]internalcharm.Container, error) {
	if len(containers) == 0 {
		return nil, nil
	}

	result := make(map[string]internalcharm.Container, len(containers))
	for k, v := range containers {
		mounts, err := decodeMetadataMounts(v.Mounts)
		if err != nil {
			return nil, fmt.Errorf("decode mounts: %w", err)
		}

		result[k] = internalcharm.Container{
			Resource: v.Resource,
			Mounts:   mounts,
			Uid:      v.Uid,
			Gid:      v.Gid,
		}
	}
	return result, nil
}

func decodeMetadataMounts(mounts []charm.Mount) ([]internalcharm.Mount, error) {
	if len(mounts) == 0 {
		return nil, nil
	}

	result := make([]internalcharm.Mount, len(mounts))
	for i, v := range mounts {
		result[i] = internalcharm.Mount{
			Storage:  v.Storage,
			Location: v.Location,
		}
	}
	return result, nil
}

func decodeMetadataRunAs(charmUser charm.RunAs) (internalcharm.RunAs, error) {
	// RunAsDefault is different from the wire protocol. Ensure we decode it
	// correctly.
	switch charmUser {
	case charm.RunAsDefault:
		return internalcharm.RunAsDefault, nil
	case charm.RunAsRoot:
		return internalcharm.RunAsRoot, nil
	case charm.RunAsSudoer:
		return internalcharm.RunAsSudoer, nil
	case charm.RunAsNonRoot:
		return internalcharm.RunAsNonRoot, nil
	default:
		return "", errors.Errorf("unknown charm user %q", charmUser)
	}
}

func decodeMetadataAssumes(bytes []byte) (*assumes.ExpressionTree, error) {
	if len(bytes) == 0 {
		return nil, nil
	}

	// All assumes expressions will be stored as a JSON blob. If we ever need
	// to access the assume expressions in the future, we can utilise SQLite
	// JSONB functions.
	dst := struct {
		Assumes *assumes.ExpressionTree `json:"assumes"`
	}{}
	if err := json.Unmarshal(bytes, &dst); err != nil {
		return nil, errors.Annotate(err, "unmarshal assumes")
	}
	return dst.Assumes, nil
}

func encodeMetadata(metadata *internalcharm.Meta) (charm.Metadata, error) {
	if metadata == nil {
		return charm.Metadata{}, charmerrors.MetadataNotValid
	}

	provides, err := encodeMetadataRelation(metadata.Provides)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("encode provides relation: %w", err)
	}

	requires, err := encodeMetadataRelation(metadata.Requires)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("encode requires relation: %w", err)
	}

	peers, err := encodeMetadataRelation(metadata.Peers)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("encode peers relation: %w", err)
	}

	storage, err := encodeMetadataStorage(metadata.Storage)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("encode storage: %w", err)
	}

	devices, err := encodeMetadataDevices(metadata.Devices)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("encode devices: %w", err)
	}

	payloadClasses, err := encodeMetadataPayloadClasses(metadata.PayloadClasses)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("encode payload classes: %w", err)
	}

	resources, err := encodeMetadataResources(metadata.Resources)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("encode resources: %w", err)
	}

	containers, err := encodeMetadataContainers(metadata.Containers)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("encode containers: %w", err)
	}

	assumes, err := encodeMetadataAssumes(metadata.Assumes)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("encode assumes: %w", err)
	}

	charmUser, err := encodeMetadataRunAs(metadata.CharmUser)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("encode charm user: %w", err)
	}

	return charm.Metadata{
		Name:           metadata.Name,
		Summary:        metadata.Summary,
		Description:    metadata.Description,
		Subordinate:    metadata.Subordinate,
		Categories:     metadata.Categories,
		Tags:           metadata.Tags,
		MinJujuVersion: metadata.MinJujuVersion,
		Terms:          metadata.Terms,
		Provides:       provides,
		Requires:       requires,
		Peers:          peers,
		ExtraBindings:  encodeMetadataExtraBindings(metadata.ExtraBindings),
		Storage:        storage,
		Devices:        devices,
		PayloadClasses: payloadClasses,
		Resources:      resources,
		Containers:     containers,
		Assumes:        assumes,
		RunAs:          charmUser,
	}, nil
}

func encodeMetadataRelation(relations map[string]internalcharm.Relation) (map[string]charm.Relation, error) {
	if len(relations) == 0 {
		return nil, nil
	}

	result := make(map[string]charm.Relation, len(relations))
	for k, v := range relations {
		role, err := encodeMetadataRole(v.Role)
		if err != nil {
			return nil, fmt.Errorf("encode role: %w", err)
		}

		scope, err := encodeMetadataScope(v.Scope)
		if err != nil {
			return nil, fmt.Errorf("encode scope: %w", err)
		}

		result[k] = charm.Relation{
			Name:      v.Name,
			Role:      role,
			Scope:     scope,
			Interface: v.Interface,
			Optional:  v.Optional,
			Limit:     v.Limit,
		}
	}
	return result, nil
}

func encodeMetadataRole(role internalcharm.RelationRole) (charm.RelationRole, error) {
	switch role {
	case internalcharm.RoleProvider:
		return charm.RoleProvider, nil
	case internalcharm.RoleRequirer:
		return charm.RoleRequirer, nil
	case internalcharm.RolePeer:
		return charm.RolePeer, nil
	default:
		return "", errors.Errorf("unknown role %q", role)
	}
}

func encodeMetadataScope(scope internalcharm.RelationScope) (charm.RelationScope, error) {
	switch scope {
	case internalcharm.ScopeGlobal:
		return charm.ScopeGlobal, nil
	case internalcharm.ScopeContainer:
		return charm.ScopeContainer, nil
	default:
		return "", errors.Errorf("unknown scope %q", scope)
	}
}

func encodeMetadataStorage(storage map[string]internalcharm.Storage) (map[string]charm.Storage, error) {
	if len(storage) == 0 {
		return nil, nil
	}

	result := make(map[string]charm.Storage, len(storage))
	for k, v := range storage {
		storeType, err := encodeMetadataStorageType(v.Type)
		if err != nil {
			return nil, fmt.Errorf("encode storage type: %w", err)
		}

		result[k] = charm.Storage{
			Name:        v.Name,
			Description: v.Description,
			Type:        storeType,
			Shared:      v.Shared,
			ReadOnly:    v.ReadOnly,
			CountMin:    v.CountMin,
			CountMax:    v.CountMax,
			MinimumSize: v.MinimumSize,
			Location:    v.Location,
			Properties:  v.Properties,
		}
	}
	return result, nil
}

func encodeMetadataStorageType(storeType internalcharm.StorageType) (charm.StorageType, error) {
	switch storeType {
	case internalcharm.StorageBlock:
		return charm.StorageBlock, nil
	case internalcharm.StorageFilesystem:
		return charm.StorageFilesystem, nil
	default:
		return "", errors.Errorf("unknown storage type %q", storeType)
	}
}

func encodeMetadataDevices(devices map[string]internalcharm.Device) (map[string]charm.Device, error) {
	if len(devices) == 0 {
		return nil, nil
	}

	result := make(map[string]charm.Device, len(devices))
	for k, v := range devices {
		result[k] = charm.Device{
			Name:        v.Name,
			Description: v.Description,
			Type:        charm.DeviceType(v.Type),
			CountMin:    v.CountMin,
			CountMax:    v.CountMax,
		}
	}
	return result, nil
}

func encodeMetadataPayloadClasses(payloadClasses map[string]internalcharm.PayloadClass) (map[string]charm.PayloadClass, error) {
	if len(payloadClasses) == 0 {
		return nil, nil
	}

	result := make(map[string]charm.PayloadClass, len(payloadClasses))
	for k, v := range payloadClasses {
		result[k] = charm.PayloadClass{
			Name: v.Name,
			Type: v.Type,
		}
	}
	return result, nil
}

func encodeMetadataResources(resources map[string]resource.Meta) (map[string]charm.Resource, error) {
	if len(resources) == 0 {
		return nil, nil
	}

	result := make(map[string]charm.Resource, len(resources))
	for k, v := range resources {
		resourceType, err := encodeMetadataResourceType(v.Type)
		if err != nil {
			return nil, fmt.Errorf("encode resource type: %w", err)
		}

		result[k] = charm.Resource{
			Name:        v.Name,
			Description: v.Description,
			Path:        v.Path,
			Type:        resourceType,
		}
	}

	return result, nil
}

func encodeMetadataResourceType(resourceType resource.Type) (charm.ResourceType, error) {
	switch resourceType {
	case resource.TypeFile:
		return charm.ResourceTypeFile, nil
	case resource.TypeContainerImage:
		return charm.ResourceTypeContainerImage, nil
	default:
		return "", errors.Errorf("unknown resource type %q", resourceType)
	}
}

func encodeMetadataContainers(containers map[string]internalcharm.Container) (map[string]charm.Container, error) {
	if len(containers) == 0 {
		return nil, nil
	}

	result := make(map[string]charm.Container, len(containers))
	for k, v := range containers {
		mounts, err := encodeMetadataMounts(v.Mounts)
		if err != nil {
			return nil, fmt.Errorf("encode mounts: %w", err)
		}

		result[k] = charm.Container{
			Resource: v.Resource,
			Mounts:   mounts,
			Uid:      v.Uid,
			Gid:      v.Gid,
		}
	}
	return result, nil
}

func encodeMetadataMounts(mounts []internalcharm.Mount) ([]charm.Mount, error) {
	if len(mounts) == 0 {
		return nil, nil
	}

	result := make([]charm.Mount, len(mounts))
	for i, v := range mounts {
		result[i] = charm.Mount{
			Storage:  v.Storage,
			Location: v.Location,
		}
	}
	return result, nil
}

func encodeMetadataAssumes(expr *assumes.ExpressionTree) ([]byte, error) {
	if expr == nil {
		return nil, nil
	}

	// All assumes expressions will be stored as a JSON blob. If we ever need
	// to access the assume expressions in the future, we can utilise SQLite
	// JSONB functions.
	return json.Marshal(struct {
		Assumes *assumes.ExpressionTree `json:"assumes"`
	}{Assumes: expr})
}

func encodeMetadataRunAs(charmUser internalcharm.RunAs) (charm.RunAs, error) {
	switch charmUser {
	case internalcharm.RunAsDefault:
		return charm.RunAsDefault, nil
	case internalcharm.RunAsRoot:
		return charm.RunAsRoot, nil
	case internalcharm.RunAsSudoer:
		return charm.RunAsSudoer, nil
	case internalcharm.RunAsNonRoot:
		return charm.RunAsNonRoot, nil
	default:
		return "", errors.Errorf("unknown charm user %q", charmUser)
	}
}

func encodeMetadataExtraBindings(bindings map[string]internalcharm.ExtraBinding) map[string]charm.ExtraBinding {
	if len(bindings) == 0 {
		return nil
	}

	result := make(map[string]charm.ExtraBinding, len(bindings))
	for k, v := range bindings {
		result[k] = charm.ExtraBinding{
			Name: v.Name,
		}
	}
	return result
}
