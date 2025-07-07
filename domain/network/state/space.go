// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coreDB "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory coreDB.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// AddSpace creates and returns a new space.
func (st *State) AddSpace(
	ctx context.Context,
	uuid network.SpaceUUID,
	name network.SpaceName,
	providerID network.Id,
	subnetIDs []string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}
	sp := space{UUID: uuid, Name: name}
	insertSpaceStmt, err := st.Prepare(`
INSERT INTO space (uuid, name) 
VALUES ($space.*)`, sp)
	if err != nil {
		return errors.Capture(err)
	}

	providerSp := providerSpace{ProviderID: providerID, SpaceUUID: uuid}
	insertProviderStmt, err := st.Prepare(`
INSERT INTO provider_space (provider_id, space_uuid)
VALUES ($providerSpace.*)`, providerSp)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, insertSpaceStmt, sp).Run(); err != nil {
			if database.IsErrConstraintUnique(err) {
				return errors.Errorf("inserting space uuid %q into space table: %w with err: %w", uuid, networkerrors.SpaceAlreadyExists, err)
			}
			return errors.Errorf("inserting space uuid %q into space table: %w", uuid, err)
		}
		if providerID != "" {
			if err := tx.Query(ctx, insertProviderStmt, providerSp).Run(); err != nil {
				return errors.Errorf("inserting provider id %q into provider_space table: %w", providerID, err)
			}
		}

		// Update all subnets (including their fan overlays) to include
		// the space uuid.
		for _, subnetID := range subnetIDs {
			if err := st.updateSubnetSpaceID(
				ctx,
				tx,
				subnet{
					UUID:      subnetID,
					SpaceUUID: uuid,
				}); err != nil {
				return errors.Errorf("updating subnet %q using space uuid %q: %w", subnetID, uuid, err)
			}
		}
		return nil
	})
	return errors.Capture(err)
}

// GetSpace returns the space by UUID. If the space is not found, an error is
// returned matching [networkerrors.SpaceNotFound].
func (st *State) GetSpace(
	ctx context.Context,
	uuid network.SpaceUUID,
) (*network.SpaceInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	sp := space{UUID: uuid}
	spacesStmt, err := st.Prepare(`
SELECT &spaceSubnetRow.*
FROM   v_space_subnet
WHERE  uuid = $space.uuid;`, spaceSubnetRow{}, sp)
	if err != nil {
		return nil, errors.Errorf("preparing select space statement: %w", err)
	}

	var spaceRows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, spacesStmt, sp).GetAll(&spaceRows)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("space not found with %s: %w", uuid, networkerrors.SpaceNotFound)
			}
			return errors.Errorf("retrieving space %q: %w", uuid, err)
		}
		return err
	}); err != nil {
		return nil, errors.Errorf("querying spaces: %w", err)
	}

	return &spaceRows.ToSpaceInfos()[0], nil
}

// GetSpaceByName returns the space by name. If the space is not found, an
// error is returned matching [networkerrors.SpaceNotFound].
func (st *State) GetSpaceByName(
	ctx context.Context,
	name network.SpaceName,
) (*network.SpaceInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	sp := space{
		Name: name,
	}

	// Append the space.name condition to the query.
	q := `
SELECT &spaceSubnetRow.*
FROM   v_space_subnet
WHERE  name = $space.name;`

	s, err := st.Prepare(q, spaceSubnetRow{}, sp)
	if err != nil {
		return nil, errors.Errorf("preparing %q: %w", q, err)
	}

	var rows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := errors.Capture(tx.Query(ctx, s, sp).GetAll(&rows))
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("space not found with %s: %w", name, networkerrors.SpaceNotFound)
			}
			return errors.Errorf("querying spaces by name: %w", err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return &rows.ToSpaceInfos()[0], nil
}

// GetAllSpaces returns all spaces for the model.
func (st *State) GetAllSpaces(
	ctx context.Context,
) (network.SpaceInfos, error) {

	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	s, err := st.Prepare(`
SELECT &spaceSubnetRow.*
FROM   v_space_subnet
`, spaceSubnetRow{})
	if err != nil {
		return nil, errors.Errorf("preparing select all spaces statement: %w", err)
	}

	var rows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, s).GetAll(&rows); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return errors.Errorf("querying all spaces: %w", err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return rows.ToSpaceInfos(), nil
}

// UpdateSpace updates the space identified by the passed uuid. If the space is
// not found, an error is returned matching [networkerrors.SpaceNotFound].
func (st *State) UpdateSpace(
	ctx context.Context,
	uuid network.SpaceUUID,
	name network.SpaceName,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	sp := space{
		UUID: uuid,
		Name: name,
	}
	stmt, err := st.Prepare(`
UPDATE space
SET    name = $space.name
WHERE  uuid = $space.uuid;`, sp)
	if err != nil {
		return errors.Errorf("preparing update space statement: %w", err)
	}
	var outcome sqlair.Outcome
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, sp).Get(&outcome)
		if err != nil {
			return errors.Errorf("updating space %q with name %q: %w", uuid, name, err)
		}
		affected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Capture(err)
		}
		if affected == 0 {
			return errors.Errorf("space not found with %s: %w", uuid, networkerrors.SpaceNotFound)
		}
		return nil
	})
	return err
}

// DeleteSpace deletes the space identified by the passed uuid. If the space is
// not found, an error is returned matching [networkerrors.SpaceNotFound].
func (st *State) DeleteSpace(
	ctx context.Context,
	uuid network.SpaceUUID,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	sp := space{UUID: uuid}

	deleteSpaceStmt, err := st.Prepare(`
DELETE FROM space 
WHERE       uuid = $space.uuid;`, sp)
	if err != nil {
		return errors.Errorf("preparing delete space statement: %w", err)
	}
	deleteProviderSpaceStmt, err := st.Prepare(`
DELETE FROM provider_space 
WHERE       space_uuid = $space.uuid;`, sp)
	if err != nil {
		return errors.Errorf("preparing delete provider space statement: %w", err)
	}
	updateSubnetSpaceUUIDStmt, err := st.Prepare(`
UPDATE subnet 
SET    space_uuid = (SELECT uuid FROM space WHERE name = 'alpha') 
WHERE  space_uuid = $space.uuid;`, sp)
	if err != nil {
		return errors.Errorf("preparing update subnet statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, deleteProviderSpaceStmt, sp).Run(); err != nil {
			return errors.Errorf("removing space %q from the provider_space table: %w", uuid, err)
		}

		if err := tx.Query(ctx, updateSubnetSpaceUUIDStmt, sp).Run(); err != nil {
			return errors.Errorf("updating subnet table by removing the space %q: %w", uuid, err)
		}

		var outcome sqlair.Outcome
		err := tx.Query(ctx, deleteSpaceStmt, sp).Get(&outcome)
		if err != nil {
			return errors.Errorf("removing space %q: %w", uuid, err)
		}
		delSpaceAffected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Capture(err)
		}
		if delSpaceAffected != 1 {
			return networkerrors.SpaceNotFound
		}

		return nil
	})
	return err
}

// IsSpaceUsedInConstraints checks if the provided space name is used in any
// constraints.
// This method doesn't check if the provided space name exists, it returns
// false in that case.
func (st *State) IsSpaceUsedInConstraints(ctx context.Context, name network.SpaceName) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	sp := spaceName{Name: string(name)}
	stmt, err := st.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM (
	SELECT NULL AS n
	FROM v_model_constraint_space
	WHERE space = $spaceName.name
	UNION 
	SELECT NULL AS n
	FROM v_application_constraint
	WHERE space_name = $spaceName.name
);`, countResult{}, sp)
	if err != nil {
		return false, errors.Errorf("preparing count space in constraints statement: %w", err)
	}

	var count countResult
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, sp).Get(&count)
		if err != nil {
			return errors.Errorf("counting the space %q in constraints: %w", name, err)
		}
		return nil
	}); err != nil {
		return false, errors.Capture(err)
	}

	return count.Count > 0, nil
}
