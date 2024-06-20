// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"encoding/json"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/domain/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/charm/resource"
)

// Conversion code is used to convert charm.Metadata code to non-domain
// charm.Metadata code. The domain charm.Metadata code is used as the
// normalisation layer for charm metadata. The persistence layer will ensure
// that the charm metadata is stored in the correct format.

func convertMetadata(metadata charm.Metadata) (internalcharm.Meta, error) {
	provides, err := convertMetadataRelation(metadata.Provides)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("convert provides relation: %w", err)
	}

	requires, err := convertMetadataRelation(metadata.Requires)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("convert requires relation: %w", err)
	}

	peers, err := convertMetadataRelation(metadata.Peers)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("convert peers relation: %w", err)
	}

	storage, err := convertMetadataStorage(metadata.Storage)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("convert storage: %w", err)
	}

	devices, err := convertMetadataDevices(metadata.Devices)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("convert devices: %w", err)
	}

	payloadClasses, err := convertMetadataPayloadClasses(metadata.PayloadClasses)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("convert payload classes: %w", err)
	}

	resources, err := convertMetadataResources(metadata.Resources)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("convert resources: %w", err)
	}

	containers, err := convertMetadataContainers(metadata.Containers)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("convert containers: %w", err)
	}

	assumes, err := convertMetadataAssumes(metadata.Assumes)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("parse assumes: %w", err)
	}

	charmUser, err := convertMetadataRunAs(metadata.RunAs)
	if err != nil {
		return internalcharm.Meta{}, fmt.Errorf("convert charm user: %w", err)
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
		ExtraBindings:  convertMetadataExtraBindings(metadata.ExtraBindings),
		Storage:        storage,
		Devices:        devices,
		PayloadClasses: payloadClasses,
		Resources:      resources,
		Containers:     containers,
		Assumes:        assumes,
		CharmUser:      charmUser,
	}, nil
}

func convertMetadataRelation(relations map[string]charm.Relation) (map[string]internalcharm.Relation, error) {
	if len(relations) == 0 {
		return nil, nil
	}

	result := make(map[string]internalcharm.Relation, len(relations))
	for k, v := range relations {
		role, err := convertMetadataRole(v.Role)
		if err != nil {
			return nil, fmt.Errorf("convert role: %w", err)
		}

		scope, err := convertMetadataScope(v.Scope)
		if err != nil {
			return nil, fmt.Errorf("convert scope: %w", err)
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

func convertMetadataRole(role charm.RelationRole) (internalcharm.RelationRole, error) {
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

func convertMetadataScope(scope charm.RelationScope) (internalcharm.RelationScope, error) {
	switch scope {
	case charm.ScopeGlobal:
		return internalcharm.ScopeGlobal, nil
	case charm.ScopeContainer:
		return internalcharm.ScopeContainer, nil
	default:
		return "", errors.Errorf("unknown scope %q", scope)
	}
}

func convertMetadataExtraBindings(bindings map[string]charm.ExtraBinding) map[string]internalcharm.ExtraBinding {
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

func convertMetadataStorage(storage map[string]charm.Storage) (map[string]internalcharm.Storage, error) {
	if len(storage) == 0 {
		return nil, nil
	}

	result := make(map[string]internalcharm.Storage, len(storage))
	for k, v := range storage {
		storeType, err := convertMetadataStorageType(v.Type)
		if err != nil {
			return nil, fmt.Errorf("convert storage type: %w", err)
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

func convertMetadataStorageType(storeType charm.StorageType) (internalcharm.StorageType, error) {
	switch storeType {
	case charm.StorageBlock:
		return internalcharm.StorageBlock, nil
	case charm.StorageFilesystem:
		return internalcharm.StorageFilesystem, nil
	default:
		return "", errors.Errorf("unknown storage type %q", storeType)
	}
}

func convertMetadataDevices(devices map[string]charm.Device) (map[string]internalcharm.Device, error) {
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

func convertMetadataPayloadClasses(payloadClasses map[string]charm.PayloadClass) (map[string]internalcharm.PayloadClass, error) {
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

func convertMetadataResources(resources map[string]charm.Resource) (map[string]resource.Meta, error) {
	if len(resources) == 0 {
		return nil, nil
	}

	result := make(map[string]resource.Meta, len(resources))
	for k, v := range resources {
		resourceType, err := convertMetadataResourceType(v.Type)
		if err != nil {
			return nil, fmt.Errorf("convert resource type: %w", err)
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

func convertMetadataResourceType(resourceType charm.ResourceType) (resource.Type, error) {
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

func convertMetadataContainers(containers map[string]charm.Container) (map[string]internalcharm.Container, error) {
	if len(containers) == 0 {
		return nil, nil
	}

	result := make(map[string]internalcharm.Container, len(containers))
	for k, v := range containers {
		mounts, err := convertMetadataMounts(v.Mounts)
		if err != nil {
			return nil, fmt.Errorf("convert mounts: %w", err)
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

func convertMetadataMounts(mounts []charm.Mount) ([]internalcharm.Mount, error) {
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

func convertMetadataRunAs(charmUser charm.RunAs) (internalcharm.RunAs, error) {
	// RunAsDefault is different from the wire protocol. Ensure we convert it
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

func convertMetadataAssumes(bytes []byte) (*assumes.ExpressionTree, error) {
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
