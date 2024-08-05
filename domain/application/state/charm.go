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

// hashKind is the type of hash to store.
type hashKind = int

const (
	// sha256HashKind is the ID for the SHA256 hash kind.
	sha256HashKind hashKind = 0
)

// CharmState is used to access the database.
type CharmState struct {
	*commonStateBase
}

// NewCharmState creates a state to access the database.
func NewCharmState(base *commonStateBase) *CharmState {
	return &CharmState{
		commonStateBase: base,
	}
}

// GetCharmIDByRevision returns the charm ID by the natural key, for a
// specific revision.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmIDByRevision(ctx context.Context, name string, revision int) (corecharm.ID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	var ident charmID
	args := charmNameRevision{
		Name:     name,
		Revision: revision,
	}

	query := `
SELECT &charmID.*
FROM charm
INNER JOIN charm_origin
ON charm.uuid = charm_origin.charm_uuid
WHERE charm.name = $charmNameRevision.name
AND charm_origin.revision = $charmNameRevision.revision;
`
	stmt, err := s.Prepare(query, ident, args)
	if err != nil {
		return "", fmt.Errorf("failed to prepare query: %w", err)
	}

	var id corecharm.ID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, args).Get(&ident); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return fmt.Errorf("failed to get charm ID: %w", err)
		}
		id = corecharm.ID(ident.UUID)
		return nil
	}); err != nil {
		return "", fmt.Errorf("failed to run transaction: %w", err)
	}
	return id, nil
}

// IsControllerCharm returns whether the charm is a controller charm.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) IsControllerCharm(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	var result charmName
	ident := charmID{UUID: id.String()}

	query := `
SELECT name AS &charmName.name
FROM charm
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, ident, result)
	if err != nil {
		return false, fmt.Errorf("failed to prepare query: %w", err)
	}

	var isController bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
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
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) IsSubordinateCharm(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	var result charmSubordinate
	ident := charmID{UUID: id.String()}

	query := `
SELECT subordinate AS &charmSubordinate.subordinate
FROM charm
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, ident, result)
	if err != nil {
		return false, fmt.Errorf("failed to prepare query: %w", err)
	}

	var isSubordinate bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
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
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) SupportsContainers(ctx context.Context, id corecharm.ID) (bool, error) {
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
				return applicationerrors.CharmNotFound
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
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) IsCharmAvailable(ctx context.Context, id corecharm.ID) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	var result charmAvailable
	ident := charmID{UUID: id.String()}

	query := `
SELECT charm_state.available AS &charmAvailable.available
FROM charm
INNER JOIN charm_state
ON charm.uuid = charm_state.charm_uuid
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, ident, result)
	if err != nil {
		return false, fmt.Errorf("failed to prepare query: %w", err)
	}

	var isAvailable bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
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
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) SetCharmAvailable(ctx context.Context, id corecharm.ID) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	selectQuery := `
SELECT &charmID.*
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
				return applicationerrors.CharmNotFound
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
func (s *CharmState) ReserveCharmRevision(ctx context.Context, id corecharm.ID, revision int) (corecharm.ID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	var charmResult charmIDName
	ident := charmID{UUID: id.String()}

	selectQuery := `
SELECT &charmIDName.*
FROM charm 
LEFT JOIN charm_state
ON charm.uuid = charm_state.charm_uuid
WHERE uuid = $charmID.uuid;
`
	selectStmt, err := s.Prepare(selectQuery, charmResult, ident)
	if err != nil {
		return "", fmt.Errorf("failed to prepare query: %w", err)
	}

	insertCharmQuery := `INSERT INTO charm (*) VALUES ($charmIDName.*);`
	insertCharmStmt, err := s.Prepare(insertCharmQuery, charmResult)
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
		if err := tx.Query(ctx, selectStmt, ident).Get(&charmResult); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
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

// GetCharmArchivePath returns the archive storage path for the charm using
// the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmArchivePath(ctx context.Context, id corecharm.ID) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	var archivePath charmArchivePath
	ident := charmID{UUID: id.String()}

	query := `
SELECT &charmArchivePath.*
FROM charm
WHERE uuid = $charmID.uuid;
`

	stmt, err := s.Prepare(query, archivePath, ident)
	if err != nil {
		return "", fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, ident).Get(&archivePath); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.CharmNotFound
			}
			return fmt.Errorf("failed to get charm archive path: %w", err)
		}
		return nil
	}); err != nil {
		return "", fmt.Errorf("failed to run transaction: %w", domain.CoerceError(err))
	}

	return archivePath.ArchivePath, nil
}

// GetCharmMetadata returns the metadata for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmMetadata(ctx context.Context, id corecharm.ID) (charm.Metadata, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Metadata{}, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	var charmMetadata charm.Metadata
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		charmMetadata, err = s.getMetadata(ctx, tx, ident)
		return errors.Trace(err)
	}); err != nil {
		return charm.Metadata{}, fmt.Errorf("failed to run transaction: %w", err)
	}

	return charmMetadata, nil
}

// getMetadata returns the metadata for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *CharmState) getMetadata(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Metadata, error) {
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

// GetCharmManifest returns the manifest for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmManifest(ctx context.Context, id corecharm.ID) (charm.Manifest, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Manifest{}, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	var manifest charm.Manifest
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		manifest, err = s.getCharmManifest(ctx, tx, ident)
		return errors.Trace(err)
	}); err != nil {
		return charm.Manifest{}, fmt.Errorf("failed to run transaction: %w", err)
	}

	return manifest, nil
}

// getCharmManifest returns the manifest for the charm, using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *CharmState) getCharmManifest(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Manifest, error) {
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

// GetCharmLXDProfile returns the LXD profile for the charm using the
// charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmLXDProfile(ctx context.Context, id corecharm.ID) ([]byte, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	var profile []byte
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		profile, err = s.getCharmLXDProfile(ctx, tx, ident)
		return errors.Trace(err)
	}); err != nil {
		return nil, fmt.Errorf("failed to run transaction: %w", err)
	}

	return profile, nil
}

// getCharmLXDProfile returns the LXD profile for the charm using the
// charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *CharmState) getCharmLXDProfile(ctx context.Context, tx *sqlair.TX, ident charmID) ([]byte, error) {
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

// GetCharmConfig returns the config for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmConfig(ctx context.Context, id corecharm.ID) (charm.Config, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Config{}, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	var charmConfig charm.Config
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		charmConfig, err = s.getCharmConfig(ctx, tx, ident)
		return errors.Trace(err)
	}); err != nil {
		return charm.Config{}, fmt.Errorf("failed to run transaction: %w", err)
	}
	return charmConfig, nil

}

// getCharmConfig returns the config for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *CharmState) getCharmConfig(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Config, error) {
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

// GetCharmActions returns the actions for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharmActions(ctx context.Context, id corecharm.ID) (charm.Actions, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Actions{}, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	var actions charm.Actions
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		actions, err = s.getCharmActions(ctx, tx, ident)
		return errors.Trace(err)
	}); err != nil {
		return charm.Actions{}, fmt.Errorf("failed to run transaction: %w", err)
	}

	return actions, nil
}

// getCharmActions returns the actions for the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
// It's safe to do this in the transaction loop, the query will cached against
// the state base, and if the decode fails, the retry logic won't be triggered,
// as it doesn't satisfy the retry error types.
func (s *CharmState) getCharmActions(ctx context.Context, tx *sqlair.TX, ident charmID) (charm.Actions, error) {
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

// GetCharm returns the charm using the charm ID.
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) GetCharm(ctx context.Context, id corecharm.ID) (charm.Charm, error) {
	db, err := s.DB()
	if err != nil {
		return charm.Charm{}, errors.Trace(err)
	}

	ident := charmID{UUID: id.String()}

	var charm charm.Charm
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		if charm.Metadata, err = s.getMetadata(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if charm.Config, err = s.getCharmConfig(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if charm.Manifest, err = s.getCharmManifest(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if charm.Actions, err = s.getCharmActions(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		if charm.LXDProfile, err = s.getCharmLXDProfile(ctx, tx, ident); err != nil {
			return errors.Trace(err)
		}

		return nil
	}); err != nil {
		return charm, fmt.Errorf("failed to run transaction: %w", err)
	}

	return charm, nil
}

// SetCharm persists the charm metadata, actions, config and manifest to
// state.
func (s *CharmState) SetCharm(ctx context.Context, charm charm.Charm, charmArgs charm.SetStateArgs) (corecharm.ID, error) {
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

		if err := s.setCharm(ctx, tx, id, charm, charmArgs.ArchivePath); err != nil {
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
// If the charm does not exist, a [errors.CharmNotFound] error is returned.
func (s *CharmState) DeleteCharm(ctx context.Context, id corecharm.ID) error {
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
				return applicationerrors.CharmNotFound
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
func (s *CharmState) getCharmMetadata(ctx context.Context, tx *sqlair.TX, ident charmID) (charmMetadata, error) {
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
func (s *CharmState) getCharmTags(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmTag, error) {
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
func (s *CharmState) getCharmCategories(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmCategory, error) {
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
func (s *CharmState) getCharmTerms(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmTerm, error) {
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
func (s *CharmState) getCharmRelations(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmRelation, error) {
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
func (s *CharmState) getCharmExtraBindings(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmExtraBinding, error) {
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
func (s *CharmState) getCharmStorage(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmStorage, error) {
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
func (s *CharmState) getCharmDevices(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmDevice, error) {
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
func (s *CharmState) getCharmPayloads(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmPayload, error) {
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
func (s *CharmState) getCharmResources(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmResource, error) {
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
func (s *CharmState) getCharmContainers(ctx context.Context, tx *sqlair.TX, ident charmID) ([]charmContainer, error) {
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

func (s *CharmState) checkSetCharmExists(ctx context.Context, tx *sqlair.TX, id corecharm.ID, name string, revision int) error {
	selectQuery := `
SELECT charm.uuid AS &charmID.*
FROM charm
LEFT JOIN charm_origin ON charm.uuid = charm_origin.charm_uuid
WHERE charm.name = $charmNameRevision.name AND charm_origin.revision = $charmNameRevision.revision

	`
	var result charmID
	selectStmt, err := s.Prepare(selectQuery, result, charmNameRevision{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}
	if err := tx.Query(ctx, selectStmt, charmNameRevision{
		Name:     name,
		Revision: revision,
	}).Get(&result); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("failed to check charm exists: %w", err)
	}

	return applicationerrors.CharmAlreadyExists
}

func (s *CharmState) setCharmHash(ctx context.Context, tx *sqlair.TX, id corecharm.ID, hash string) error {
	ident := charmID{UUID: id.String()}
	args := setCharmHash{
		CharmUUID:  ident.UUID,
		HashKindID: sha256HashKind,
		Hash:       hash,
	}

	query := `INSERT INTO charm_hash (*) VALUES ($setCharmHash.*);`
	stmt, err := s.Prepare(query, args)
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, args).Run(); err != nil {
		return fmt.Errorf("failed to insert charm hash: %w", err)
	}

	return nil
}

func (s *CharmState) setCharmInitialOrigin(
	ctx context.Context, tx *sqlair.TX, id corecharm.ID,
	source charm.CharmSource, revision int, version string) error {
	ident := charmID{UUID: id.String()}

	encodedOriginSource, err := encodeOriginSource(source)
	if err != nil {
		return fmt.Errorf("failed to encode charm origin source: %w", err)
	}

	args := setCharmSourceRevisionVersion{
		CharmUUID: ident.UUID,
		SourceID:  encodedOriginSource,
		Revision:  revision,
		Version:   version,
	}

	query := `INSERT INTO charm_origin (*) VALUES ($setCharmSourceRevisionVersion.*);`
	stmt, err := s.Prepare(query, args)
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := tx.Query(ctx, stmt, args).Run(); err != nil {
		return fmt.Errorf("failed to insert charm origin: %w", err)
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
