// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/version/v2"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

type relationKind = string

const (
	relationKindProvides relationKind = "provides"
	relationKindRequires relationKind = "requires"
	relationKindPeers    relationKind = "peers"
)

type decodeMetadataArgs struct {
	tags          []charmTag
	categories    []charmCategory
	terms         []charmTerm
	relations     []charmRelation
	extraBindings []charmExtraBinding
	storage       []charmStorage
	devices       []charmDevice
	resources     []charmResource
	containers    []charmContainer
}

func decodeMetadata(metadata charmMetadata, args decodeMetadataArgs) (charm.Metadata, error) {
	var (
		err        error
		minVersion version.Number
	)
	if metadata.MinJujuVersion != "" {
		minVersion, err = version.Parse(metadata.MinJujuVersion)
		if err != nil {
			return charm.Metadata{}, errors.Errorf("cannot parse min juju version %q: %w", metadata.MinJujuVersion, err)
		}
	}

	runAs, err := decodeRunAs(metadata.RunAs)
	if err != nil {
		return charm.Metadata{}, errors.Errorf("cannot decode run as %q: %w", metadata.RunAs, err)
	}

	provides, requires, peer, err := decodeRelations(args.relations)
	if err != nil {
		return charm.Metadata{}, errors.Errorf("cannot decode relations: %w", err)
	}

	storage, err := decodeStorage(args.storage)
	if err != nil {
		return charm.Metadata{}, errors.Errorf("cannot decode storage: %w", err)
	}

	resources, err := decodeResources(args.resources)
	if err != nil {
		return charm.Metadata{}, errors.Errorf("cannot decode resources: %w", err)
	}

	containers, err := decodeContainers(args.containers)
	if err != nil {
		return charm.Metadata{}, errors.Errorf("cannot decode containers: %w", err)
	}

	return charm.Metadata{
		Name:           metadata.Name,
		Summary:        metadata.Summary,
		Description:    metadata.Description,
		Subordinate:    metadata.Subordinate,
		MinJujuVersion: minVersion,
		RunAs:          runAs,
		Assumes:        metadata.Assumes,
		Tags:           decodeTags(args.tags),
		Categories:     decodeCategories(args.categories),
		Terms:          decodeTerms(args.terms),
		Provides:       provides,
		Requires:       requires,
		Peers:          peer,
		ExtraBindings:  decodeExtraBindings(args.extraBindings),
		Storage:        storage,
		Devices:        decodeDevices(args.devices),
		Resources:      resources,
		Containers:     containers,
	}, nil
}

func decodeRunAs(runAs string) (charm.RunAs, error) {
	switch runAs {
	case "default", "":
		return charm.RunAsDefault, nil
	case "root":
		return charm.RunAsRoot, nil
	case "sudoer":
		return charm.RunAsSudoer, nil
	case "user":
		return charm.RunAsNonRoot, nil
	default:
		return "", errors.Errorf("unknown run as value %q", runAs)
	}
}

func decodeTags(tags []charmTag) []string {
	var result []string
	for _, tag := range tags {
		result = append(result, tag.Tag)
	}
	return result
}

func decodeCategories(categories []charmCategory) []string {
	var result []string
	for _, category := range categories {
		result = append(result, category.Category)
	}
	return result
}

func decodeTerms(terms []charmTerm) []string {
	var result []string
	for _, category := range terms {
		result = append(result, category.Term)
	}
	return result
}

func decodeRelations(relations []charmRelation) (map[string]charm.Relation, map[string]charm.Relation, map[string]charm.Relation, error) {
	makeRelation := func(relation charmRelation) (charm.Relation, error) {
		role, err := decodeRelationRole(relation.Role)
		if err != nil {
			return charm.Relation{}, errors.Errorf("cannot decode relation role %q: %w", relation.Role, err)
		}

		scope, err := decodeRelationScope(relation.Scope)
		if err != nil {
			return charm.Relation{}, errors.Errorf("cannot decode relation scope %q: %w", relation.Scope, err)
		}

		return charm.Relation{
			Key:       relation.Key,
			Name:      relation.Name,
			Interface: relation.Interface,
			Optional:  relation.Optional,
			Limit:     relation.Capacity,
			Role:      role,
			Scope:     scope,
		}, nil
	}

	var provides, requires, peers map[string]charm.Relation
	for _, relation := range relations {
		rel, err := makeRelation(relation)
		if err != nil {
			return nil, nil, nil, errors.Errorf("cannot make relation: %w", err)
		}

		switch relation.Kind {
		case relationKindProvides:
			if provides == nil {
				provides = make(map[string]charm.Relation)
			}
			provides[relation.Key] = rel
		case relationKindRequires:
			if requires == nil {
				requires = make(map[string]charm.Relation)
			}
			requires[relation.Key] = rel
		case relationKindPeers:
			if peers == nil {
				peers = make(map[string]charm.Relation)
			}
			peers[relation.Key] = rel
		default:
			return nil, nil, nil, errors.Errorf("unknown relation role %q", relation.Kind)
		}
	}

	return provides, requires, peers, nil
}

func decodeRelationRole(role string) (charm.RelationRole, error) {
	switch role {
	case "provider":
		return charm.RoleProvider, nil
	case "requirer":
		return charm.RoleRequirer, nil
	case "peer":
		return charm.RolePeer, nil
	default:
		return "", errors.Errorf("unknown relation role %q", role)
	}
}

func decodeRelationScope(scope string) (charm.RelationScope, error) {
	switch scope {
	case "global":
		return charm.ScopeGlobal, nil
	case "container":
		return charm.ScopeContainer, nil
	default:
		return "", errors.Errorf("unknown relation scope %q", scope)
	}
}

func decodeExtraBindings(bindings []charmExtraBinding) map[string]charm.ExtraBinding {
	if len(bindings) == 0 {
		return nil
	}

	result := make(map[string]charm.ExtraBinding)
	for _, binding := range bindings {
		result[binding.Key] = charm.ExtraBinding{
			Name: binding.Name,
		}
	}
	return result
}

func decodeStorage(storages []charmStorage) (map[string]charm.Storage, error) {
	if len(storages) == 0 {
		return nil, nil
	}

	result := make(map[string]charm.Storage)
	for _, storage := range storages {
		// If the storage for the key already exists, then we need to merge the
		// information. Generally, this will be a property addition.
		if existing, ok := result[storage.Name]; ok {
			existing.Properties = append(existing.Properties, storage.Property)

			// Ensure we write it back to the map.
			result[storage.Name] = existing
			continue
		}

		kind, err := decodeStorageType(storage.Kind)
		if err != nil {
			return nil, errors.Errorf("cannot decode storage type %q: %w", storage.Kind, err)
		}

		// If we've got a property that isn't an empty string, then we need to
		// add it to the properties list.
		var properties []string
		if storage.Property != "" {
			properties = append(properties, storage.Property)
		}

		result[storage.Name] = charm.Storage{
			Name:        storage.Name,
			Description: storage.Description,
			Type:        kind,
			Shared:      storage.Shared,
			ReadOnly:    storage.ReadOnly,
			CountMin:    storage.CountMin,
			CountMax:    storage.CountMax,
			MinimumSize: storage.MinimumSize,
			Location:    storage.Location,
			Properties:  properties,
		}
	}
	return result, nil
}

func decodeStorageType(kind string) (charm.StorageType, error) {
	switch kind {
	case "block":
		return charm.StorageBlock, nil
	case "filesystem":
		return charm.StorageFilesystem, nil
	default:
		return "", errors.Errorf("unknown storage kind %q", kind)
	}
}

func decodeDevices(devices []charmDevice) map[string]charm.Device {
	if len(devices) == 0 {
		return nil
	}

	result := make(map[string]charm.Device)
	for _, device := range devices {
		result[device.Key] = charm.Device{
			Name:        device.Name,
			Description: device.Description,
			Type:        charm.DeviceType(device.DeviceType),
			CountMin:    device.CountMin,
			CountMax:    device.CountMax,
		}
	}
	return result
}

func decodeResources(resources []charmResource) (map[string]charm.Resource, error) {
	if len(resources) == 0 {
		return nil, nil
	}

	result := make(map[string]charm.Resource)
	for _, resource := range resources {
		kind, err := decodeResourceType(resource.Kind)
		if err != nil {
			return nil, errors.Errorf("cannot parse resource type %q: %w", resource.Kind, err)
		}

		result[resource.Name] = charm.Resource{
			Name:        resource.Name,
			Path:        resource.Path,
			Description: resource.Description,
			Type:        kind,
		}
	}
	return result, nil
}

func decodeResourceType(kind string) (charm.ResourceType, error) {
	switch kind {
	case "file":
		return charm.ResourceTypeFile, nil
	case "oci-image":
		return charm.ResourceTypeContainerImage, nil
	default:
		return "", errors.Errorf("unknown resource kind %q", kind)
	}
}

func decodeContainers(containers []charmContainer) (map[string]charm.Container, error) {
	if len(containers) == 0 {
		return nil, nil
	}

	result := make(map[string]charm.Container)
	for _, container := range containers {
		// If the container for the key already exists, then we need to merge
		// the information. Generally, this will be a mounts addition.
		if existing, ok := result[container.Key]; ok {
			existing.Mounts = append(existing.Mounts, charm.Mount{
				Storage:  container.Storage,
				Location: container.Location,
			})

			// Ensure we write it back to the map.
			result[container.Key] = existing
			continue
		}

		var uid *int
		if container.Uid >= 0 {
			uid = &container.Uid
		}
		var gid *int
		if container.Uid >= 0 {
			gid = &container.Gid
		}

		var mounts []charm.Mount
		if container.Storage != "" || container.Location != "" {
			mounts = append(mounts, charm.Mount{
				Storage:  container.Storage,
				Location: container.Location,
			})
		}

		result[container.Key] = charm.Container{
			Resource: container.Resource,
			Uid:      uid,
			Gid:      gid,
			Mounts:   mounts,
		}
	}
	return result, nil
}

func encodeMetadata(id corecharm.ID, metadata charm.Metadata) (setCharmMetadata, error) {
	runAs, err := encodeRunAs(metadata.RunAs)
	if err != nil {
		return setCharmMetadata{}, errors.Errorf("cannot encode run as %q: %w", metadata.RunAs, err)
	}

	return setCharmMetadata{
		CharmUUID:      id.String(),
		Name:           metadata.Name,
		Summary:        metadata.Summary,
		Description:    metadata.Description,
		Subordinate:    metadata.Subordinate,
		MinJujuVersion: metadata.MinJujuVersion.String(),
		RunAsID:        runAs,
		Assumes:        metadata.Assumes,
	}, nil
}

func encodeRunAs(runAs charm.RunAs) (int, error) {
	switch runAs {
	case charm.RunAsDefault, "":
		return 0, nil
	case charm.RunAsRoot:
		return 1, nil
	case charm.RunAsSudoer:
		return 2, nil
	case charm.RunAsNonRoot:
		return 3, nil
	default:
		return -1, errors.Errorf("unknown run as value %q", runAs)
	}
}

func encodeTags(id corecharm.ID, tags []string) []setCharmTag {
	var result []setCharmTag
	for i, tag := range tags {
		result = append(result, setCharmTag{
			CharmUUID: id.String(),
			Tag:       tag,
			Index:     i,
		})
	}
	return result
}

func encodeCategories(id corecharm.ID, categories []string) []setCharmCategory {
	var result []setCharmCategory
	for i, category := range categories {
		result = append(result, setCharmCategory{
			CharmUUID: id.String(),
			Category:  category,
			Index:     i,
		})
	}
	return result
}

func encodeTerms(id corecharm.ID, terms []string) []setCharmTerm {
	var result []setCharmTerm
	for i, term := range terms {
		result = append(result, setCharmTerm{
			CharmUUID: id.String(),
			Term:      term,
			Index:     i,
		})
	}
	return result
}

func encodeRelations(id corecharm.ID, metatadata charm.Metadata) ([]setCharmRelation, error) {
	var result []setCharmRelation
	for _, relation := range metatadata.Provides {
		encoded, err := encodeRelation(id, relationKindProvides, relation)
		if err != nil {
			return nil, errors.Errorf("cannot encode provides relation: %w", err)
		}
		result = append(result, encoded)
	}

	for _, relation := range metatadata.Requires {
		encoded, err := encodeRelation(id, relationKindRequires, relation)
		if err != nil {
			return nil, errors.Errorf("cannot encode requires relation: %w", err)
		}
		result = append(result, encoded)
	}

	for _, relation := range metatadata.Peers {
		encoded, err := encodeRelation(id, relationKindPeers, relation)
		if err != nil {
			return nil, errors.Errorf("cannot encode peers relation: %w", err)
		}
		result = append(result, encoded)
	}

	return result, nil
}

func encodeRelation(id corecharm.ID, kind string, relation charm.Relation) (setCharmRelation, error) {
	relationUUID, err := uuid.NewUUID()
	if err != nil {
		return setCharmRelation{}, errors.Errorf("generating relation uuid")
	}

	kindID, err := encodeRelationKind(kind)
	if err != nil {
		return setCharmRelation{}, errors.Errorf("encoding relation kind %q: %w", kind, err)
	}

	roleID, err := encodeRelationRole(relation.Role)
	if err != nil {
		return setCharmRelation{}, errors.Errorf("encoding relation role %q: %w", relation.Role, err)
	}

	scopeID, err := encodeRelationScope(relation.Scope)
	if err != nil {
		return setCharmRelation{}, errors.Errorf("encoding relation scope %q: %w", relation.Scope, err)
	}

	return setCharmRelation{
		UUID:      relationUUID.String(),
		CharmUUID: id.String(),
		KindID:    kindID,
		Key:       relation.Key,
		Name:      relation.Name,
		RoleID:    roleID,
		Interface: relation.Interface,
		Optional:  relation.Optional,
		Capacity:  relation.Limit,
		ScopeID:   scopeID,
	}, nil
}

func encodeRelationKind(kind string) (int, error) {
	// This values are hardcoded to match the index relation kind values in the
	// database.
	switch kind {
	case relationKindProvides:
		return 0, nil
	case relationKindRequires:
		return 1, nil
	case relationKindPeers:
		return 2, nil
	default:
		return -1, errors.Errorf("unknown relation kind %q", kind)
	}
}

func encodeRelationRole(role charm.RelationRole) (int, error) {
	// This values are hardcoded to match the index relation role values in the
	// database.
	switch role {
	case charm.RoleProvider:
		return 0, nil
	case charm.RoleRequirer:
		return 1, nil
	case charm.RolePeer:
		return 2, nil
	default:
		return -1, errors.Errorf("unknown relation role %q", role)
	}
}

func encodeRelationScope(scope charm.RelationScope) (int, error) {
	// This values are hardcoded to match the index relation scope values in the
	// database.
	switch scope {
	case charm.ScopeGlobal:
		return 0, nil
	case charm.ScopeContainer:
		return 1, nil
	default:
		return -1, errors.Errorf("unknown relation scope %q", scope)
	}
}

func encodeExtraBindings(id corecharm.ID, extraBindings map[string]charm.ExtraBinding) []setCharmExtraBinding {
	var result []setCharmExtraBinding
	for key, binding := range extraBindings {
		result = append(result, setCharmExtraBinding{
			CharmUUID: id.String(),
			Key:       key,
			Name:      binding.Name,
		})
	}
	return result
}

func encodeStorage(id corecharm.ID, storage map[string]charm.Storage) ([]setCharmStorage, []setCharmStorageProperty, error) {
	var (
		storages   []setCharmStorage
		properties []setCharmStorageProperty
	)
	for _, storage := range storage {
		kind, err := encodeStorageType(storage.Type)
		if err != nil {
			return nil, nil, errors.Errorf("cannot encode storage type %q: %w", storage.Type, err)
		}

		storages = append(storages, setCharmStorage{
			CharmUUID:   id.String(),
			Name:        storage.Name,
			Description: storage.Description,
			KindID:      kind,
			Shared:      storage.Shared,
			ReadOnly:    storage.ReadOnly,
			CountMin:    storage.CountMin,
			CountMax:    storage.CountMax,
			MinimumSize: storage.MinimumSize,
			Location:    storage.Location,
		})

		for i, property := range storage.Properties {
			properties = append(properties, setCharmStorageProperty{
				CharmUUID: id.String(),
				Name:      storage.Name,
				Index:     i,
				Value:     property,
			})
		}
	}
	return storages, properties, nil
}

func encodeStorageType(kind charm.StorageType) (int, error) {
	// This values are hardcoded to match the index storage type values in the
	// database.
	switch kind {
	case charm.StorageBlock:
		return 0, nil
	case charm.StorageFilesystem:
		return 1, nil
	default:
		return -1, errors.Errorf("unknown storage kind %q", kind)
	}
}

func encodeDevices(id corecharm.ID, devices map[string]charm.Device) []setCharmDevice {
	var result []setCharmDevice
	for key, device := range devices {
		result = append(result, setCharmDevice{
			CharmUUID:   id.String(),
			Key:         key,
			Name:        device.Name,
			Description: device.Description,
			// This is currently safe to do this as the device type is a string,
			// and there is no validation around what is a device type. In the
			// future, we should probably validate this.
			DeviceType: string(device.Type),
			CountMin:   device.CountMin,
			CountMax:   device.CountMax,
		})
	}
	return result
}

func encodeResources(id corecharm.ID, resources map[string]charm.Resource) ([]setCharmResource, error) {
	var result []setCharmResource
	for _, resource := range resources {
		kind, err := encodeResourceType(resource.Type)
		if err != nil {
			return nil, errors.Errorf("cannot encode resource type %q: %w", resource.Type, err)
		}

		result = append(result, setCharmResource{
			CharmUUID:   id.String(),
			Name:        resource.Name,
			KindID:      kind,
			Path:        resource.Path,
			Description: resource.Description,
		})

	}
	return result, nil
}

func encodeResourceType(kind charm.ResourceType) (int, error) {
	// This values are hardcoded to match the index resource type values in the
	// database.
	switch kind {
	case charm.ResourceTypeFile:
		return 0, nil
	case charm.ResourceTypeContainerImage:
		return 1, nil
	default:
		return -1, errors.Errorf("unknown resource kind %q", kind)
	}
}

func encodeContainers(id corecharm.ID, containerSet map[string]charm.Container) ([]setCharmContainer, []setCharmMount, error) {
	var (
		containers []setCharmContainer
		mounts     []setCharmMount
	)
	for key, container := range containerSet {
		uid := -1
		if container.Uid != nil {
			uid = *container.Uid
		}
		gid := -1
		if container.Gid != nil {
			gid = *container.Gid
		}

		containers = append(containers, setCharmContainer{
			CharmUUID: id.String(),
			Key:       key,
			Resource:  container.Resource,
			Uid:       uid,
			Gid:       gid,
		})

		for i, mount := range container.Mounts {
			mounts = append(mounts, setCharmMount{
				CharmUUID: id.String(),
				Key:       key,
				Index:     i,
				Storage:   mount.Storage,
				Location:  mount.Location,
			})
		}
	}
	return containers, mounts, nil
}
