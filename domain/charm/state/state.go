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

// hashKind is the type of hash to store.
type hashKind = int

const (
	// sha256HashKind is the ID for the SHA256 hash kind.
	sha256HashKind hashKind = 0
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
		if metadata, err = s.getCharmMetadata(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if tags, err = s.getCharmTags(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if categories, err = s.getCharmCategories(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if terms, err = s.getCharmTerms(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if relations, err = s.getCharmRelations(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if extraBindings, err = s.getCharmExtraBindings(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if storage, err = s.getCharmStorage(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if devices, err = s.getCharmDevices(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if payloads, err = s.getCharmPayloads(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if resources, err = s.getCharmResources(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if containers, err = s.getCharmContainers(ctx, tx, ident); err != nil {
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

// GetCharmManifest returns the manifest for the charm using the charm ID.
// If the charm does not exist, a NotFound error is returned.
func (s *State) GetCharmManifest(ctx context.Context, id corecharm.ID) (charm.Manifest, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Manifest{}, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	query := `
SELECT &charmManifest.*
FROM v_charm_manifest
WHERE charm_uuid = $charmID.uuid
ORDER BY array_index ASC;
`

	stmt, err := s.Prepare(query, charmManifest{}, ident)
	if err != nil {
		return charm.Manifest{}, fmt.Errorf("failed to prepare query: %w", err)
	}

	var manifests []charmManifest
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).GetAll(&manifests); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return charmerrors.NotFound
			}
			return fmt.Errorf("failed to get charm manifest: %w", err)
		}
		return nil
	}); err != nil {
		return charm.Manifest{}, fmt.Errorf("failed to run transaction: %w", err)
	}

	return decodeManifest(manifests)
}

// GetCharmLXDProfile returns the LXD profile for the charm using the
// charm ID.
// If the charm does not exist, a NotFound error is returned.
func (s *State) GetCharmLXDProfile(ctx context.Context, id corecharm.ID) ([]byte, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

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

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&profile); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return charmerrors.NotFound
			}
			return fmt.Errorf("failed to get charm lxd profile: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to run transaction: %w", err)
	}

	return profile.LXDProfile, nil
}

// GetCharmConfig returns the config for the charm using the charm ID.
// If the charm does not exist, a NotFound error is returned.
func (s *State) GetCharmConfig(ctx context.Context, id corecharm.ID) (charm.Config, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Config{}, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

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
		return charm.Config{}, fmt.Errorf("failed to prepare query: %w", err)
	}
	configStmt, err := s.Prepare(configQuery, charmConfig{}, ident)
	if err != nil {
		return charm.Config{}, fmt.Errorf("failed to prepare query: %w", err)
	}

	var configs []charmConfig
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, charmStmt, ident).Get(&ident); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return charmerrors.NotFound
			}
		}

		if err := tx.Query(ctx, configStmt, ident).GetAll(&configs); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("failed to get charm config: %w", err)
		}
		return nil
	}); err != nil {
		return charm.Config{}, fmt.Errorf("failed to run transaction: %w", err)
	}

	return decodeConfig(configs)
}

// GetCharmActions returns the actions for the charm using the charm ID.
// If the charm does not exist, a NotFound error is returned.
func (s *State) GetCharmActions(ctx context.Context, id corecharm.ID) (charm.Actions, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Actions{}, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

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
		return charm.Actions{}, fmt.Errorf("failed to prepare query: %w", err)
	}
	actionsStmt, err := s.Prepare(actionQuery, charmAction{}, ident)
	if err != nil {
		return charm.Actions{}, fmt.Errorf("failed to prepare query: %w", err)
	}

	var actions []charmAction
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, charmStmt, ident).Get(&ident); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return charmerrors.NotFound
			}
		}

		if err := tx.Query(ctx, actionsStmt, ident).GetAll(&actions); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("failed to get charm actions: %w", err)
		}
		return nil
	}); err != nil {
		return charm.Actions{}, fmt.Errorf("failed to run transaction: %w", err)
	}

	return decodeActions(actions), nil
}

// SetCharm persists the charm metadata, actions, config and manifest to
// state.
func (s *State) SetCharm(ctx context.Context, charm charm.Charm, charmArgs charm.SetStateArgs) (corecharm.ID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	id, err := corecharm.NewID()
	if err != nil {
		return "", fmt.Errorf("failed to set charm: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check the charm doesn't already exist, if it does, return an already
		// exists error. Also doing this early, prevents the moving straight
		// to a write transaction.
		if err := s.checkSetCharmExists(ctx, tx, id, charm.Metadata.Name, charmArgs.Revision); err != nil {
			return errors.Trace(err)
		}

		if err := s.setCharmMetadata(ctx, tx, id, charm.Metadata, charm.LXDProfile, charmArgs.ArchivePath); err != nil {
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

		if err := s.setCharmHash(ctx, tx, id, charmArgs.Hash); err != nil {
			return errors.Trace(err)
		}

		if err := s.setCharmInitialOrigin(ctx, tx, id, charmArgs.Source, charmArgs.Revision, charmArgs.Version); err != nil {
			return errors.Trace(err)
		}

		return nil
	}); err != nil {
		return "", fmt.Errorf("failed to run transaction: %w", err)
	}

	return id, nil
}

var tablesToDeleteFrom = []string{
	"charm_action",
	"charm_category",
	"charm_channel",
	"charm_config",
	"charm_container_mount",
	"charm_container",
	"charm_device",
	"charm_extra_binding",
	"charm_hash",
	"charm_manifest_base",
	"charm_origin",
	"charm_payload",
	"charm_platform",
	"charm_relation",
	"charm_resource",
	"charm_state",
	"charm_storage_property",
	"charm_storage",
	"charm_tag",
	"charm_term",
}

type deleteStatement struct {
	stmt      *sqlair.Statement
	tableName string
}

// DeleteCharm removes the charm from the state.
// If the charm does not exist, a NotFound error is returned.
func (s *State) DeleteCharm(ctx context.Context, id corecharm.ID) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	selectQuery, err := s.Prepare(`SELECT charm.uuid AS &charmID.* FROM charm WHERE uuid = $charmID.uuid;`, charmID{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	deleteQuery, err := s.Prepare(`DELETE FROM charm WHERE uuid = $charmID.uuid;`, charmID{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	// Prepare the delete statements for each table.
	stmts := make([]deleteStatement, len(tablesToDeleteFrom))
	for i, table := range tablesToDeleteFrom {
		query := fmt.Sprintf("DELETE FROM %s WHERE charm_uuid = $charmUUID.charm_uuid;", table)

		stmt, err := s.Prepare(query, charmUUID{})
		if err != nil {
			return fmt.Errorf("failed to prepare query: %w", err)
		}

		stmts[i] = deleteStatement{
			stmt:      stmt,
			tableName: table,
		}
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, selectQuery, charmID{UUID: id.String()}).Get(&charmID{}); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return charmerrors.NotFound
			}
		}

		// Delete the foreign key references first.
		for _, stmt := range stmts {
			if err := tx.Query(ctx, stmt.stmt, charmUUID{UUID: id.String()}).Run(); err != nil {
				return fmt.Errorf("failed to delete related data for %q: %w", stmt.tableName, err)
			}
		}

		// Then delete the charm itself.
		if err := tx.Query(ctx, deleteQuery, charmID{UUID: id.String()}).Run(); err != nil {
			return fmt.Errorf("failed to delete charm: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to run transaction: %w", err)
	}

	return nil
}

// getCharmMetadata returns the metadata for the charm using the charm ID.
// This is the core metadata for the charm.
func (s *State) getCharmMetadata(ctx context.Context, tx *sqlair.TX, ident charmID) (charmMetadata, error) {
	query := `
SELECT &charmMetadata.*
FROM v_charm
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, charmMetadata{}, ident)
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
func (s *State) getCharmTags(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmTag, error) {
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
func (s *State) getCharmCategories(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmCategory, error) {
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
func (s *State) getCharmTerms(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmTerm, error) {
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
func (s *State) getCharmRelations(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmRelation, error) {
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
func (s *State) getCharmExtraBindings(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmExtraBinding, error) {
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
func (s *State) getCharmStorage(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmStorage, error) {
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
func (s *State) getCharmDevices(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmDevice, error) {
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
func (s *State) getCharmPayloads(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmPayload, error) {
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
func (s *State) getCharmResources(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmResource, error) {
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
func (s *State) getCharmContainers(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmContainer, error) {
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

func (s *State) checkSetCharmExists(ctx context.Context, tx *sqlair.TX, id corecharm.ID, name string, revision int) error {
	selectQuery := `
SELECT charm.uuid AS &charmID.*
FROM charm
LEFT JOIN charm_origin ON charm.uuid = charm_origin.charm_uuid
WHERE charm.name = $charmNameRevision.name AND charm_origin.revision = $charmNameRevision.revision

	`
	selectStmt, err := s.Prepare(selectQuery, charmID{}, charmNameRevision{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}
	if err := tx.Query(ctx, selectStmt, charmNameRevision{
		Name:     name,
		Revision: revision,
	}).Get(&charmID{}); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("failed to check charm exists: %w", err)
	}

	return charmerrors.AlreadyExists
}

func (s *State) setCharmHash(ctx context.Context, tx *sqlair.TX, id corecharm.ID, hash string) error {
	ident := charmID{UUID: id.String()}

	query := `INSERT INTO charm_hash (*) VALUES ($setCharmHash.*);`
	stmt, err := s.Prepare(query, setCharmHash{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, setCharmHash{
		CharmUUID:  ident.UUID,
		HashKindID: sha256HashKind,
		Hash:       hash,
	}).Run(); err != nil {
		return fmt.Errorf("failed to insert charm hash: %w", err)
	}

	return nil
}

func (s *State) setCharmInitialOrigin(
	ctx context.Context, tx *sqlair.TX, id corecharm.ID,
	source charm.CharmSource, revision int, version string) error {
	ident := charmID{UUID: id.String()}

	encodedOriginSource, err := encodeOriginSource(source)
	if err != nil {
		return fmt.Errorf("failed to encode charm origin source: %w", err)
	}

	query := `INSERT INTO charm_origin (*) VALUES ($setCharmSourceRevisionVersion.*);`
	stmt, err := s.Prepare(query, setCharmSourceRevisionVersion{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, setCharmSourceRevisionVersion{
		CharmUUID: ident.UUID,
		SourceID:  encodedOriginSource,
		Revision:  revision,
		Version:   version,
	}).Run(); err != nil {
		return fmt.Errorf("failed to insert charm origin: %w", err)
	}

	return nil
}

func (s *State) setCharmMetadata(
	ctx context.Context,
	tx *sqlair.TX,
	id corecharm.ID,
	metadata charm.Metadata,
	lxdProfile []byte,
	archivePath string) error {
	ident := charmID{UUID: id.String()}

	encodedMetadata, err := encodeMetadata(id, metadata, lxdProfile)
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

func (s *State) setCharmTags(ctx context.Context, tx *sqlair.TX, id corecharm.ID, tags []string) error {
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

func (s *State) setCharmCategories(ctx context.Context, tx *sqlair.TX, id corecharm.ID, categories []string) error {
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

func (s *State) setCharmTerms(ctx context.Context, tx *sqlair.TX, id corecharm.ID, terms []string) error {
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

func (s *State) setCharmRelations(ctx context.Context, tx *sqlair.TX, id corecharm.ID, metadata charm.Metadata) error {
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

func (s *State) setCharmExtraBindings(ctx context.Context, tx *sqlair.TX, id corecharm.ID, extraBindings map[string]charm.ExtraBinding) error {
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

func (s *State) setCharmStorage(ctx context.Context, tx *sqlair.TX, id corecharm.ID, storage map[string]charm.Storage) error {
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

func (s *State) setCharmDevices(ctx context.Context, tx *sqlair.TX, id corecharm.ID, devices map[string]charm.Device) error {
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

func (s *State) setCharmPayloads(ctx context.Context, tx *sqlair.TX, id corecharm.ID, payloads map[string]charm.PayloadClass) error {
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

func (s *State) setCharmResources(ctx context.Context, tx *sqlair.TX, id corecharm.ID, resources map[string]charm.Resource) error {
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

func (s *State) setCharmContainers(ctx context.Context, tx *sqlair.TX, id corecharm.ID, containers map[string]charm.Container) error {
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

func (s *State) setCharmActions(ctx context.Context, tx *sqlair.TX, id corecharm.ID, actions charm.Actions) error {
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

func (s *State) setCharmConfig(ctx context.Context, tx *sqlair.TX, id corecharm.ID, config charm.Config) error {
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

func (s *State) setCharmManifest(ctx context.Context, tx *sqlair.TX, id corecharm.ID, manifest charm.Manifest) error {
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

func encodeOriginSource(source charm.CharmSource) (int, error) {
	switch source {
	case charm.LocalSource:
		return 0, nil
	case charm.CharmHubSource:
		return 1, nil
	default:
		return 0, fmt.Errorf("unsupported source type: %q", source)
	}
}
