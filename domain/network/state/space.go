// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	coreDB "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/environs/config"
)

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
	Errorf(string, ...interface{})
}

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
	logger Logger
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory coreDB.TxnRunnerFactory, logger Logger) *State {
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

	insertSpaceStmt, err := sqlair.Prepare(`
INSERT INTO space (uuid, name) 
VALUES ($Space.uuid, $Space.name)`, Space{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	insertProviderStmt, err := sqlair.Prepare(`
INSERT INTO provider_space (provider_id, space_uuid)
VALUES ($ProviderSpace.provider_id, $ProviderSpace.space_uuid)`, ProviderSpace{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	findFanSubnetsStmt, err := sqlair.Prepare(`
SELECT subject_subnet_uuid AS &Subnet.uuid
FROM   subnet_association
WHERE  association_type_id = 0 AND associated_subnet_uuid IN ($S[:])`, sqlair.S{}, Subnet{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	checkInputSubnetsStmt, err := sqlair.Prepare(`
SELECT &Subnet.uuid
FROM   subnet
JOIN   subnet_type
ON     subnet.subnet_type_id = subnet_type.id
WHERE  subnet_type.is_space_settable = FALSE AND subnet.uuid IN ($S[:])`, sqlair.S{}, Subnet{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	subnetIDsInS := sqlair.S{}
	for _, sid := range subnetIDs {
		subnetIDsInS = append(subnetIDsInS, sid)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// We must first check on the provided subnet ids to validate
		// that are of a type on which the space can be set.

		var nonSettableSubnets []Subnet
		if err := tx.Query(ctx, checkInputSubnetsStmt, subnetIDsInS).GetAll(&nonSettableSubnets); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			st.logger.Errorf("checking if there are fan subnets for space %q, %v", uuid, err)
			return errors.Annotatef(domain.CoerceError(err), "checking if there are fan subnets for space %q", uuid)
		}

		// If any row is returned we must fail with the returned fan
		// subnet uuids.
		if len(nonSettableSubnets) > 0 {
			deduplicatedSubnetUUIDs := set.NewStrings(transform.Slice(nonSettableSubnets, func(s Subnet) string { return s.UUID })...).Values()
			return errors.Errorf(
				"cannot set space for FAN subnet UUIDs %q - it is always inherited from underlay", deduplicatedSubnetUUIDs)
		}

		if err := tx.Query(ctx, insertSpaceStmt, Space{UUID: uuid, Name: name}).Run(); err != nil {
			st.logger.Errorf("inserting space uuid %q into space table, %v", uuid, err)
			return errors.Annotatef(domain.CoerceError(err), "inserting space uuid %q into space table", uuid)
		}
		if providerID != "" {
			if err := tx.Query(ctx, insertProviderStmt, ProviderSpace{ProviderID: providerID, SpaceUUID: uuid}).Run(); err != nil {
				st.logger.Errorf("inserting provider id %q into provider_space table, %v", providerID, err)
				return errors.Annotatef(domain.CoerceError(err), "inserting provider id %q into provider_space table", providerID)
			}
		}

		// Retrieve the fan overlays (if any) of the passed subnet ids.
		var fanSubnets []Subnet
		err = tx.Query(ctx, findFanSubnetsStmt, subnetIDsInS).GetAll(&fanSubnets)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			st.logger.Errorf("retrieving the fan subnets for space %q, %v", uuid, err)
			return errors.Annotatef(domain.CoerceError(err), "retrieving the fan subnets for space %q", uuid)
		}
		// Append the fan subnet (unique) ids (if any) to the provided
		// subnet ids.
		deduplicatedSubnetUUIDs := set.NewStrings(transform.Slice(fanSubnets, func(s Subnet) string { return s.UUID })...).Values()
		subnetIDs = append(subnetIDs, deduplicatedSubnetUUIDs...)

		// Update all subnets (including their fan overlays) to include
		// the space uuid.
		for _, subnetID := range subnetIDs {
			if err := st.updateSubnetSpaceID(ctx, tx, subnetID, uuid); err != nil {
				st.logger.Errorf("updating subnet %q using space uuid %q, %v", subnetID, uuid, err)
				return errors.Annotatef(domain.CoerceError(err), "updating subnet %q using space uuid %q", subnetID, uuid)
			}
		}
		return nil
	})
	return errors.Trace(domain.CoerceError(err))
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
		return nil, errors.Trace(domain.CoerceError(err))
	}

	// Append the space uuid condition to the query only if it's passed to the function.
	q := retrieveSpacesStmt + " WHERE space.uuid = $M.id;"

	spacesStmt, err := sqlair.Prepare(q, SpaceSubnetRow{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(domain.CoerceError(err), "preparing %q", q)
	}

	var spaceRows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, spacesStmt, sqlair.M{"id": uuid}).GetAll(&spaceRows)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			st.logger.Errorf("retrieving space %q, %v", uuid, err)
			return errors.Annotatef(domain.CoerceError(err), "retrieving space %q", uuid)
		}

		return nil
	}); errors.Is(err, sqlair.ErrNoRows) || len(spaceRows) == 0 {
		return nil, errors.NotFoundf("space %q", uuid)
	} else if err != nil {
		st.logger.Errorf("querying spaces, %v", err)
		return nil, errors.Annotate(domain.CoerceError(err), "querying spaces")
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
		return nil, errors.Trace(domain.CoerceError(err))
	}

	// Append the space.name condition to the query.
	q := retrieveSpacesStmt + " WHERE space.name = $M.name;"

	s, err := sqlair.Prepare(q, SpaceSubnetRow{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(domain.CoerceError(err), "preparing %q", q)
	}

	var rows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s, sqlair.M{"name": name}).GetAll(&rows))
	}); errors.Is(err, sqlair.ErrNoRows) || len(rows) == 0 {
		return nil, errors.NotFoundf("space with name %q", name)
	} else if err != nil {
		st.logger.Errorf("querying spaces by name %q, %v", name, err)
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
		return nil, errors.Trace(domain.CoerceError(err))
	}

	s, err := sqlair.Prepare(retrieveSpacesStmt, SpaceSubnetRow{})
	if err != nil {
		return nil, errors.Annotatef(domain.CoerceError(err), "preparing %q", retrieveSpacesStmt)
	}

	var rows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s).GetAll(&rows))
	}); errors.Is(err, sqlair.ErrNoRows) || len(rows) == 0 {
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

	q := `
UPDATE space
SET    name = ?
WHERE  uuid = ?;`
	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, q, name, uuid)
		if err != nil {
			st.logger.Errorf("updating space %q with name %q, %v", uuid, name, err)
			return errors.Trace(domain.CoerceError(err))
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return errors.Trace(domain.CoerceError(err))
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
		return errors.Trace(domain.CoerceError(err))
	}

	deleteSpaceStmt := "DELETE FROM space WHERE uuid = ?;"
	deleteProviderSpaceStmt := "DELETE FROM provider_space WHERE space_uuid = ?;"
	updateSubnetSpaceUUIDStmt := "UPDATE subnet SET space_uuid = ? WHERE space_uuid = ?;"

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, deleteProviderSpaceStmt, uuid); err != nil {
			st.logger.Errorf("removing space %q from the provider_space table, %v", uuid, err)
			return errors.Trace(domain.CoerceError(err))
		}

		if _, err := tx.ExecContext(ctx, updateSubnetSpaceUUIDStmt, network.AlphaSpaceId, uuid); err != nil {
			st.logger.Errorf("updating subnet table by removing the space %q, %v", uuid, err)
			return errors.Trace(domain.CoerceError(err))
		}

		delSpaceResult, err := tx.ExecContext(ctx, deleteSpaceStmt, uuid)
		if err != nil {
			st.logger.Errorf("removing space %q, %v", uuid, err)
			return errors.Trace(domain.CoerceError(err))
		}
		delSpaceAffected, err := delSpaceResult.RowsAffected()
		if err != nil {
			return errors.Trace(domain.CoerceError(err))
		}
		if delSpaceAffected != 1 {
			return fmt.Errorf("space %s not found", uuid)
		}

		return nil
	})
}

// FanConfig returns the current model's fan config value.
func (st *State) FanConfig(ctx context.Context) (string, error) {
	var fanConfig string

	db, err := st.DB()
	if err != nil {
		return fanConfig, errors.Trace(err)
	}

	return fanConfig, db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := `SELECT value FROM model_config WHERE key=?`
		row := tx.QueryRowContext(ctx, stmt, config.FanConfig)
		if err := row.Scan(&fanConfig); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("model's fan config %w%w", errors.NotFound, errors.Hide(err))
		} else if err != nil {
			return domain.CoerceError(err)
		}
		return nil
	})
}
