// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	corecharm "github.com/juju/juju/core/charm"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/charm"
	charmerrors "github.com/juju/juju/domain/charm/errors"
)

// State is used to access the database.
type State struct {
	*domain.StateBase
}

// NewState creates a state to access the database.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetCharmIDByRevision returns the charm ID by the natural key, for a
// specific revision.
// If the charm does not exist, a NotFound error is returned.
func (s *State) GetCharmIDByRevision(ctx context.Context, name string, revision int) (corecharm.ID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	query := `
SELECT charm.uuid AS &charmID.*
FROM charm
INNER JOIN charm_origin
ON charm.uuid = charm_origin.charm_uuid
WHERE charm.name = $charmNameRevision.name
AND charm_origin.revision = $charmNameRevision.revision;
`
	stmt, err := s.Prepare(query, charmID{}, charmNameRevision{})
	if err != nil {
		return "", fmt.Errorf("failed to prepare query: %w", err)
	}

	var id corecharm.ID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result charmID
		if err := tx.Query(ctx, stmt, charmNameRevision{
			Name:     name,
			Revision: revision,
		}).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return charmerrors.NotFound
			}
			return fmt.Errorf("failed to get charm ID: %w", err)
		}
		id = corecharm.ID(result.UUID)
		return nil
	}); err != nil {
		return "", fmt.Errorf("failed to run transaction: %w", err)
	}
	return id, nil
}

// IsControllerCharm returns whether the charm is a controller charm.
// If the charm does not exist, a NotFound error is returned.
func (s *State) IsControllerCharm(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	query := `
SELECT name AS &charmName.name
FROM charm
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, ident, charmName{})
	if err != nil {
		return false, fmt.Errorf("failed to prepare query: %w", err)
	}

	var isController bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result charmName
		if err := tx.Query(ctx, stmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return charmerrors.NotFound
			}
			return fmt.Errorf("failed to get charm ID: %w", err)
		}
		isController = result.Name == "juju-controller"
		return nil
	}); err != nil {
		return false, fmt.Errorf("failed to run transaction: %w", err)
	}
	return isController, nil
}

// IsSubordinateCharm returns whether the charm is a subordinate charm.
// If the charm does not exist, a NotFound error is returned.
func (s *State) IsSubordinateCharm(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	query := `
SELECT subordinate AS &charmSubordinate.subordinate
FROM charm
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, ident, charmSubordinate{})
	if err != nil {
		return false, fmt.Errorf("failed to prepare query: %w", err)
	}

	var isSubordinate bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result charmSubordinate
		if err := tx.Query(ctx, stmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return charmerrors.NotFound
			}
			return fmt.Errorf("failed to get charm ID: %w", err)
		}
		isSubordinate = result.Subordinate
		return nil
	}); err != nil {
		return false, fmt.Errorf("failed to run transaction: %w", err)
	}
	return isSubordinate, nil
}

// SupportsContainers returns whether the charm supports containers.
// If the charm does not exist, a NotFound error is returned.
func (s *State) SupportsContainers(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	query := `
SELECT charm_container.charm_uuid AS &charmID.uuid
FROM charm
LEFT JOIN charm_container
ON charm.uuid = charm_container.charm_uuid
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, ident)
	if err != nil {
		return false, fmt.Errorf("failed to prepare query: %w", err)
	}

	var supportsContainers bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result []charmID
		if err := tx.Query(ctx, stmt, ident).GetAll(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return charmerrors.NotFound
			}
			return fmt.Errorf("failed to get charm ID: %w", err)
		}
		var num int
		for _, r := range result {
			if r.UUID == id.String() {
				num++
			}
		}
		supportsContainers = num > 0
		return nil
	}); err != nil {
		return false, fmt.Errorf("failed to run transaction: %w", err)
	}
	return supportsContainers, nil
}

// IsCharmAvailable returns whether the charm is available for use.
// If the charm does not exist, a NotFound error is returned.
func (s *State) IsCharmAvailable(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	query := `
SELECT charm_state.available AS &charmAvailable.available
FROM charm
INNER JOIN charm_state
ON charm.uuid = charm_state.charm_uuid
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, ident, charmAvailable{})
	if err != nil {
		return false, fmt.Errorf("failed to prepare query: %w", err)
	}

	var isAvailable bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result charmAvailable
		if err := tx.Query(ctx, stmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return charmerrors.NotFound
			}
			return fmt.Errorf("failed to get charm ID: %w", err)
		}
		isAvailable = result.Available
		return nil
	}); err != nil {
		return false, fmt.Errorf("failed to run transaction: %w", err)
	}
	return isAvailable, nil
}

// SetCharmAvailable sets the charm as available for use.
// If the charm does not exist, a NotFound error is returned.
func (s *State) SetCharmAvailable(ctx context.Context, id corecharm.ID) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	selectQuery := `
SELECT charm.uuid AS &charmID.*
FROM charm
WHERE uuid = $charmID.uuid;
	`

	selectStmt, err := s.Prepare(selectQuery, ident)
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	updateQuery := `
UPDATE charm_state
SET available = true
WHERE charm_uuid = $charmID.uuid;
`

	updateStmt, err := s.Prepare(updateQuery, ident)
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result charmID
		if err := tx.Query(ctx, selectStmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return charmerrors.NotFound
			}
			return fmt.Errorf("failed to set charm available: %w", err)
		}

		if err := tx.Query(ctx, updateStmt, ident).Run(); err != nil {
			return fmt.Errorf("failed to set charm available: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to run transaction: %w", err)
	}

	return nil
}

// ReserveCharmRevision defines a placeholder for a new charm revision.
// The original charm will need to exist, the returning charm ID will be
// the new charm ID for the revision.
func (s *State) ReserveCharmRevision(ctx context.Context, id corecharm.ID, revision int) (corecharm.ID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	selectQuery := `
SELECT charm.* AS &charmIDName.*
FROM charm 
LEFT JOIN charm_state
ON charm.uuid = charm_state.charm_uuid
WHERE uuid = $charmID.uuid;
`
	selectStmt, err := s.Prepare(selectQuery, charmIDName{}, ident)
	if err != nil {
		return "", fmt.Errorf("failed to prepare query: %w", err)
	}

	insertCharmQuery := `INSERT INTO charm (*) VALUES ($charmIDName.*);`
	insertCharmStmt, err := s.Prepare(insertCharmQuery, charmIDName{})
	if err != nil {
		return "", fmt.Errorf("failed to prepare query: %w", err)
	}

	insertCharmStateQuery := `INSERT INTO charm_state ("charm_uuid", "available") VALUES ($charmID.uuid, false);`
	insertCharmStateStmt, err := s.Prepare(insertCharmStateQuery, ident)
	if err != nil {
		return "", fmt.Errorf("failed to prepare query: %w", err)
	}

	newID, err := corecharm.NewID()
	if err != nil {
		return "", fmt.Errorf("failed to reserve charm revision: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var charmResult charmIDName
		if err := tx.Query(ctx, selectStmt, ident).Get(&charmResult); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return charmerrors.NotFound
			}
			return fmt.Errorf("failed to reserve charm revision: %w", err)
		}

		newCharm := charmResult
		newCharm.UUID = newID.String()
		if err := tx.Query(ctx, insertCharmStmt, newCharm).Run(); err != nil {
			return fmt.Errorf("failed to reserve charm revision: inserting charm: %w", err)
		}

		if err := tx.Query(ctx, insertCharmStateStmt, charmID{
			UUID: newID.String(),
		}).Run(); err != nil {
			return fmt.Errorf("failed to reserve charm revision: inserting charm state: %w", err)
		}

		return nil
	}); err != nil {
		return "", fmt.Errorf("failed to run transaction: %w", err)
	}

	return newID, nil
}

// GetCharmMetadata returns the metadata for the charm using the charm ID.
// If the charm does not exist, a NotFound error is returned.
func (s *State) GetCharmMetadata(ctx context.Context, id corecharm.ID) (charm.Metadata, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

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
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		if metadata, err = getCharmMetadata(ctx, tx, s, ident); err != nil {
			return errors.Trace(err)
		}

		if tags, err = getCharmTags(ctx, tx, s, ident); err != nil {
			return errors.Trace(err)
		}

		if categories, err = getCharmCategories(ctx, tx, s, ident); err != nil {
			return errors.Trace(err)
		}

		if terms, err = getCharmTerms(ctx, tx, s, ident); err != nil {
			return errors.Trace(err)
		}

		if relations, err = getCharmRelations(ctx, tx, s, ident); err != nil {
			return errors.Trace(err)
		}

		if extraBindings, err = getCharmExtraBindings(ctx, tx, s, ident); err != nil {
			return errors.Trace(err)
		}

		if storage, err = getCharmStorage(ctx, tx, s, ident); err != nil {
			return errors.Trace(err)
		}

		if devices, err = getCharmDevices(ctx, tx, s, ident); err != nil {
			return errors.Trace(err)
		}

		if payloads, err = getCharmPayloads(ctx, tx, s, ident); err != nil {
			return errors.Trace(err)
		}

		if resources, err = getCharmResources(ctx, tx, s, ident); err != nil {
			return errors.Trace(err)
		}

		if containers, err = getCharmContainers(ctx, tx, s, ident); err != nil {
			return errors.Trace(err)
		}

		return nil
	}); err != nil {
		return charm.Metadata{}, fmt.Errorf("failed to run transaction: %w", err)
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

// getCharmMetadata returns the metadata for the charm using the charm ID.
// This is the core metadata for the charm.
func getCharmMetadata(ctx context.Context, tx *sqlair.TX, p domain.Preparer, ident charmID) (charmMetadata, error) {
	query := `
SELECT v_charm.* AS &charmMetadata.*
FROM v_charm
WHERE uuid = $charmID.uuid;
`
	stmt, err := p.Prepare(query, charmMetadata{}, ident)
	if err != nil {
		return charmMetadata{}, fmt.Errorf("failed to prepare query: %w", err)
	}

	var metadata charmMetadata
	if err := tx.Query(ctx, stmt, ident).Get(&metadata); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return charmMetadata{}, charmerrors.NotFound
		}
		return charmMetadata{}, fmt.Errorf("failed to select charm metadata: %w", err)
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
func getCharmTags(ctx context.Context, tx *sqlair.TX, p domain.Preparer, ident charmID) ([]charmTag, error) {
	query := `
SELECT charm_tag.* AS &charmTag.*
FROM charm_tag
WHERE charm_uuid = $charmID.uuid
ORDER BY "index" ASC;
`
	stmt, err := p.Prepare(query, charmTag{}, ident)
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
func getCharmCategories(ctx context.Context, tx *sqlair.TX, p domain.Preparer, ident charmID) ([]charmCategory, error) {
	query := `
SELECT charm_category.* AS &charmCategory.*
FROM charm_category
WHERE charm_uuid = $charmID.uuid
ORDER BY "index" ASC;
`
	stmt, err := p.Prepare(query, charmCategory{}, ident)
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
func getCharmTerms(ctx context.Context, tx *sqlair.TX, p domain.Preparer, ident charmID) ([]charmTerm, error) {
	query := `
SELECT charm_term.* AS &charmTerm.*
FROM charm_term
WHERE charm_uuid = $charmID.uuid
ORDER BY "index" ASC;
`
	stmt, err := p.Prepare(query, charmTerm{}, ident)
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
func getCharmRelations(ctx context.Context, tx *sqlair.TX, p domain.Preparer, ident charmID) ([]charmRelation, error) {
	query := `
SELECT v_charm_relation.* AS &charmRelation.*
FROM v_charm_relation
WHERE charm_uuid = $charmID.uuid;
	`
	stmt, err := p.Prepare(query, charmRelation{}, ident)
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
func getCharmExtraBindings(ctx context.Context, tx *sqlair.TX, p domain.Preparer, ident charmID) ([]charmExtraBinding, error) {
	query := `
SELECT charm_extra_binding.* AS &charmExtraBinding.*
FROM charm_extra_binding
WHERE charm_uuid = $charmID.uuid;
`

	stmt, err := p.Prepare(query, charmExtraBinding{}, ident)
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
func getCharmStorage(ctx context.Context, tx *sqlair.TX, p domain.Preparer, ident charmID) ([]charmStorage, error) {
	query := `
SELECT v_charm_storage.* AS &charmStorage.*
FROM v_charm_storage
WHERE charm_uuid = $charmID.uuid
ORDER BY property_index ASC;
`

	stmt, err := p.Prepare(query, charmStorage{}, ident)
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
func getCharmDevices(ctx context.Context, tx *sqlair.TX, p domain.Preparer, ident charmID) ([]charmDevice, error) {
	query := `
SELECT charm_device.* AS &charmDevice.*
FROM charm_device
WHERE charm_uuid = $charmID.uuid;
`

	stmt, err := p.Prepare(query, charmDevice{}, ident)
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
func getCharmPayloads(ctx context.Context, tx *sqlair.TX, p domain.Preparer, ident charmID) ([]charmPayload, error) {
	query := `
SELECT charm_payload.* AS &charmPayload.*
FROM charm_payload
WHERE charm_uuid = $charmID.uuid;
`

	stmt, err := p.Prepare(query, charmPayload{}, ident)
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
func getCharmResources(ctx context.Context, tx *sqlair.TX, p domain.Preparer, ident charmID) ([]charmResource, error) {
	query := `
SELECT v_charm_resource.* AS &charmResource.*
FROM v_charm_resource
WHERE charm_uuid = $charmID.uuid;
`

	stmt, err := p.Prepare(query, charmResource{}, ident)
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
func getCharmContainers(ctx context.Context, tx *sqlair.TX, p domain.Preparer, ident charmID) ([]charmContainer, error) {
	query := `
SELECT v_charm_container.* AS &charmContainer.*
FROM v_charm_container
WHERE charm_uuid = $charmID.uuid
ORDER BY "index" ASC;
`

	stmt, err := p.Prepare(query, charmContainer{}, ident)
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
