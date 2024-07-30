// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
)

// State represents a type for interacting with the underlying state.
// Composes both application and charm state, so we can interact with both
// from the single state, whilst also keeping the concerns separate.
type State struct {
	*domain.StateBase
	*ApplicationState
	*CharmState
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory, logger logger.Logger) *State {
	base := &commonStateBase{
		StateBase: domain.NewStateBase(factory),
	}
	return &State{
		ApplicationState: NewApplicationState(base, logger),
		CharmState:       NewCharmState(base),
	}
}

type commonStateBase struct {
	*domain.StateBase
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
