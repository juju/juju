// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	coreDB "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/database"
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

	subnetBinds, subnetVals := database.SliceToPlaceholder(subnetIDs)
	findFanSubnetsStmt := fmt.Sprintf(`
SELECT subject_subnet_uuid
FROM   subnet_association
WHERE  association_type_id = 0 AND associated_subnet_uuid IN (%s)`, subnetBinds)

	checkInputSubnetsStmt := fmt.Sprintf(`
SELECT uuid
FROM   subnet
JOIN   subnet_type
ON     subnet.subnet_type_id = subnet_type.id
WHERE  subnet_type.is_space_settable = FALSE AND subnet.uuid IN (%s)`, subnetBinds)

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// We must first check on the provided subnet ids to validate
		// that are of a type on which the space can be set.
		nonSettableSubnets, err := tx.QueryContext(ctx, checkInputSubnetsStmt, subnetVals...)
		if err != nil {
			return errors.Annotatef(err, "checking if there are fan subnets for space %q", uuid.String())
		}
		defer func() { _ = nonSettableSubnets.Close() }()
		// If any row is returned we must fail with the returned fan
		// subnet uuids.
		uniqueErrorSubnetUUIDs := make(map[string]string)
		var nonSettableUUIDs []string
		for nonSettableSubnets.Next() {
			var fanSubnetID string
			err = nonSettableSubnets.Scan(&fanSubnetID)
			if err != nil {
				return errors.Trace(err)
			}
			if _, ok := uniqueErrorSubnetUUIDs[fanSubnetID]; !ok {
				uniqueErrorSubnetUUIDs[fanSubnetID] = fanSubnetID
				nonSettableUUIDs = append(nonSettableUUIDs, fanSubnetID)
			}
		}
		if len(nonSettableUUIDs) > 0 {
			return errors.Errorf(
				"cannot set space for FAN subnet UUIDs %q - it is always inherited from underlay", nonSettableUUIDs)
		}

		if _, err := tx.ExecContext(ctx, insertSpaceStmt, uuid.String(), name); err != nil {
			return errors.Annotatef(err, "inserting space uuid %q into space table", uuid.String())
		}
		if providerID != "" {
			if _, err := tx.ExecContext(ctx, insertProviderStmt, providerID, uuid.String()); err != nil {
				return errors.Annotatef(err, "inserting provider id %q into provider_space table", providerID)
			}
		}

		// Retrieve the fan overlays (if any) of the passed subnet ids.
		rows, err := tx.QueryContext(ctx, findFanSubnetsStmt, subnetVals...)
		if err != nil {
			return errors.Annotatef(err, "retrieving the fan subnets for space %q", uuid.String())
		}
		defer func() { _ = rows.Close() }()
		// Append the fan subnet (unique) ids (if any) to the provided
		// subnet ids.
		uniqueFanSubnetIDs := make(map[string]string)
		for rows.Next() {
			var fanSubnetID string
			err = rows.Scan(&fanSubnetID)
			if err != nil {
				return errors.Trace(err)
			}
			if _, ok := uniqueFanSubnetIDs[fanSubnetID]; !ok {
				uniqueFanSubnetIDs[fanSubnetID] = fanSubnetID
			}
		}
		for _, fanSubnetID := range uniqueFanSubnetIDs {
			subnetIDs = append(subnetIDs, fanSubnetID)
		}

		// Update all subnets (including their fan overlays) to include
		// the space uuid.
		for _, subnetID := range subnetIDs {
			if err := updateSubnetSpaceIDTx(ctx, tx, subnetID, uuid.String()); err != nil {
				return errors.Annotatef(err, "updating subnet %q using space uuid %q", subnetID, uuid.String())
			}
		}
		return nil
	})
	return errors.Trace(err)
}

const retrieveSpacesStmt = `
SELECT     
    space.uuid                           AS &SpaceSubnetRow.uuid,
    space.name                           AS &SpaceSubnetRow.name,
    provider_space.provider_id           AS &SpaceSubnetRow.provider_id,
    subnet.uuid                          AS &SpaceSubnetRow.subnet_uuid,
    subnet.cidr                          AS &SpaceSubnetRow.subnet_cidr,
    subnet.vlan_tag                      AS &SpaceSubnetRow.subnet_vlan_tag,
    provider_subnet.provider_id          AS &SpaceSubnetRow.subnet_provider_id,
    provider_network.provider_network_id AS &SpaceSubnetRow.subnet_provider_network_id,
    availability_zone.name               AS &SpaceSubnetRow.subnet_az
FROM space 
    LEFT JOIN provider_space
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

	spacesStmt, err := sqlair.Prepare(q, SpaceSubnetRow{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var spaceRows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, spacesStmt, sqlair.M{"id": uuid}).GetAll(&spaceRows)
		if err != nil {
			return errors.Annotatef(err, "retrieving space %q", uuid)
		}

		return nil
	}); err != nil {
		return nil, errors.Annotate(err, "querying spaces")
	}
	if len(spaceRows) == 0 {
		return nil, errors.NotFoundf("space %q", uuid)
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

	// Append the space.name condition to the query.
	q := retrieveSpacesStmt + " WHERE space.name = $M.name;"

	s, err := sqlair.Prepare(q, SpaceSubnetRow{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var rows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s, sqlair.M{"name": name}).GetAll(&rows))
	}); err != nil {
		return nil, errors.Annotate(err, "querying spaces by name")
	}
	if len(rows) == 0 {
		return nil, errors.NotFoundf("space with name %q", name)
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

	s, err := sqlair.Prepare(retrieveSpacesStmt, SpaceSubnetRow{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", retrieveSpacesStmt)
	}

	var rows SpaceSubnetRows
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
		res, err := tx.ExecContext(ctx, q, name, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if affected == 0 {
			return errors.NotFoundf("space %q", uuid)
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
	updateSubnetSpaceUUIDStmt := "UPDATE subnet SET space_uuid = ? WHERE space_uuid = ?;"

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

		if _, err := tx.ExecContext(ctx, updateSubnetSpaceUUIDStmt, network.AlphaSpaceId, uuid); err != nil {
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
