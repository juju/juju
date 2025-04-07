// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

type State struct {
	*domain.StateBase
	clock  clock.Clock
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory, clock clock.Clock, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		clock:     clock,
		logger:    logger,
	}
}

func (s *State) checkCharmExists(ctx context.Context, tx *sqlair.TX, id charmID) error {
	selectQuery := `
SELECT &charmID.*
FROM charm
WHERE uuid = $charmID.uuid;
	`

	result := charmID{UUID: id.UUID}
	selectStmt, err := s.Prepare(selectQuery, result)
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}
	if err := tx.Query(ctx, selectStmt, result).Get(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.CharmNotFound
		}
		return errors.Errorf("failed to check charm exists: %w", err)
	}

	return nil
}

// checkCharmReferenceExists checks if a charm with the given reference name and
// revision exists in the database.
//
//   - If the charm doesn't exists, it returns an empty id and no error
//   - If the charm exists, it returns the id and the error
//     [applicationerrors.CharmAlreadyExists]
//   - Any other error are returned if the check fails
func (s *State) checkCharmReferenceExists(ctx context.Context, tx *sqlair.TX, referenceName string, revision int) (corecharm.ID, error) {
	selectQuery := `
SELECT &charmID.*
FROM charm
WHERE reference_name = $charmReferenceNameRevisionSource.reference_name 
AND revision = $charmReferenceNameRevisionSource.revision
	`
	ref := charmReferenceNameRevisionSource{
		ReferenceName: referenceName,
		Revision:      revision,
	}

	var result charmID
	selectStmt, err := s.Prepare(selectQuery, result, ref)
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}
	if err := tx.Query(ctx, selectStmt, ref).Get(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return "", nil
		}
		return "", errors.Errorf("failed to check charm exists: %w", err)
	}

	return result.UUID, applicationerrors.CharmAlreadyExists
}

func (s *State) setCharm(ctx context.Context, tx *sqlair.TX, uuid corecharm.ID, ch charm.Charm, downloadInfo *charm.DownloadInfo) error {
	if err := s.setCharmState(ctx, tx, uuid, ch); err != nil {
		return errors.Capture(err)
	}

	if err := s.setCharmMetadata(ctx, tx, uuid, ch.Metadata); err != nil {
		return errors.Capture(err)
	}

	if err := s.setCharmTags(ctx, tx, uuid, ch.Metadata.Tags); err != nil {
		return errors.Capture(err)
	}

	if err := s.setCharmCategories(ctx, tx, uuid, ch.Metadata.Categories); err != nil {
		return errors.Capture(err)
	}

	if err := s.setCharmTerms(ctx, tx, uuid, ch.Metadata.Terms); err != nil {
		return errors.Capture(err)
	}

	if err := s.setCharmRelations(ctx, tx, uuid, ch.Metadata); err != nil {
		return errors.Capture(err)
	}

	if err := s.setCharmExtraBindings(ctx, tx, uuid, ch.Metadata.ExtraBindings); err != nil {
		return errors.Capture(err)
	}

	if err := s.setCharmStorage(ctx, tx, uuid, ch.Metadata.Storage); err != nil {
		return errors.Capture(err)
	}

	if err := s.setCharmDevices(ctx, tx, uuid, ch.Metadata.Devices); err != nil {
		return errors.Capture(err)
	}

	if err := s.setCharmResources(ctx, tx, uuid, ch.Metadata.Resources); err != nil {
		return errors.Capture(err)
	}

	if err := s.setCharmContainers(ctx, tx, uuid, ch.Metadata.Containers); err != nil {
		return errors.Capture(err)
	}

	if err := s.setCharmActions(ctx, tx, uuid, ch.Actions); err != nil {
		return errors.Capture(err)
	}

	if err := s.setCharmConfig(ctx, tx, uuid, ch.Config); err != nil {
		return errors.Capture(err)
	}

	if err := s.setCharmManifest(ctx, tx, uuid, ch.Manifest); err != nil {
		return errors.Capture(err)
	}

	// Insert the download info if the charm is from CharmHub.
	if ch.Source == charm.CharmHubSource {
		if err := s.setCharmDownloadInfo(ctx, tx, uuid, downloadInfo); err != nil {
			return errors.Capture(err)
		}
	}

	return nil
}

func (s *State) setCharmState(
	ctx context.Context,
	tx *sqlair.TX,
	id corecharm.ID,
	ch charm.Charm,
) error {
	sourceID, err := encodeCharmSource(ch.Source)
	if err != nil {
		return errors.Errorf("encoding charm source: %w", err)
	}

	architectureID, err := encodeArchitecture(ch.Architecture)
	if err != nil {
		return errors.Errorf("encoding charm architecture: %w", err)
	}

	nullableArchitectureID := sql.NullInt64{}
	if architectureID >= 0 {
		nullableArchitectureID = sql.NullInt64{
			Int64: int64(architectureID),
			Valid: true,
		}
	}

	nullableObjectStoreUUID := sql.NullString{}
	if !ch.ObjectStoreUUID.IsEmpty() {
		nullableObjectStoreUUID = sql.NullString{
			String: ch.ObjectStoreUUID.String(),
			Valid:  true,
		}
	}

	chState := setCharmState{
		UUID:            id.String(),
		ReferenceName:   ch.ReferenceName,
		Revision:        ch.Revision,
		ArchivePath:     ch.ArchivePath,
		ObjectStoreUUID: nullableObjectStoreUUID,
		Available:       ch.Available,
		Version:         ch.Version,
		SourceID:        sourceID,
		ArchitectureID:  nullableArchitectureID,
		LXDProfile:      ch.LXDProfile,
	}

	charmQuery := `INSERT INTO charm (*) VALUES ($setCharmState.*);`
	charmStmt, err := s.Prepare(charmQuery, chState)
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	hash := setCharmHash{
		CharmUUID:  id.String(),
		HashKindID: 0,
		Hash:       ch.Hash,
	}

	hashQuery := `INSERT INTO charm_hash (*) VALUES ($setCharmHash.*);`
	hashStmt, err := s.Prepare(hashQuery, hash)
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, charmStmt, chState).Run(); err != nil {
		return errors.Errorf("inserting charm state: %w", err)
	}

	if err := tx.Query(ctx, hashStmt, hash).Run(); err != nil {
		return errors.Errorf("inserting charm hash: %w", err)
	}

	return nil
}

func (s *State) setCharmDownloadInfo(ctx context.Context, tx *sqlair.TX, id corecharm.ID, downloadInfo *charm.DownloadInfo) error {
	if downloadInfo == nil {
		return nil
	}

	provenance, err := encodeProvenance(downloadInfo.Provenance)
	if err != nil {
		return errors.Errorf("encoding charm provenance: %w", err)
	}

	downloadInfoState := setCharmDownloadInfo{
		CharmUUID:          id.String(),
		ProvenanceID:       provenance,
		CharmhubIdentifier: downloadInfo.CharmhubIdentifier,
		DownloadURL:        downloadInfo.DownloadURL,
		DownloadSize:       downloadInfo.DownloadSize,
	}

	// If the charmDownloadInfo has already been inserted, we don't need to do
	// anything. We want to keep the original download information.
	query := `
INSERT INTO charm_download_info (*) 
VALUES ($setCharmDownloadInfo.*)
ON CONFLICT DO NOTHING;`
	stmt, err := s.Prepare(query, downloadInfoState)
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, downloadInfoState).Run(); err != nil {
		return errors.Errorf("inserting charm download info: %w", err)
	}

	return nil
}

func (s *State) setCharmMetadata(
	ctx context.Context,
	tx *sqlair.TX,
	id corecharm.ID,
	metadata charm.Metadata,
) error {
	encodedMetadata, err := encodeMetadata(id, metadata)
	if err != nil {
		return errors.Errorf("encoding charm metadata: %w", err)
	}

	query := `INSERT INTO charm_metadata (*) VALUES ($setCharmMetadata.*);`
	stmt, err := s.Prepare(query, encodedMetadata)
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedMetadata).Run(); err != nil {
		return errors.Errorf("inserting charm metadata: %w", err)
	}

	return nil
}

func (s *State) setCharmTags(ctx context.Context, tx *sqlair.TX, id corecharm.ID, tags []string) error {
	// If there are no tags, we don't need to do anything.
	if len(tags) == 0 {
		return nil
	}

	query := `INSERT INTO charm_tag (*) VALUES ($setCharmTag.*);`
	stmt, err := s.Prepare(query, setCharmTag{})
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodeTags(id, tags)).Run(); err != nil {
		return errors.Errorf("inserting charm tag: %w", err)
	}

	return nil
}

func (s *State) setCharmCategories(ctx context.Context, tx *sqlair.TX, id corecharm.ID, categories []string) error {
	// If there are no categories, we don't need to do anything.
	if len(categories) == 0 {
		return nil
	}

	query := `INSERT INTO charm_category (*) VALUES ($setCharmCategory.*);`
	stmt, err := s.Prepare(query, setCharmCategory{})
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodeCategories(id, categories)).Run(); err != nil {
		return errors.Errorf("inserting charm categories: %w", err)
	}

	return nil
}

func (s *State) setCharmTerms(ctx context.Context, tx *sqlair.TX, id corecharm.ID, terms []string) error {
	// If there are no terms, we don't need to do anything.
	if len(terms) == 0 {
		return nil
	}

	query := `INSERT INTO charm_term (*) VALUES ($setCharmTerm.*);`
	stmt, err := s.Prepare(query, setCharmTerm{})
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodeTerms(id, terms)).Run(); err != nil {
		return errors.Errorf("inserting charm terms: %w", err)
	}

	return nil
}

func (s *State) setCharmRelations(ctx context.Context, tx *sqlair.TX, id corecharm.ID, metadata charm.Metadata) error {
	encodedRelations, err := encodeRelations(id, metadata)
	if err != nil {
		return errors.Errorf("encoding charm relations: %w", err)
	}

	// If there are no relations, we don't need to do anything.
	if len(encodedRelations) == 0 {
		return nil
	}

	query := `INSERT INTO charm_relation (*) VALUES ($setCharmRelation.*);`
	stmt, err := s.Prepare(query, setCharmRelation{})
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedRelations).Run(); internaldatabase.IsErrConstraintUnique(err) {
		return applicationerrors.CharmRelationNameConflict
	} else if err != nil {
		return errors.Errorf("inserting charm relations: %w", err)
	}

	return nil
}

func (s *State) setCharmExtraBindings(ctx context.Context, tx *sqlair.TX, id corecharm.ID, extraBindings map[string]charm.ExtraBinding) error {
	// If there is no extraBindings, we don't need to do anything.
	if len(extraBindings) == 0 {
		return nil
	}

	encodedBindings, err := encodeExtraBindings(id, extraBindings)
	if err != nil {
		return errors.Errorf("encoding charm extra bindings: %w", err)
	}
	query := `INSERT INTO charm_extra_binding (*) VALUES ($setCharmExtraBinding.*);`
	stmt, err := s.Prepare(query, setCharmExtraBinding{})
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedBindings).Run(); err != nil {
		return errors.Errorf("inserting charm extra bindings: %w", err)
	}

	return nil
}

func (s *State) setCharmStorage(ctx context.Context, tx *sqlair.TX, id corecharm.ID, storage map[string]charm.Storage) error {
	// If there is no storage, we don't need to do anything.
	if len(storage) == 0 {
		return nil
	}

	encodedStorage, encodedProperties, err := encodeStorage(id, storage)
	if err != nil {
		return errors.Errorf("encoding charm storage: %w", err)
	}

	storageQuery := `INSERT INTO charm_storage (*) VALUES ($setCharmStorage.*);`
	storageStmt, err := s.Prepare(storageQuery, setCharmStorage{})
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, storageStmt, encodedStorage).Run(); err != nil {
		return errors.Errorf("inserting charm storage: %w", err)
	}

	// Only insert properties if there are any.
	if len(encodedProperties) > 0 {
		propertiesQuery := `INSERT INTO charm_storage_property (*) VALUES ($setCharmStorageProperty.*);`
		propertiesStmt, err := s.Prepare(propertiesQuery, setCharmStorageProperty{})
		if err != nil {
			return errors.Errorf("preparing query: %w", err)
		}

		if err := tx.Query(ctx, propertiesStmt, encodedProperties).Run(); err != nil {
			return errors.Errorf("inserting charm storage properties: %w", err)
		}
	}
	return nil
}

func (s *State) setCharmDevices(ctx context.Context, tx *sqlair.TX, id corecharm.ID, devices map[string]charm.Device) error {
	// If there are no devices, we don't need to do anything.
	if len(devices) == 0 {
		return nil
	}

	query := `INSERT INTO charm_device (*) VALUES ($setCharmDevice.*);`
	stmt, err := s.Prepare(query, setCharmDevice{})
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodeDevices(id, devices)).Run(); err != nil {
		return errors.Errorf("inserting charm devices: %w", err)
	}

	return nil
}

func (s *State) setCharmResources(ctx context.Context, tx *sqlair.TX, id corecharm.ID, resources map[string]charm.Resource) error {
	// If there are no resources, we don't need to do anything.
	if len(resources) == 0 {
		return nil
	}

	encodedResources, err := encodeResources(id, resources)
	if err != nil {
		return errors.Errorf("encoding charm resources: %w", err)
	}

	query := `INSERT INTO charm_resource (*) VALUES ($setCharmResource.*);`
	stmt, err := s.Prepare(query, setCharmResource{})
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedResources).Run(); err != nil {
		return errors.Errorf("inserting charm resources: %w", err)
	}

	return nil
}

func (s *State) setCharmContainers(ctx context.Context, tx *sqlair.TX, id corecharm.ID, containers map[string]charm.Container) error {
	// If there are no containers, we don't need to do anything.
	if len(containers) == 0 {
		return nil
	}

	encodedContainers, encodedMounts, err := encodeContainers(id, containers)
	if err != nil {
		return errors.Errorf("encoding charm containers: %w", err)
	}

	containerQuery := `INSERT INTO charm_container (*) VALUES ($setCharmContainer.*);`
	containerStmt, err := s.Prepare(containerQuery, setCharmContainer{})
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, containerStmt, encodedContainers).Run(); err != nil {
		return errors.Errorf("inserting charm containers: %w", err)
	}

	// Only insert mounts if there are any.
	if len(encodedMounts) > 0 {
		mountQuery := `INSERT INTO charm_container_mount (*) VALUES ($setCharmMount.*);`
		mountStmt, err := s.Prepare(mountQuery, setCharmMount{})
		if err != nil {
			return errors.Errorf("preparing query: %w", err)
		}

		if err := tx.Query(ctx, mountStmt, encodedMounts).Run(); err != nil {
			return errors.Errorf("inserting charm container mounts: %w", err)
		}
	}

	return nil
}

func (s *State) setCharmActions(ctx context.Context, tx *sqlair.TX, id corecharm.ID, actions charm.Actions) error {
	// If there are no resources, we don't need to do anything.
	if len(actions.Actions) == 0 {
		return nil
	}

	query := `INSERT INTO charm_action (*) VALUES ($setCharmAction.*);`
	stmt, err := s.Prepare(query, setCharmAction{})
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodeActions(id, actions)).Run(); err != nil {
		return errors.Errorf("inserting charm actions: %w", err)
	}

	return nil
}

func (s *State) setCharmConfig(ctx context.Context, tx *sqlair.TX, id corecharm.ID, config charm.Config) error {
	// If there are no resources, we don't need to do anything.
	if len(config.Options) == 0 {
		return nil
	}

	encodedConfig, err := encodeConfig(id, config)
	if err != nil {
		return errors.Errorf("encoding charm config: %w", err)
	}

	query := `INSERT INTO charm_config (*) VALUES ($setCharmConfig.*);`
	stmt, err := s.Prepare(query, setCharmConfig{})
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedConfig).Run(); err != nil {
		return errors.Errorf("inserting charm config: %w", err)
	}

	return nil
}

func (s *State) setCharmManifest(ctx context.Context, tx *sqlair.TX, id corecharm.ID, manifest charm.Manifest) error {
	if len(manifest.Bases) == 0 {
		return applicationerrors.CharmManifestNotFound
	}

	encodedManifest, err := encodeManifest(id, manifest)
	if err != nil {
		return errors.Errorf("encoding charm manifest: %w", err)
	}

	query := `INSERT INTO charm_manifest_base (*) VALUES ($setCharmManifest.*);`
	stmt, err := s.Prepare(query, setCharmManifest{})
	if err != nil {
		return errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, encodedManifest).Run(); err != nil {
		return errors.Errorf("inserting charm manifest: %w", err)
	}

	return nil
}

// getCharm returns the charm for the given charm ID.
// This will delegate to the various get methods to get the charm metadata,
// config, manifest, actions and LXD profile.
func (s *State) getCharm(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Charm, *charm.DownloadInfo, error) {
	ch, err := s.getCharmState(ctx, tx, ident)
	if err != nil {
		return ch, nil, errors.Capture(err)
	}

	if ch.Metadata, err = s.getMetadata(ctx, tx, ident); err != nil {
		return ch, nil, errors.Capture(err)
	}

	if ch.Config, err = s.getCharmConfig(ctx, tx, ident); err != nil {
		return ch, nil, errors.Capture(err)
	}

	if ch.Manifest, err = s.getCharmManifest(ctx, tx, ident); err != nil {
		return ch, nil, errors.Capture(err)
	}

	if ch.Actions, err = s.getCharmActions(ctx, tx, ident); err != nil {
		return ch, nil, errors.Capture(err)
	}

	if ch.LXDProfile, _, err = s.getCharmLXDProfile(ctx, tx, ident); err != nil {
		return ch, nil, errors.Capture(err)
	}

	// Download information should only be recorded for charmhub charms.
	// If it's not present, ensure we report it as not found.
	var downloadInfo *charm.DownloadInfo
	if info, err := s.getCharmDownloadInfo(ctx, tx, ident); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ch, nil, errors.Capture(err)
	} else if err == nil {
		downloadInfo = &info
	} else if ch.Source == charm.CharmHubSource {
		return ch, nil, applicationerrors.CharmDownloadInfoNotFound
	}
	return ch, downloadInfo, nil
}

func (s *State) getCharmState(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Charm, error) {
	charmQuery := `
SELECT &charmState.*
FROM charm
WHERE uuid = $charmID.uuid;
`

	charmStmt, err := s.Prepare(charmQuery, charmState{}, ident)
	if err != nil {
		return charm.Charm{}, errors.Errorf("preparing query: %w", err)
	}

	hashQuery := `
SELECT &charmHash.*
FROM charm_hash
WHERE charm_uuid = $charmID.uuid;
`
	hashStmt, err := s.Prepare(hashQuery, charmHash{}, ident)
	if err != nil {
		return charm.Charm{}, errors.Errorf("preparing hash query: %w", err)
	}

	var charmState charmState
	if err := tx.Query(ctx, charmStmt, ident).Get(&charmState); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.Charm{}, applicationerrors.CharmNotFound
		}
		return charm.Charm{}, errors.Errorf("getting charm state: %w", err)
	}

	var hash charmHash
	if err := tx.Query(ctx, hashStmt, ident).Get(&hash); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.Charm{}, applicationerrors.CharmNotFound
		}
		return charm.Charm{}, errors.Errorf("getting charm hash: %w", err)
	}

	result, err := decodeCharmState(charmState)
	if err != nil {
		return charm.Charm{}, errors.Errorf("decoding charm state: %w", err)
	}

	result.Hash = hash.Hash

	return result, nil
}

func (s *State) getCharmDownloadInfo(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.DownloadInfo, error) {
	query := `
SELECT &charmDownloadInfo.*
FROM charm AS c
JOIN charm_download_info AS cdi ON c.uuid = cdi.charm_uuid
JOIN charm_provenance AS cp ON cp.id = cdi.provenance_id
WHERE c.uuid = $charmID.uuid
AND c.source_id = 1;
`

	stmt, err := s.Prepare(query, charmDownloadInfo{}, ident)
	if err != nil {
		return charm.DownloadInfo{}, errors.Errorf("preparing query: %w", err)
	}

	var downloadInfo charmDownloadInfo
	if err := tx.Query(ctx, stmt, ident).Get(&downloadInfo); err != nil {
		return charm.DownloadInfo{}, errors.Errorf("getting charm download info: %w", err)
	}

	provenance, err := decodeProvenance(downloadInfo.Provenance)
	if err != nil {
		return charm.DownloadInfo{}, errors.Errorf("decoding charm provenance: %w", err)
	}

	return charm.DownloadInfo{
		Provenance:         provenance,
		CharmhubIdentifier: downloadInfo.CharmhubIdentifier,
		DownloadURL:        downloadInfo.DownloadURL,
		DownloadSize:       downloadInfo.DownloadSize,
	}, nil
}

// getMetadata returns the metadata for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *State) getMetadata(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Metadata, error) {
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
		resources     []charmResource
		containers    []charmContainer
	)

	var err error
	if metadata, err = s.getCharmMetadata(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Capture(err)
	}

	if tags, err = s.getCharmTags(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Capture(err)
	}

	if categories, err = s.getCharmCategories(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Capture(err)
	}

	if terms, err = s.getCharmTerms(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Capture(err)
	}

	if relations, err = s.getCharmRelations(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Capture(err)
	}

	if extraBindings, err = s.getCharmExtraBindings(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Capture(err)
	}

	if storage, err = s.getCharmStorage(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Capture(err)
	}

	if devices, err = s.getCharmDevices(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Capture(err)
	}

	if resources, err = s.getCharmResources(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Capture(err)
	}

	if containers, err = s.getCharmContainers(ctx, tx, ident); err != nil {
		return charm.Metadata{}, errors.Capture(err)
	}

	return decodeMetadata(metadata, decodeMetadataArgs{
		tags:          tags,
		categories:    categories,
		terms:         terms,
		relations:     relations,
		extraBindings: extraBindings,
		storage:       storage,
		devices:       devices,
		resources:     resources,
		containers:    containers,
	})
}

// getCharmManifest returns the manifest for the charm, using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *State) getCharmManifest(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Manifest, error) {
	query := `
SELECT &charmManifest.*
FROM v_charm_manifest
WHERE charm_uuid = $charmID.uuid
ORDER BY array_index ASC, nested_array_index ASC;
`

	stmt, err := s.Prepare(query, charmManifest{}, ident)
	if err != nil {
		return charm.Manifest{}, errors.Errorf("preparing query: %w", err)
	}

	var manifests []charmManifest
	if err := tx.Query(ctx, stmt, ident).GetAll(&manifests); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.Manifest{}, applicationerrors.CharmNotFound
		}
		return charm.Manifest{}, errors.Errorf("getting charm manifest: %w", err)
	}

	return decodeManifest(manifests)
}

// getCharmLXDProfile returns the LXD profile for the charm using the
// charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *State) getCharmLXDProfile(ctx context.Context, tx *sqlair.TX, ident charmID) ([]byte, charm.Revision, error) {
	charmQuery := `
SELECT &charmID.*
FROM charm
WHERE uuid = $charmID.uuid;
	`

	lxdProfileQuery := `
SELECT &charmLXDProfile.*
FROM charm
JOIN charm_metadata AS cm ON charm.uuid = cm.charm_uuid
WHERE uuid = $charmID.uuid;
	`

	charmStmt, err := s.Prepare(charmQuery, ident)
	if err != nil {
		return nil, -1, errors.Errorf("preparing charm query: %w", err)
	}
	var profile charmLXDProfile
	lxdProfileStmt, err := s.Prepare(lxdProfileQuery, profile, ident)
	if err != nil {
		return nil, -1, errors.Errorf("preparing lxd profile query: %w", err)
	}

	if err := tx.Query(ctx, charmStmt, ident).Get(&ident); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, -1, applicationerrors.CharmNotFound
		}
		return nil, -1, errors.Errorf("getting charm: %w", err)
	}

	if err := tx.Query(ctx, lxdProfileStmt, ident).Get(&profile); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, -1, applicationerrors.LXDProfileNotFound
		}
		return nil, -1, errors.Errorf("getting charm lxd profile: %w", err)
	}

	// TODO - figure out why this is happening
	// Sometimes we get an empty slice, sometimes a nil slice.
	// Cater for both cases.
	if len(profile.LXDProfile) == 0 {
		profile.LXDProfile = nil
	}
	return profile.LXDProfile, profile.Revision, nil
}

// getCharmConfig returns the config for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *State) getCharmConfig(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Config, error) {
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
		return charm.Config{}, errors.Errorf("preparing charm query: %w", err)
	}
	configStmt, err := s.Prepare(configQuery, charmConfig{}, ident)
	if err != nil {
		return charm.Config{}, errors.Errorf("preparing config query: %w", err)
	}

	if err := tx.Query(ctx, charmStmt, ident).Get(&ident); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.Config{}, applicationerrors.CharmNotFound
		}
		return charm.Config{}, errors.Errorf("getting charm: %w", err)
	}

	var configs []charmConfig
	if err := tx.Query(ctx, configStmt, ident).GetAll(&configs); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.Config{}, nil
		}
		return charm.Config{}, errors.Errorf("getting charm config: %w", err)
	}

	return decodeConfig(configs)
}

// getCharmActions returns the actions for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *State) getCharmActions(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Actions, error) {
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
		return charm.Actions{}, errors.Errorf("preparing charm query: %w", err)
	}
	actionsStmt, err := s.Prepare(actionQuery, charmAction{}, ident)
	if err != nil {
		return charm.Actions{}, errors.Errorf("preparing action query: %w", err)
	}

	if err := tx.Query(ctx, charmStmt, ident).Get(&ident); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.Actions{}, applicationerrors.CharmNotFound
		}
		return charm.Actions{}, errors.Errorf("getting charm: %w", err)
	}

	var actions []charmAction
	if err := tx.Query(ctx, actionsStmt, ident).GetAll(&actions); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charm.Actions{}, nil
		}
		return charm.Actions{}, errors.Errorf("getting charm actions: %w", err)
	}

	return decodeActions(actions), nil
}

// getCharmMetadata returns the metadata for the charm using the charm ID.
// This is the core metadata for the charm.
func (s *State) getCharmMetadata(ctx context.Context, tx *sqlair.TX, ident charmID) (charmMetadata, error) {
	query := `
SELECT &charmMetadata.*
FROM v_charm_metadata
WHERE uuid = $charmID.uuid;
`
	var metadata charmMetadata
	stmt, err := s.Prepare(query, metadata, ident)
	if err != nil {
		return metadata, errors.Errorf("preparing query: %w", err)
	}

	if err := tx.Query(ctx, stmt, ident).Get(&metadata); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return metadata, applicationerrors.CharmNotFound
		}
		return metadata, errors.Errorf("failed to select charm metadata: %w", err)
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
func (s *State) getCharmTags(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmTag, error) {
	query := `
SELECT &charmTag.*
FROM charm_tag
WHERE charm_uuid = $charmID.uuid
ORDER BY array_index ASC;
`
	stmt, err := s.Prepare(query, charmTag{}, ident)
	if err != nil {
		return nil, errors.Errorf("preparing query: %w", err)
	}

	var result []charmTag
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, errors.Errorf("failed to select charm tags: %w", err)
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
func (s *State) getCharmCategories(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmCategory, error) {
	query := `
SELECT &charmCategory.*
FROM charm_category
WHERE charm_uuid = $charmID.uuid
ORDER BY array_index ASC;
`
	stmt, err := s.Prepare(query, charmCategory{}, ident)
	if err != nil {
		return nil, errors.Errorf("preparing query: %w", err)
	}

	var result []charmCategory
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, errors.Errorf("failed to select charm categories: %w", err)
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
func (s *State) getCharmTerms(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmTerm, error) {
	query := `
SELECT &charmTerm.*
FROM charm_term
WHERE charm_uuid = $charmID.uuid
ORDER BY array_index ASC;
`
	stmt, err := s.Prepare(query, charmTerm{}, ident)
	if err != nil {
		return nil, errors.Errorf("preparing query: %w", err)
	}

	var result []charmTerm
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, errors.Errorf("failed to select charm terms: %w", err)
	}

	return result, nil
}

// getCharmRelations returns the relations for the charm using the charm ID.
// This is a slice of all the relations for the charm. Additional processing
// is required to separate the relations into provides, requires and peers.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
func (s *State) getCharmRelations(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmRelation, error) {
	query := `
SELECT &charmRelation.*
FROM v_charm_relation
WHERE charm_uuid = $charmID.uuid;
	`
	stmt, err := s.Prepare(query, charmRelation{}, ident)
	if err != nil {
		return nil, errors.Errorf("preparing query: %w", err)
	}

	var result []charmRelation
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, errors.Errorf("failed to select charm relations: %w", err)
	}

	return result, nil
}

// getCharmExtraBindings returns the extra bindings for the charm using the
// charm ID. This is a slice of charmExtraBinding and not a map of string to
// string, because sqlair doesn't work on scalar types. If the sqlair library
// gains support for scalar types, this can be changed.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
func (s *State) getCharmExtraBindings(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmExtraBinding, error) {
	query := `
SELECT &charmExtraBinding.*
FROM charm_extra_binding
WHERE charm_uuid = $charmID.uuid;
`

	stmt, err := s.Prepare(query, charmExtraBinding{}, ident)
	if err != nil {
		return nil, errors.Errorf("preparing query: %w", err)
	}

	var result []charmExtraBinding
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, errors.Errorf("failed to select charm extra bindings: %w", err)
	}

	return result, nil
}

// getCharmStorage returns the storage for the charm using the charm ID.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
// Charm properties are expected to be unique, no duplicates are expected.
func (s *State) getCharmStorage(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmStorage, error) {
	query := `
SELECT &charmStorage.*
FROM v_charm_storage
WHERE charm_uuid = $charmID.uuid
ORDER BY property_index ASC;
`

	stmt, err := s.Prepare(query, charmStorage{}, ident)
	if err != nil {
		return nil, errors.Errorf("preparing query: %w", err)
	}

	var result []charmStorage
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, errors.Errorf("failed to select charm storage: %w", err)
	}

	return result, nil
}

// getCharmDevices returns the devices for the charm using the charm ID.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
func (s *State) getCharmDevices(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmDevice, error) {
	query := `
SELECT &charmDevice.*
FROM charm_device
WHERE charm_uuid = $charmID.uuid;
`

	stmt, err := s.Prepare(query, charmDevice{}, ident)
	if err != nil {
		return nil, errors.Errorf("preparing query: %w", err)
	}

	var result []charmDevice
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, errors.Errorf("failed to select charm device: %w", err)
	}

	return result, nil
}

// getCharmResources returns the resources for the charm using the charm ID.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
func (s *State) getCharmResources(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmResource, error) {
	query := `
SELECT &charmResource.*
FROM v_charm_resource
WHERE charm_uuid = $charmID.uuid;
`

	stmt, err := s.Prepare(query, charmResource{}, ident)
	if err != nil {
		return nil, errors.Errorf("preparing query: %w", err)
	}

	var result []charmResource
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, errors.Errorf("failed to select charm resource: %w", err)
	}

	return result, nil
}

// getCharmContainers returns the containers for the charm using the charm ID.
// If the charm does not exist, no error is returned. It is expected that
// the caller will handle this case.
func (s *State) getCharmContainers(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmContainer, error) {
	query := `
SELECT &charmContainer.*
FROM v_charm_container
WHERE charm_uuid = $charmID.uuid
ORDER BY array_index ASC;
`

	stmt, err := s.Prepare(query, charmContainer{}, ident)
	if err != nil {
		return nil, errors.Errorf("preparing query: %w", err)
	}

	var result []charmContainer
	if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return result, nil
		}
		return nil, errors.Errorf("failed to select charm container: %w", err)
	}

	return result, nil
}

// checkUnitNotDead checks if the unit exists and is not dead. It's possible to
// access alive and dying units, but not dead ones:
// - If the unit is not found, [applicationerrors.UnitNotFound] is returned.
// - If the unit is dead, [applicationerrors.UnitIsDead] is returned.
func (st *State) checkUnitNotDead(ctx context.Context, tx *sqlair.TX, ident unitUUID) error {
	type life struct {
		LifeID domainlife.Life `db:"life_id"`
	}

	query := `
SELECT &life.*
FROM unit
WHERE uuid = $unitUUID.uuid;
`
	stmt, err := st.Prepare(query, ident, life{})
	if err != nil {
		return errors.Errorf("preparing query for unit %q: %w", ident.UnitUUID, err)
	}

	var result life
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return applicationerrors.UnitNotFound
	} else if err != nil {
		return errors.Errorf("checking unit %q exists: %w", ident.UnitUUID, err)
	}

	switch result.LifeID {
	case domainlife.Dead:
		return applicationerrors.UnitIsDead
	default:
		return nil
	}
}

// checkApplicationNameAvailable checks if the application name is available.
// If the application name is available, nil is returned. If the application
// name is not available, [applicationerrors.ApplicationAlreadyExists] is
// returned.
func (st *State) checkApplicationNameAvailable(ctx context.Context, tx *sqlair.TX, name string) error {
	app := applicationDetails{Name: name}

	var result countResult
	existsQueryStmt, err := st.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM application
WHERE name = $applicationDetails.name
`, app, result)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, existsQueryStmt, app).Get(&result); errors.Is(err, sqlair.ErrNoRows) {
		return nil
	} else if err != nil {
		return errors.Errorf("checking if application %q exists: %w", name, err)
	}
	if result.Count > 0 {
		return applicationerrors.ApplicationAlreadyExists
	}
	return nil
}

// checkApplicationAlive checks if the application exists and is alive.
//   - If the application is not alive, [applicationerrors.ApplicationNotAlive] is returned.
//   - If the application is not found, [applicationerrors.ApplicationNotFound]
//     is returned.
func (st *State) checkApplicationAlive(ctx context.Context, tx *sqlair.TX, appUUID coreapplication.ID) error {
	return st.checkApplicationLife(ctx, tx, appUUID, domainlife.Alive)
}

// checkApplicationNotDead checks if the application exists and is not dead. It's
// possible to access alive and dying applications, but not dead ones.
//   - If the application is dead, [applicationerrors.ApplicationIsDead] is returned.
//   - If the application is not found, [applicationerrors.ApplicationNotFound]
//     is returned.
func (st *State) checkApplicationNotDead(ctx context.Context, tx *sqlair.TX, appUUID coreapplication.ID) error {
	return st.checkApplicationLife(ctx, tx, appUUID, domainlife.Dying)
}

// checkApplicationLife checks if the application exists and its life has not
// advanced beyond the specified allowed life.
// Note: this is a helper method and should be called directly.
// Instead use one of:
//   - checkApplicationAlive
//   - checkApplicationNotDead
func (st *State) checkApplicationLife(ctx context.Context, tx *sqlair.TX, appUUID coreapplication.ID, allowed domainlife.Life) error {
	type life struct {
		LifeID domainlife.Life `db:"life_id"`
	}

	ident := applicationID{ID: appUUID}
	query := `
SELECT &life.*
FROM application
WHERE uuid = $applicationID.uuid;
`
	stmt, err := st.Prepare(query, ident, life{})
	if err != nil {
		return errors.Errorf("preparing query for application %q: %w", ident.ID, err)
	}

	var result life
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return applicationerrors.ApplicationNotFound
	} else if err != nil {
		return errors.Errorf("checking application %q exists: %w", ident.ID, err)
	}

	switch result.LifeID {
	case domainlife.Dead:
		if allowed < result.LifeID {
			return applicationerrors.ApplicationIsDead
		}
	case domainlife.Dying:
		if allowed < result.LifeID {
			return applicationerrors.ApplicationNotAlive
		}
	default:
		return nil
	}
	return nil
}

func decodeCharmState(state charmState) (charm.Charm, error) {
	arch, err := decodeArchitecture(state.ArchitectureID)
	if err != nil {
		return charm.Charm{}, err
	}

	source, err := decodeCharmSource(state.SourceID)
	if err != nil {
		return charm.Charm{}, err
	}

	var objectStoreUUID objectstore.UUID
	if state.ObjectStoreUUID.Valid {
		objectStoreUUID, err = objectstore.ParseUUID(state.ObjectStoreUUID.String)
		if err != nil {
			return charm.Charm{}, err
		}
	}

	return charm.Charm{
		ReferenceName:   state.ReferenceName,
		Revision:        state.Revision,
		ArchivePath:     state.ArchivePath,
		ObjectStoreUUID: objectStoreUUID,
		Available:       state.Available,
		Version:         state.Version,
		Architecture:    arch,
		Source:          source,
	}, nil

}

func decodeArchitecture(arch sql.NullInt64) (application.Architecture, error) {
	if !arch.Valid {
		return architecture.Unknown, nil
	}

	switch arch.Int64 {
	case 0:
		return architecture.AMD64, nil
	case 1:
		return architecture.ARM64, nil
	case 2:
		return architecture.PPC64EL, nil
	case 3:
		return architecture.S390X, nil
	case 4:
		return architecture.RISCV64, nil
	default:
		return -1, errors.Errorf("unsupported architecture: %d", arch.Int64)
	}
}

func decodeCharmSource(source int) (charm.CharmSource, error) {
	switch source {
	case 1:
		return charm.CharmHubSource, nil
	case 0:
		return charm.LocalSource, nil
	default:
		return "", errors.Errorf("unsupported charm source: %d", source)
	}
}

func encodeArchitecture(a architecture.Architecture) (int, error) {
	switch a {
	// This is a valid case if we're uploading charms and the value isn't
	// supplied.
	case architecture.Unknown:
		return -1, nil
	case architecture.AMD64:
		return 0, nil
	case architecture.ARM64:
		return 1, nil
	case architecture.PPC64EL:
		return 2, nil
	case architecture.S390X:
		return 3, nil
	case architecture.RISCV64:
		return 4, nil
	default:
		return 0, errors.Errorf("unsupported architecture: %d", a)
	}
}

func encodeCharmSource(source charm.CharmSource) (int, error) {
	switch source {
	case charm.LocalSource:
		return 0, nil
	case charm.CharmHubSource:
		return 1, nil
	default:
		return 0, errors.Errorf("unsupported source type: %s", source)
	}
}

func encodeProvenance(provenance charm.Provenance) (int, error) {
	switch provenance {
	case charm.ProvenanceDownload:
		return 0, nil
	case charm.ProvenanceMigration:
		return 1, nil
	case charm.ProvenanceUpload:
		return 2, nil
	case charm.ProvenanceBootstrap:
		return 3, nil
	default:
		return 0, errors.Errorf("unsupported provenance type: %s", provenance)
	}
}

func decodeProvenance(provenance string) (charm.Provenance, error) {
	switch provenance {
	case "download":
		return charm.ProvenanceDownload, nil
	case "migration":
		return charm.ProvenanceMigration, nil
	case "upload":
		return charm.ProvenanceUpload, nil
	case "bootstrap":
		return charm.ProvenanceBootstrap, nil
	default:
		return "", errors.Errorf("unknown provenance: %s", provenance)
	}
}
