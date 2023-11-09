// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v3"

	coreDB "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory coreDB.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// AddSpace creates and returns a new space.
func (st *State) AddSpace(
	ctx context.Context,
	uuid utils.UUID,
	name string,
	providerID network.Id,
	subnetIDs []string,
) error {
	if !names.IsValidSpace(name) {
		return errors.NewNotValid(nil, fmt.Sprintf("invalid space name '%s'", name))
	}

	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	insertSpaceStmt := `
INSERT INTO space (uuid, name)
VALUES (?, ?)`
	insertProviderStmt := `
INSERT INTO provider_space (provider_id, space_uuid)
VALUES (?, ?)`
	checkSubnetFanLocalUnderlay := `
SELECT subnet.cidr,subnet_type.is_space_settable
FROM   subnet_type
JOIN   subnet
ON     subnet.subnet_type_id = subnet_type.id
WHERE  subnet.uuid = ?`
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, insertSpaceStmt, uuid.String(), name); err != nil {
			return errors.Trace(err)
		}
		if _, err := tx.ExecContext(ctx, insertProviderStmt, providerID, uuid.String()); err != nil {
			return errors.Trace(err)
		}
		for _, subnetID := range subnetIDs {
			// Check if the subnet is a fan overlay, in that case
			// the space cannot be created on this subnet: it must
			// be inherited from the underlay.
			var (
				cidr            string
				isSpaceSettable bool
			)
			row := tx.QueryRowContext(ctx, checkSubnetFanLocalUnderlay, subnetID)
			if err := row.Scan(&cidr, &isSpaceSettable); err != nil {
				return errors.Trace(err)
			}
			if !isSpaceSettable {
				return errors.Errorf(
					"cannot set space for FAN subnet %q - it is always inherited from underlay", cidr)
			}

			if err := updateSubnetSpaceIDTx(ctx, tx, subnetID, uuid.String()); err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	})
	return errors.Trace(err)
}

const retrieveSpacesStmt = `
SELECT     
    space.uuid                           AS &Space.uuid,
    space.name                           AS &Space.name,
    provider_space.provider_id           AS &Space.provider_id,
    subnet.uuid                          AS &Space.subnet_uuid,
    subnet.cidr                          AS &Space.subnet_cidr,
    subnet.vlan_tag                      AS &Space.vlan_tag,
    provider_subnet.provider_id          AS &Space.subnet_provider_id,
    provider_network.provider_network_id AS &Space.subnet_provider_network_id,
    availability_zone.name               AS &Space.subnet_az
FROM space 
    JOIN provider_space
    ON space.uuid = provider_space.space_uuid
    LEFT JOIN subnet   
    ON space.uuid = subnet.space_uuid
    LEFT JOIN provider_subnet
    ON subnet.uuid = provider_subnet.subnet_uuid
    LEFT JOIN provider_network_subnet
    ON subnet.uuid = provider_network_subnet.subnet_uuid
    LEFT JOIN provider_network
    ON provider_network_subnet.provider_network_uuid = provider_network.uuid
    LEFT JOIN availability_zone_subnet
    ON availability_zone_subnet.subnet_uuid = subnet.uuid
    LEFT JOIN availability_zone
    ON availability_zone_subnet.availability_zone_uuid = availability_zone.uuid`

// GetSpace returns the space by UUID.
func (st *State) GetSpace(
	ctx context.Context,
	uuid string,
) (*network.SpaceInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Append the space uuid condition to the query only if it's passed to the function.
	q := retrieveSpacesStmt + " WHERE space.uuid = $M.id;"

	s, err := sqlair.Prepare(q, Space{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var rows Spaces
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s, sqlair.M{"id": uuid}).GetAll(&rows))
	}); err != nil {
		return nil, errors.Annotate(err, "querying spaces")
	}

	if len(rows) == 0 {
		return nil, errors.NotFoundf("space %q", uuid)
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

	// Append the close the q stament since we are retrieving all
	// spaces.
	q := retrieveSpacesStmt + ";"

	s, err := sqlair.Prepare(q, Space{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var rows Spaces
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s).GetAll(&rows))
	}); err != nil {
		return nil, errors.Annotate(err, "querying all spaces")
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
		return errors.Trace(err)
	}

	q := `
UPDATE space
SET    name = ?
WHERE  uuid = ?;`
	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {

		if _, err := tx.ExecContext(ctx, q, name, uuid); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
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

	deleteSpaceStmt := "DELETE FROM space WHERE uuid = ?;"
	deleteProviderSpaceStmt := "DELETE FROM provider_space WHERE space_uuid = ?;"
	updateSubnetSpaceUUIDStmt := "UPDATE subnet SET space_uuid = NULL WHERE space_uuid = ?;"

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		delProviderSpaceResult, err := tx.ExecContext(ctx, deleteProviderSpaceStmt, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		delProviderSpaceAffected, err := delProviderSpaceResult.RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if delProviderSpaceAffected != 1 {
			return fmt.Errorf("provider space id for space %s not found", uuid)
		}

		if _, err := tx.ExecContext(ctx, updateSubnetSpaceUUIDStmt, uuid); err != nil {
			return errors.Trace(err)
		}

		delSpaceResult, err := tx.ExecContext(ctx, deleteSpaceStmt, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		delSpaceAffected, err := delSpaceResult.RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if delSpaceAffected != 1 {
			return fmt.Errorf("space %s not found", uuid)
		}

		return nil
	})
}
