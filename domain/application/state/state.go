// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
)

// State represents a type for interacting with the underlying state.
// Composes both application and charm state, so we can interact with both
// from the single state, whilst also keeping the concerns separate.
type State struct {
	*ApplicationState
	*CharmState
}

type commonStateBase struct {
	*domain.StateBase
}

func (s *commonStateBase) checkChamReferenceExists(ctx context.Context, tx *sqlair.TX, referenceName string, revision int) (corecharm.ID, error) {
	selectQuery := `
SELECT &charmIDName.*
FROM charm
LEFT JOIN charm_origin
WHERE charm.uuid = charm_origin.charm_uuid
AND charm_origin.reference_name = $charmReferenceNameRevision.reference_name 
AND charm_origin.revision = $charmReferenceNameRevision.revision
	`
	ref := charmReferenceNameRevision{
		ReferenceName: referenceName,
		Revision:      revision,
	}

	var result charmIDName
	selectStmt, err := s.Prepare(selectQuery, result, ref)
	if err != nil {
		return "", fmt.Errorf("failed to prepare query: %w", err)
	}
	if err := tx.Query(ctx, selectStmt, ref).Get(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to check charm exists: %w", err)
	}

	id, err := corecharm.ParseID(result.UUID)
	if err != nil {
		return "", fmt.Errorf("failed to parse charm ID: %w", err)
	}

	return id, applicationerrors.CharmAlreadyExists
}

func (s *commonStateBase) setCharm(ctx context.Context, tx *sqlair.TX, id corecharm.ID, charm charm.Charm, archivePath string) error {
	if err := s.setCharmMetadata(ctx, tx, id, charm.Metadata, charm.LXDProfile, archivePath); err != nil {
		return errors.Trace(err)
	}

	if err := s.setCharmTags(ctx, tx, id, charm.Metadata.Tags); err != nil {
		return errors.Trace(err)
	}

	if err := s.setCharmCategories(ctx, tx, id, charm.Metadata.Categories); err != nil {
		return errors.Trace(err)
	}

	if err := s.setCharmTerms(ctx, tx, id, charm.Metadata.Terms); err != nil {
		return errors.Trace(err)
	}

	if err := s.setCharmRelations(ctx, tx, id, charm.Metadata); err != nil {
		return errors.Trace(err)
	}

	if err := s.setCharmExtraBindings(ctx, tx, id, charm.Metadata.ExtraBindings); err != nil {
		return errors.Trace(err)
	}

	if err := s.setCharmStorage(ctx, tx, id, charm.Metadata.Storage); err != nil {
		return errors.Trace(err)
	}

	if err := s.setCharmDevices(ctx, tx, id, charm.Metadata.Devices); err != nil {
		return errors.Trace(err)
	}

	if err := s.setCharmPayloads(ctx, tx, id, charm.Metadata.PayloadClasses); err != nil {
		return errors.Trace(err)
	}

	if err := s.setCharmResources(ctx, tx, id, charm.Metadata.Resources); err != nil {
		return errors.Trace(err)
	}

	if err := s.setCharmContainers(ctx, tx, id, charm.Metadata.Containers); err != nil {
		return errors.Trace(err)
	}

	if err := s.setCharmActions(ctx, tx, id, charm.Actions); err != nil {
		return errors.Trace(err)
	}

	if err := s.setCharmConfig(ctx, tx, id, charm.Config); err != nil {
		return errors.Trace(err)
	}

	if err := s.setCharmManifest(ctx, tx, id, charm.Manifest); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (s *commonStateBase) setCharmMetadata(
	ctx context.Context,
	tx *sqlair.TX,
	id corecharm.ID,
	metadata charm.Metadata,
	lxdProfile []byte,
	archivePath string) error {
	ident := charmID{UUID: id.String()}

	encodedMetadata, err := encodeMetadata(id, metadata, lxdProfile, archivePath)
	if err != nil {
		return fmt.Errorf("failed to encode charm metadata: %w", err)
	}

	query := `INSERT INTO charm (*) VALUES ($setCharmMetadata.*);`
	stmt, err := s.Prepare(query, encodedMetadata, ident)
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedMetadata).Run(); err != nil {
		return fmt.Errorf("failed to insert charm metadata: %w", err)
	}

	return nil
}

func (s *commonStateBase) setCharmTags(ctx context.Context, tx *sqlair.TX, id corecharm.ID, tags []string) error {
	// If there are no tags, we don't need to do anything.
	if len(tags) == 0 {
		return nil
	}

	query := `INSERT INTO charm_tag (*) VALUES ($setCharmTag.*);`
	stmt, err := s.Prepare(query, setCharmTag{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodeTags(id, tags)).Run(); err != nil {
		return fmt.Errorf("failed to insert charm tag: %w", err)
	}

	return nil
}

func (s *commonStateBase) setCharmCategories(ctx context.Context, tx *sqlair.TX, id corecharm.ID, categories []string) error {
	// If there are no categories, we don't need to do anything.
	if len(categories) == 0 {
		return nil
	}

	query := `INSERT INTO charm_category (*) VALUES ($setCharmCategory.*);`
	stmt, err := s.Prepare(query, setCharmCategory{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodeCategories(id, categories)).Run(); err != nil {
		return fmt.Errorf("failed to insert charm categories: %w", err)
	}

	return nil
}

func (s *commonStateBase) setCharmTerms(ctx context.Context, tx *sqlair.TX, id corecharm.ID, terms []string) error {
	// If there are no terms, we don't need to do anything.
	if len(terms) == 0 {
		return nil
	}

	query := `INSERT INTO charm_term (*) VALUES ($setCharmTerm.*);`
	stmt, err := s.Prepare(query, setCharmTerm{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodeTerms(id, terms)).Run(); err != nil {
		return fmt.Errorf("failed to insert charm terms: %w", err)
	}

	return nil
}

func (s *commonStateBase) setCharmRelations(ctx context.Context, tx *sqlair.TX, id corecharm.ID, metadata charm.Metadata) error {
	encodedRelations, err := encodeRelations(id, metadata)
	if err != nil {
		return fmt.Errorf("failed to encode charm relations: %w", err)
	}

	// If there are no relations, we don't need to do anything.
	if len(encodedRelations) == 0 {
		return nil
	}

	query := `INSERT INTO charm_relation (*) VALUES ($setCharmRelation.*);`
	stmt, err := s.Prepare(query, setCharmRelation{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedRelations).Run(); err != nil {
		return fmt.Errorf("failed to insert charm relations: %w", err)
	}

	return nil
}

func (s *commonStateBase) setCharmExtraBindings(ctx context.Context, tx *sqlair.TX, id corecharm.ID, extraBindings map[string]charm.ExtraBinding) error {
	// If there is no extraBindings, we don't need to do anything.
	if len(extraBindings) == 0 {
		return nil
	}

	query := `INSERT INTO charm_extra_binding (*) VALUES ($setCharmExtraBinding.*);`
	stmt, err := s.Prepare(query, setCharmExtraBinding{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodeExtraBindings(id, extraBindings)).Run(); err != nil {
		return fmt.Errorf("failed to insert charm extra bindings: %w", err)
	}

	return nil
}

func (s *commonStateBase) setCharmStorage(ctx context.Context, tx *sqlair.TX, id corecharm.ID, storage map[string]charm.Storage) error {
	// If there is no storage, we don't need to do anything.
	if len(storage) == 0 {
		return nil
	}

	encodedStorage, encodedProperties, err := encodeStorage(id, storage)
	if err != nil {
		return fmt.Errorf("failed to encode charm storage: %w", err)
	}

	storageQuery := `INSERT INTO charm_storage (*) VALUES ($setCharmStorage.*);`
	storageStmt, err := s.Prepare(storageQuery, setCharmStorage{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, storageStmt, encodedStorage).Run(); err != nil {
		return fmt.Errorf("failed to insert charm storage: %w", err)
	}

	// Only insert properties if there are any.
	if len(encodedProperties) > 0 {
		propertiesQuery := `INSERT INTO charm_storage_property (*) VALUES ($setCharmStorageProperty.*);`
		propertiesStmt, err := s.Prepare(propertiesQuery, setCharmStorageProperty{})
		if err != nil {
			return fmt.Errorf("failed to prepare query: %w", err)
		}

		if err := tx.Query(ctx, propertiesStmt, encodedProperties).Run(); err != nil {
			return fmt.Errorf("failed to insert charm storage properties: %w", err)
		}
	}
	return nil
}

func (s *commonStateBase) setCharmDevices(ctx context.Context, tx *sqlair.TX, id corecharm.ID, devices map[string]charm.Device) error {
	// If there are no devices, we don't need to do anything.
	if len(devices) == 0 {
		return nil
	}

	query := `INSERT INTO charm_device (*) VALUES ($setCharmDevice.*);`
	stmt, err := s.Prepare(query, setCharmDevice{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodeDevices(id, devices)).Run(); err != nil {
		return fmt.Errorf("failed to insert charm devices: %w", err)
	}

	return nil
}

func (s *commonStateBase) setCharmPayloads(ctx context.Context, tx *sqlair.TX, id corecharm.ID, payloads map[string]charm.PayloadClass) error {
	// If there are no payloads, we don't need to do anything.
	if len(payloads) == 0 {
		return nil
	}

	query := `INSERT INTO charm_payload (*) VALUES ($setCharmPayload.*);`
	stmt, err := s.Prepare(query, setCharmPayload{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodePayloads(id, payloads)).Run(); err != nil {
		return fmt.Errorf("failed to insert charm payloads: %w", err)
	}

	return nil
}

func (s *commonStateBase) setCharmResources(ctx context.Context, tx *sqlair.TX, id corecharm.ID, resources map[string]charm.Resource) error {
	// If there are no resources, we don't need to do anything.
	if len(resources) == 0 {
		return nil
	}

	encodedResources, err := encodeResources(id, resources)
	if err != nil {
		return fmt.Errorf("failed to encode charm resources: %w", err)
	}

	query := `INSERT INTO charm_resource (*) VALUES ($setCharmResource.*);`
	stmt, err := s.Prepare(query, setCharmResource{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedResources).Run(); err != nil {
		return fmt.Errorf("failed to insert charm resources: %w", err)
	}

	return nil
}

func (s *commonStateBase) setCharmContainers(ctx context.Context, tx *sqlair.TX, id corecharm.ID, containers map[string]charm.Container) error {
	// If there are no containers, we don't need to do anything.
	if len(containers) == 0 {
		return nil
	}

	encodedContainers, encodedMounts, err := encodeContainers(id, containers)
	if err != nil {
		return fmt.Errorf("failed to encode charm containers: %w", err)
	}

	containerQuery := `INSERT INTO charm_container (*) VALUES ($setCharmContainer.*);`
	containerStmt, err := s.Prepare(containerQuery, setCharmContainer{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, containerStmt, encodedContainers).Run(); err != nil {
		return fmt.Errorf("failed to insert charm containers: %w", err)
	}

	// Only insert mounts if there are any.
	if len(encodedMounts) > 0 {
		mountQuery := `INSERT INTO charm_container_mount (*) VALUES ($setCharmMount.*);`
		mountStmt, err := s.Prepare(mountQuery, setCharmMount{})
		if err != nil {
			return fmt.Errorf("failed to prepare query: %w", err)
		}

		if err := tx.Query(ctx, mountStmt, encodedMounts).Run(); err != nil {
			return fmt.Errorf("failed to insert charm container mounts: %w", err)
		}
	}

	return nil
}

func (s *commonStateBase) setCharmActions(ctx context.Context, tx *sqlair.TX, id corecharm.ID, actions charm.Actions) error {
	// If there are no resources, we don't need to do anything.
	if len(actions.Actions) == 0 {
		return nil
	}

	query := `INSERT INTO charm_action (*) VALUES ($setCharmAction.*);`
	stmt, err := s.Prepare(query, setCharmAction{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodeActions(id, actions)).Run(); err != nil {
		return fmt.Errorf("failed to insert charm actions: %w", err)
	}

	return nil
}

func (s *commonStateBase) setCharmConfig(ctx context.Context, tx *sqlair.TX, id corecharm.ID, config charm.Config) error {
	// If there are no resources, we don't need to do anything.
	if len(config.Options) == 0 {
		return nil
	}

	encodedConfig, err := encodeConfig(id, config)
	if err != nil {
		return fmt.Errorf("failed to encode charm config: %w", err)
	}

	query := `INSERT INTO charm_config (*) VALUES ($setCharmConfig.*);`
	stmt, err := s.Prepare(query, setCharmConfig{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedConfig).Run(); err != nil {
		return fmt.Errorf("failed to insert charm config: %w", err)
	}

	return nil
}

func (s *commonStateBase) setCharmManifest(ctx context.Context, tx *sqlair.TX, id corecharm.ID, manifest charm.Manifest) error {
	// If there are no resources, we don't need to do anything.
	if len(manifest.Bases) == 0 {
		return nil
	}

	encodedManifest, err := encodeManifest(id, manifest)
	if err != nil {
		return fmt.Errorf("failed to encode charm manifest: %w", err)
	}

	query := `INSERT INTO charm_manifest_base (*) VALUES ($setCharmManifest.*);`
	stmt, err := s.Prepare(query, setCharmManifest{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedManifest).Run(); err != nil {
		return fmt.Errorf("failed to insert charm manifest: %w", err)
	}

	return nil
}

// getCharmOrigin returns the charm origin for the given charm ID.
func (s *commonStateBase) getCharmOrigin(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.CharmOrigin, error) {
	originQuery := `
SELECT &charmOrigin.*
FROM v_charm_origin
WHERE charm_uuid = $charmID.uuid;
`
	platformQuery := `
SELECT &charmPlatform.*
FROM charm_platform
WHERE charm_uuid = $charmID.uuid;
`

	originStmt, err := s.Prepare(originQuery, charmOrigin{}, ident)
	if err != nil {
		return charm.CharmOrigin{}, fmt.Errorf("failed to prepare query: %w", err)
	}

	platformStmt, err := s.Prepare(platformQuery, charmPlatform{}, ident)
	if err != nil {
		return charm.CharmOrigin{}, fmt.Errorf("failed to prepare query: %w", err)
	}

	var origin charmOrigin
	if err := tx.Query(ctx, originStmt, ident).Get(&origin); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.CharmOrigin{}, applicationerrors.CharmNotFound
		}
		return charm.CharmOrigin{}, fmt.Errorf("failed to get charm origin: %w", err)
	}

	var platform charmPlatform
	if err := tx.Query(ctx, platformStmt, ident).Get(&platform); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.CharmOrigin{}, applicationerrors.CharmNotFound
		}
		return charm.CharmOrigin{}, fmt.Errorf("failed to get charm platform: %w", err)
	}

	return decodeCharmOrigin(origin, platform)
}

// getCharm returns the charm for the given charm ID.
// This will delegate to the various get methods to get the charm metadata,
// config, manifest, actions and LXD profile.
func (s *commonStateBase) getCharm(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Charm, error) {
	var (
		charm charm.Charm
		err   error
	)
	if charm.Metadata, err = s.getMetadata(ctx, tx, ident); err != nil {
		return charm, errors.Trace(err)
	}

	if charm.Config, err = s.getCharmConfig(ctx, tx, ident); err != nil {
		return charm, errors.Trace(err)
	}

	if charm.Manifest, err = s.getCharmManifest(ctx, tx, ident); err != nil {
		return charm, errors.Trace(err)
	}

	if charm.Actions, err = s.getCharmActions(ctx, tx, ident); err != nil {
		return charm, errors.Trace(err)
	}

	if charm.LXDProfile, err = s.getCharmLXDProfile(ctx, tx, ident); err != nil {
		return charm, errors.Trace(err)
	}
	return charm, nil
}

// getMetadata returns the metadata for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *commonStateBase) getMetadata(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Metadata, error) {
	// Unlike other domain methods, we're not constructing a row struct here.
	// Attempting to get the metadata as a series of rows will yield potentially
	// hundreds of rows, which is not what we want. This is because of all the
	// nested maps and slices in the metadata.
	// Instead, we'll call the metadata in multiple calls, but in one
	// transaction.
	// This will then open up the possibility of using these methods to
	// get specific parts of the metadata in the future (e.g. GetCharmRelations)

	var (
		metadata      charmMetadata
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
	)

	var err error
	if metadata, err = s.getCharmMetadata(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	if tags, err = s.getCharmTags(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	if categories, err = s.getCharmCategories(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	if terms, err = s.getCharmTerms(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	if relations, err = s.getCharmRelations(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	if extraBindings, err = s.getCharmExtraBindings(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	if storage, err = s.getCharmStorage(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	if devices, err = s.getCharmDevices(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	if payloads, err = s.getCharmPayloads(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	if resources, err = s.getCharmResources(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	if containers, err = s.getCharmContainers(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	return decodeMetadata(metadata, decodeMetadataArgs{
		tags:          tags,
		categories:    categories,
		terms:         terms,
		relations:     relations,
		extraBindings: extraBindings,
		storage:       storage,
		devices:       devices,
		payloads:      payloads,
		resources:     resources,
		containers:    containers,
	})
}

// getCharmManifest returns the manifest for the charm, using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *commonStateBase) getCharmManifest(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Manifest, error) {
	query := `
SELECT &charmManifest.*
FROM v_charm_manifest
WHERE charm_uuid = $charmID.uuid
ORDER BY array_index ASC, nested_array_index ASC;
`

	stmt, err := s.Prepare(query, charmManifest{}, ident)
	if err != nil {
		return charm.Manifest{}, fmt.Errorf("failed to prepare query: %w", err)
	}

	var manifests []charmManifest
	if err := tx.Query(ctx, stmt, ident).GetAll(&manifests); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.Manifest{}, applicationerrors.CharmNotFound
		}
		return charm.Manifest{}, fmt.Errorf("failed to get charm manifest: %w", err)
	}

	return decodeManifest(manifests)
}

// getCharmLXDProfile returns the LXD profile for the charm using the
// charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *commonStateBase) getCharmLXDProfile(ctx context.Context, tx *sqlair.TX, ident charmID) ([]byte, error) {
	query := `
SELECT &charmLXDProfile.*
FROM charm
WHERE uuid = $charmID.uuid;
	`

	var profile charmLXDProfile
	stmt, err := s.Prepare(query, profile, ident)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, ident).Get(&profile); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, applicationerrors.CharmNotFound
		}
		return nil, fmt.Errorf("failed to get charm lxd profile: %w", err)
	}

	return profile.LXDProfile, nil
}

// getCharmConfig returns the config for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *commonStateBase) getCharmConfig(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Config, error) {
	charmQuery := `
SELECT &charmID.*
FROM charm
WHERE uuid = $charmID.uuid;
`
	configQuery := `
SELECT &charmConfig.*
FROM v_charm_config
WHERE charm_uuid = $charmID.uuid;
`

	charmStmt, err := s.Prepare(charmQuery, ident)
	if err != nil {
		return charm.Config{}, fmt.Errorf("failed to prepare charm query: %w", err)
	}
	configStmt, err := s.Prepare(configQuery, charmConfig{}, ident)
	if err != nil {
		return charm.Config{}, fmt.Errorf("failed to prepare config query: %w", err)
	}

	if err := tx.Query(ctx, charmStmt, ident).Get(&ident); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.Config{}, applicationerrors.CharmNotFound
		}
	}

	var configs []charmConfig
	if err := tx.Query(ctx, configStmt, ident).GetAll(&configs); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.Config{}, nil
		}
		return charm.Config{}, fmt.Errorf("failed to get charm config: %w", err)
	}

	return decodeConfig(configs)
}

// getCharmActions returns the actions for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *commonStateBase) getCharmActions(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Actions, error) {
	charmQuery := `
SELECT &charmID.*
FROM charm
WHERE uuid = $charmID.uuid;
	`
	actionQuery := `
SELECT &charmAction.*
FROM charm_action
WHERE charm_uuid = $charmID.uuid;
	`

	charmStmt, err := s.Prepare(charmQuery, ident)
	if err != nil {
		return charm.Actions{}, fmt.Errorf("failed to prepare charm query: %w", err)
	}
	actionsStmt, err := s.Prepare(actionQuery, charmAction{}, ident)
	if err != nil {
		return charm.Actions{}, fmt.Errorf("failed to prepare action query: %w", err)
	}

	if err := tx.Query(ctx, charmStmt, ident).Get(&ident); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.Actions{}, applicationerrors.CharmNotFound
		}
	}

	var actions []charmAction
	if err := tx.Query(ctx, actionsStmt, ident).GetAll(&actions); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.Actions{}, nil
		}
		return charm.Actions{}, fmt.Errorf("failed to get charm actions: %w", err)
	}

	return decodeActions(actions), nil
}

// getCharmMetadata returns the metadata for the charm using the charm ID.
// This is the core metadata for the charm.
func (s *commonStateBase) getCharmMetadata(ctx context.Context, tx *sqlair.TX, ident charmID) (charmMetadata, error) {
	query := `
SELECT &charmMetadata.*
FROM v_charm
WHERE uuid = $charmID.uuid;
`
	var metadata charmMetadata
	stmt, err := s.Prepare(query, metadata, ident)
	if err != nil {
		return metadata, fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, ident).Get(&metadata); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return metadata, applicationerrors.CharmNotFound
		}
		return metadata, fmt.Errorf("failed to select charm metadata: %w", err)
	}

	return metadata, nil
}

// getCharmTags returns the tags for the charm using the charm ID.
// This is a slice of charmTags and not a slice of strings, because sqlair
// doesn't work on scalar types. If the sqlair library gains support for scalar
// types, this can be changed.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
// Tags are expected to be unique, no duplicates are expected.
func (s *commonStateBase) getCharmTags(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmTag, error) {
	query := `
SELECT &charmTag.*
FROM charm_tag
WHERE charm_uuid = $charmID.uuid
ORDER BY array_index ASC;
`
	stmt, err := s.Prepare(query, charmTag{}, ident)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	var result []charmTag
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, fmt.Errorf("failed to select charm tags: %w", err)
	}

	return result, nil
}

// getCharmCategories returns the categories for the charm using the charm ID.
// This is a slice of charmCategories and not a slice of strings, because sqlair
// doesn't work on scalar types. If the sqlair library gains support for scalar
// types, this can be changed.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
// Categories are expected to be unique, no duplicates are expected.
func (s *commonStateBase) getCharmCategories(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmCategory, error) {
	query := `
SELECT &charmCategory.*
FROM charm_category
WHERE charm_uuid = $charmID.uuid
ORDER BY array_index ASC;
`
	stmt, err := s.Prepare(query, charmCategory{}, ident)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	var result []charmCategory
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, fmt.Errorf("failed to select charm categories: %w", err)
	}

	return result, nil
}

// getCharmTerms returns the terms for the charm using the charm ID.
// This is a slice of charmTerms and not a slice of strings, because sqlair
// doesn't work on scalar types. If the sqlair library gains support for scalar
// types, this can be changed.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
// Terms are expected to be unique, no duplicates are expected.
func (s *commonStateBase) getCharmTerms(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmTerm, error) {
	query := `
SELECT &charmTerm.*
FROM charm_term
WHERE charm_uuid = $charmID.uuid
ORDER BY array_index ASC;
`
	stmt, err := s.Prepare(query, charmTerm{}, ident)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	var result []charmTerm
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, fmt.Errorf("failed to select charm terms: %w", err)
	}

	return result, nil
}

// getCharmRelations returns the relations for the charm using the charm ID.
// This is a slice of all the relations for the charm. Additional processing
// is required to separate the relations into provides, requires and peers.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
func (s *commonStateBase) getCharmRelations(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmRelation, error) {
	query := `
SELECT &charmRelation.*
FROM v_charm_relation
WHERE charm_uuid = $charmID.uuid;
	`
	stmt, err := s.Prepare(query, charmRelation{}, ident)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	var result []charmRelation
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, fmt.Errorf("failed to select charm relations: %w", err)
	}

	return result, nil
}

// getCharmExtraBindings returns the extra bindings for the charm using the
// charm ID. This is a slice of charmExtraBinding and not a map of string to
// string, because sqlair doesn't work on scalar types. If the sqlair library
// gains support for scalar types, this can be changed.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
func (s *commonStateBase) getCharmExtraBindings(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmExtraBinding, error) {
	query := `
SELECT &charmExtraBinding.*
FROM charm_extra_binding
WHERE charm_uuid = $charmID.uuid;
`

	stmt, err := s.Prepare(query, charmExtraBinding{}, ident)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	var result []charmExtraBinding
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, fmt.Errorf("failed to select charm extra bindings: %w", err)
	}

	return result, nil
}

// getCharmStorage returns the storage for the charm using the charm ID.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
// Charm properties are expected to be unique, no duplicates are expected.
func (s *commonStateBase) getCharmStorage(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmStorage, error) {
	query := `
SELECT &charmStorage.*
FROM v_charm_storage
WHERE charm_uuid = $charmID.uuid
ORDER BY property_index ASC;
`

	stmt, err := s.Prepare(query, charmStorage{}, ident)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	var result []charmStorage
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, fmt.Errorf("failed to select charm storage: %w", err)
	}

	return result, nil
}

// getCharmDevices returns the devices for the charm using the charm ID.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
func (s *commonStateBase) getCharmDevices(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmDevice, error) {
	query := `
SELECT &charmDevice.*
FROM charm_device
WHERE charm_uuid = $charmID.uuid;
`

	stmt, err := s.Prepare(query, charmDevice{}, ident)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	var result []charmDevice
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, fmt.Errorf("failed to select charm device: %w", err)
	}

	return result, nil
}

// getCharmPayloads returns the payloads for the charm using the charm ID.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
func (s *commonStateBase) getCharmPayloads(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmPayload, error) {
	query := `
SELECT &charmPayload.*
FROM charm_payload
WHERE charm_uuid = $charmID.uuid;
`

	stmt, err := s.Prepare(query, charmPayload{}, ident)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	var result []charmPayload
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, fmt.Errorf("failed to select charm payload: %w", err)
	}

	return result, nil
}

// getCharmResources returns the resources for the charm using the charm ID.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
func (s *commonStateBase) getCharmResources(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmResource, error) {
	query := `
SELECT &charmResource.*
FROM v_charm_resource
WHERE charm_uuid = $charmID.uuid;
`

	stmt, err := s.Prepare(query, charmResource{}, ident)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	var result []charmResource
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, fmt.Errorf("failed to select charm payload: %w", err)
	}

	return result, nil
}

// getCharmContainers returns the containers for the charm using the charm ID.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
func (s *commonStateBase) getCharmContainers(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmContainer, error) {
	query := `
SELECT &charmContainer.*
FROM v_charm_container
WHERE charm_uuid = $charmID.uuid
ORDER BY array_index ASC;
`

	stmt, err := s.Prepare(query, charmContainer{}, ident)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	var result []charmContainer
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, fmt.Errorf("failed to select charm container: %w", err)
	}

	return result, nil
}
