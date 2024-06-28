// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coreDB "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/database"
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
	uuid string,
	name string,
	providerID network.Id,
	subnetIDs []string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}
	space := Space{UUID: uuid, Name: name}
	insertSpaceStmt, err := st.Prepare(`
INSERT INTO space (uuid, name) 
VALUES ($Space.*)`, space)
	if err != nil {
		return errors.Trace(err)
	}

	providerSpace := ProviderSpace{ProviderID: providerID, SpaceUUID: uuid}
	insertProviderStmt, err := st.Prepare(`
INSERT INTO provider_space (provider_id, space_uuid)
VALUES ($ProviderSpace.*)`, providerSpace)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, insertSpaceStmt, space).Run(); err != nil {
			if database.IsErrConstraintUnique(err) {
				return fmt.Errorf("inserting space uuid %q into space table: %w with err: %w", uuid, networkerrors.ErrSpaceAlreadyExists, err)
			}
			return errors.Annotatef(err, "inserting space uuid %q into space table", uuid)
		}
		if providerID != "" {
			if err := tx.Query(ctx, insertProviderStmt, providerSpace).Run(); err != nil {
				return errors.Annotatef(err, "inserting provider id %q into provider_space table", providerID)
			}
		}

		// Update all subnets (including their fan overlays) to include
		// the space uuid.
		for _, subnetID := range subnetIDs {
			if err := st.updateSubnetSpaceID(
				ctx,
				tx,
				Subnet{
					UUID:      subnetID,
					SpaceUUID: uuid,
				}); err != nil {
				return errors.Annotatef(err, "updating subnet %q using space uuid %q", subnetID, uuid)
			}
		}
		return nil
	})
	return errors.Trace(domain.CoerceError(err))
}

// GetSpace returns the space by UUID.
func (st *State) GetSpace(
	ctx context.Context,
	uuid string,
) (*network.SpaceInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	space := Space{UUID: uuid}
	spacesStmt, err := st.Prepare(`
SELECT (uuid,
       name,
       provider_id,
       subnet_uuid,
       subnet_cidr,
       subnet_vlan_tag,
       subnet_provider_id,
       subnet_provider_network_id,
       subnet_az) AS (&SpaceSubnetRow.*)
FROM   v_space_subnet
WHERE  uuid = $Space.uuid;`, SpaceSubnetRow{}, space)
	if err != nil {
		return nil, errors.Annotate(err, "preparing select space statement")
	}

	var spaceRows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, spacesStmt, space).GetAll(&spaceRows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "retrieving space %q", uuid)
		}
		return err
	}); errors.Is(err, sqlair.ErrNoRows) {
		return nil, fmt.Errorf("space not found with %s: %w", uuid, networkerrors.ErrSpaceNotFound)
	} else if err != nil {
		return nil, errors.Annotate(err, "querying spaces")
	}

	return &spaceRows.ToSpaceInfos()[0], nil
}

// GetSpaceByName returns the space by name.
func (st *State) GetSpaceByName(
	ctx context.Context,
	name string,
) (*network.SpaceInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	space := Space{
		Name: name,
	}

	// Append the space.name condition to the query.
	q := `
SELECT (uuid,
       name,
       provider_id,
       subnet_uuid,
       subnet_cidr,
       subnet_vlan_tag,
       subnet_provider_id,
       subnet_provider_network_id,
       subnet_az) AS (&SpaceSubnetRow.*)
FROM   v_space_subnet
WHERE  name = $Space.name;`

	s, err := st.Prepare(q, SpaceSubnetRow{}, space)
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var rows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s, space).GetAll(&rows))
	}); errors.Is(err, sqlair.ErrNoRows) {
		return nil, fmt.Errorf("space not found with %s: %w", name, networkerrors.ErrSpaceNotFound)
	} else if err != nil {
		return nil, errors.Annotate(domain.CoerceError(err), "querying spaces by name")
	}

	return &rows.ToSpaceInfos()[0], nil
}

// GetAllSpaces returns all spaces for the model.
func (st *State) GetAllSpaces(
	ctx context.Context,
) (network.SpaceInfos, error) {

	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	s, err := st.Prepare(`
SELECT (uuid,
       name,
       provider_id,
       subnet_uuid,
       subnet_cidr,
       subnet_vlan_tag,
       subnet_provider_id,
       subnet_provider_network_id,
       subnet_az) AS (&SpaceSubnetRow.*)
FROM   v_space_subnet
`, SpaceSubnetRow{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing select all spaces statement")
	}

	var rows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s).GetAll(&rows))
	}); errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		st.logger.Errorf("querying all spaces, %v", err)
		return nil, errors.Annotate(domain.CoerceError(err), "querying all spaces")
	}

	return rows.ToSpaceInfos(), nil
}

// UpdateSpace updates the space identified by the passed uuid.
func (st *State) UpdateSpace(
	ctx context.Context,
	uuid string,
	name string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	space := Space{
		UUID: uuid,
		Name: name,
	}
	stmt, err := st.Prepare(`
UPDATE space
SET    name = $Space.name
WHERE  uuid = $Space.uuid;`, space)
	if err != nil {
		return errors.Annotate(err, "preparing update space statement")
	}
	var outcome sqlair.Outcome
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, space).Get(&outcome)
		if err != nil {
			return errors.Annotatef(err, "updating space %q with name %q", uuid, name)
		}
		affected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if affected == 0 {
			return fmt.Errorf("space not found with %s: %w", uuid, networkerrors.ErrSpaceNotFound)
		}
		return nil
	})
	return domain.CoerceError(err)
}

// DeleteSpace deletes the space identified by the passed uuid.
func (st *State) DeleteSpace(
	ctx context.Context,
	uuid string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	space := Space{UUID: uuid}

	deleteSpaceStmt, err := st.Prepare(`
DELETE FROM space 
WHERE       uuid = $Space.uuid;`, space)
	if err != nil {
		return errors.Annotate(err, "preparing delete space statement")
	}
	deleteProviderSpaceStmt, err := st.Prepare(`
DELETE FROM provider_space 
WHERE       space_uuid = $Space.uuid;`, space)
	if err != nil {
		return errors.Annotate(err, "preparing delete provider space statement")
	}
	updateSubnetSpaceUUIDStmt, err := st.Prepare(`
UPDATE subnet 
SET    space_uuid = (SELECT uuid FROM space WHERE name = 'alpha') 
WHERE  space_uuid = $Space.uuid;`, space)
	if err != nil {
		return errors.Annotate(err, "preparing update subnet statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, deleteProviderSpaceStmt, space).Run(); err != nil {
			return errors.Annotatef(err, "removing space %q from the provider_space table", uuid)
		}

		if err := tx.Query(ctx, updateSubnetSpaceUUIDStmt, space).Run(); err != nil {
			return errors.Annotatef(err, "updating subnet table by removing the space %q", uuid)
		}

		var outcome sqlair.Outcome
		err := tx.Query(ctx, deleteSpaceStmt, space).Get(&outcome)
		if err != nil {
			return errors.Annotatef(err, "removing space %q", uuid)
		}
		delSpaceAffected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if delSpaceAffected != 1 {
			return networkerrors.ErrSpaceNotFound
		}

		return nil
	})
	return domain.CoerceError(err)
}
