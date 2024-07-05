// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/version/v2"

	"github.com/juju/juju/domain/charm"
)

type decodeMetadataArgs struct {
	tags          []charmTag
	categories    []charmCategory
	terms         []charmTerm
	relations     []charmRelation
	extraBindings []charmExtraBinding
	storage       []charmStorage
	devices       []charmDevice
	payloads      []charmPayload
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
			return charm.Metadata{}, fmt.Errorf("cannot parse min juju version %q: %w", metadata.MinJujuVersion, err)
		}
	}

	runAs, err := decodeRunAs(metadata.RunAs)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("cannot decode run as %q: %w", metadata.RunAs, err)
	}

	provides, requires, peer, err := decodeRelations(args.relations)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("cannot decode relations: %w", err)
	}

	storage, err := decodeStorage(args.storage)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("cannot decode storage: %w", err)
	}

	resources, err := decodeResources(args.resources)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("cannot decode resources: %w", err)
	}

	containers, err := decodeContainers(args.containers)
	if err != nil {
		return charm.Metadata{}, fmt.Errorf("cannot decode containers: %w", err)
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
		PayloadClasses: decodePayloads(args.payloads),
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
		return "", fmt.Errorf("unknown run as value %q", runAs)
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
			return charm.Relation{}, fmt.Errorf("cannot decode relation role %q: %w", relation.Role, err)
		}

		scope, err := decodeRelationScope(relation.Scope)
		if err != nil {
			return charm.Relation{}, fmt.Errorf("cannot decode relation scope %q: %w", relation.Scope, err)
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
			return nil, nil, nil, fmt.Errorf("cannot make relation: %w", err)
		}

		switch relation.Kind {
		case "provides":
			if provides == nil {
				provides = make(map[string]charm.Relation)
			}
			provides[relation.Key] = rel
		case "requires":
			if requires == nil {
				requires = make(map[string]charm.Relation)
			}
			requires[relation.Key] = rel
		case "peers":
			if peers == nil {
				peers = make(map[string]charm.Relation)
			}
			peers[relation.Key] = rel
		default:
			return nil, nil, nil, fmt.Errorf("unknown relation role %q", relation.Kind)
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
		return "", fmt.Errorf("unknown relation role %q", role)
	}
}

func decodeRelationScope(scope string) (charm.RelationScope, error) {
	switch scope {
	case "global":
		return charm.ScopeGlobal, nil
	case "container":
		return charm.ScopeContainer, nil
	default:
		return "", fmt.Errorf("unknown relation scope %q", scope)
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
		if existing, ok := result[storage.Key]; ok {
			existing.Properties = append(existing.Properties, storage.Property)

			// Ensure we write it back to the map.
			result[storage.Key] = existing
			continue
		}

		kind, err := decodeStorageType(storage.Kind)
		if err != nil {
			return nil, fmt.Errorf("cannot decode storage type %q: %w", storage.Kind, err)
		}

		// If we've got a property that isn't an empty string, then we need to
		// add it to the properties list.
		var properties []string
		if storage.Property != "" {
			properties = append(properties, storage.Property)
		}

		result[storage.Key] = charm.Storage{
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
		return "", fmt.Errorf("unknown storage kind %q", kind)
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

func decodePayloads(payloads []charmPayload) map[string]charm.PayloadClass {
	if len(payloads) == 0 {
		return nil
	}

	result := make(map[string]charm.PayloadClass)
	for _, payload := range payloads {
		result[payload.Key] = charm.PayloadClass{
			Name: payload.Name,
			Type: payload.Type,
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
			return nil, fmt.Errorf("cannot parse resource type %q: %w", resource.Kind, err)
		}

		result[resource.Key] = charm.Resource{
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
		return "", fmt.Errorf("unknown resource kind %q", kind)
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
