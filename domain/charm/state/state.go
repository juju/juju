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

	query := `
SELECT name AS &charmName.name
FROM charm
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, charmID{}, charmName{})
	if err != nil {
		return false, fmt.Errorf("failed to prepare query: %w", err)
	}

	var isController bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result charmName
		if err := tx.Query(ctx, stmt, charmID{UUID: id.String()}).Get(&result); err != nil {
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

	query := `
SELECT subordinate AS &charmSubordinate.subordinate
FROM charm
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, charmID{}, charmSubordinate{})
	if err != nil {
		return false, fmt.Errorf("failed to prepare query: %w", err)
	}

	var isSubordinate bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result charmSubordinate
		if err := tx.Query(ctx, stmt, charmID{UUID: id.String()}).Get(&result); err != nil {
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

	query := `
SELECT charm_container.charm_uuid AS &charmID.uuid
FROM charm
LEFT JOIN charm_container
ON charm.uuid = charm_container.charm_uuid
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, charmID{})
	if err != nil {
		return false, fmt.Errorf("failed to prepare query: %w", err)
	}

	var supportsContainers bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result []charmID
		if err := tx.Query(ctx, stmt, charmID{UUID: id.String()}).GetAll(&result); err != nil {
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

	query := `
SELECT charm_state.available AS &charmAvailable.available
FROM charm
INNER JOIN charm_state
ON charm.uuid = charm_state.charm_uuid
WHERE uuid = $charmID.uuid;
`
	stmt, err := s.Prepare(query, charmID{}, charmAvailable{})
	if err != nil {
		return false, fmt.Errorf("failed to prepare query: %w", err)
	}

	var isAvailable bool
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result charmAvailable
		if err := tx.Query(ctx, stmt, charmID{UUID: id.String()}).Get(&result); err != nil {
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

	selectQuery := `
SELECT charm.uuid AS &charmID.*
FROM charm
WHERE uuid = $charmID.uuid;
	`

	selectStmt, err := s.Prepare(selectQuery, charmID{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	updateQuery := `
UPDATE charm_state
SET available = true
WHERE charm_uuid = $charmID.uuid;
`

	updateStmt, err := s.Prepare(updateQuery, charmID{})
	if err != nil {
		return fmt.Errorf("failed to prepare query: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result charmID
		if err := tx.Query(ctx, selectStmt, charmID{UUID: id.String()}).Get(&result); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return charmerrors.NotFound
			}
			return fmt.Errorf("failed to set charm available: %w", err)
		}

		if err := tx.Query(ctx, updateStmt, charmID{UUID: id.String()}).Run(); err != nil {
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

	selectQuery := `
SELECT charm.* AS &charm.*, charm_state.* AS &charmState.*
FROM charm 
LEFT JOIN charm_state
ON charm.uuid = charm_state.charm_uuid
WHERE uuid = $charmID.uuid;
`
	selectStmt, err := s.Prepare(selectQuery, charm{}, charmState{}, charmID{})
	if err != nil {
		return "", fmt.Errorf("failed to prepare query: %w", err)
	}

	insertCharmQuery := `INSERT INTO charm (*) VALUES ($charm.*);`
	insertCharmStmt, err := s.Prepare(insertCharmQuery, charm{})
	if err != nil {
		return "", fmt.Errorf("failed to prepare query: %w", err)
	}

	insertCharmStateQuery := `INSERT INTO charm_state (*) VALUES ($charmState.*);`
	insertCharmStateStmt, err := s.Prepare(insertCharmStateQuery, charmState{})
	if err != nil {
		return "", fmt.Errorf("failed to prepare query: %w", err)
	}

	newID, err := corecharm.NewID()
	if err != nil {
		return "", fmt.Errorf("failed to reserve charm revision: %w", err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var (
			charmResult       charm
			charmsStateResult charmState
		)
		if err := tx.Query(ctx, selectStmt, charmID{UUID: id.String()}).Get(&charmResult, &charmsStateResult); err != nil {
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

		// This is defensive, a simple insert should be enough, but if the
		// charm state is updated, this will at least perform correctly.
		newCharmState := charmsStateResult
		newCharmState.CharmUUID = newID.String()
		newCharmState.Available = false
		if err := tx.Query(ctx, insertCharmStateStmt, newCharmState).Run(); err != nil {
			return fmt.Errorf("failed to reserve charm revision: inserting charm state: %w", err)
		}

		return nil
	}); err != nil {
		return "", fmt.Errorf("failed to run transaction: %w", err)
	}

	return newID, nil
}

// // GetCharmMetadata returns the metadata for the charm using the charm ID.
// // If the charm does not exist, a NotFound error is returned.
// func (s *State) GetCharmMetadata(ctx context.Context, charmID corecharm.ID) (charm.Metadata, error) {}
//
// // GetCharmManifest returns the manifest for the charm using the charm ID.
// // If the charm does not exist, a NotFound error is returned.
// func (s *State) GetCharmManifest(ctx context.Context, charmID corecharm.ID) (charm.Manifest, error) {}
//
// // GetCharmActions returns the actions for the charm using the charm ID.
// // If the charm does not exist, a NotFound error is returned.
// func (s *State) GetCharmActions(ctx context.Context, charmID corecharm.ID) (charm.Actions, error) {}
//
// // GetCharmConfig returns the config for the charm using the charm ID.
// // If the charm does not exist, a NotFound error is returned.
// func (s *State) GetCharmConfig(ctx context.Context, charmID corecharm.ID) (charm.Config, error) {}
//
// // GetCharmLXDProfile returns the LXD profile for the charm using the
// // charm ID.
// // If the charm does not exist, a NotFound error is returned.
// func (s *State) GetCharmLXDProfile(ctx context.Context, charmID corecharm.ID) ([]byte, error) {}
//
// // SetCharm persists the charm metadata, actions, config and manifest to
// // state.
// func (s *State) SetCharm(ctx context.Context, charm charm.Charm) (corecharm.ID, error) {}
