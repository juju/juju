// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/google/uuid"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	internaldatabase "github.com/juju/juju/internal/database"
)

// AllSubnetsQuery returns the SQL query that finds all subnet UUIDs from the
// subnet table, needed for the subnets watcher.
func (st *State) AllSubnetsQuery(ctx context.Context, db database.TxnRunner) ([]string, error) {
	var subnetUUIDs []string

	return subnetUUIDs, db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := "SELECT uuid FROM subnet"
		rows, err := tx.QueryContext(ctx, stmt)
		if err != nil {
			return errors.Trace(domain.CoerceError(err))
		}
		defer rows.Close()

		for rows.Next() {
			var subnetUUID string
			if err := rows.Scan(&subnetUUID); err != nil {
				return errors.Trace(domain.CoerceError(err))
			}
			subnetUUIDs = append(subnetUUIDs, subnetUUID)
		}

		return nil
	})
}

// UpsertSubnets updates or adds each one of the provided subnets in one
// transaction.
func (st *State) UpsertSubnets(ctx context.Context, subnets []network.SubnetInfo) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, subnet := range subnets {
			err := st.updateSubnetSpaceID(
				ctx,
				tx,
				string(subnet.ID),
				subnet.SpaceID,
			)
			if err != nil && !errors.Is(err, errors.NotFound) {
				return errors.Trace(domain.CoerceError(err))
			}
			// If the subnet doesn't exist yet we need to create it.
			if errors.Is(err, errors.NotFound) {
				if err := st.addSubnet(
					ctx,
					tx,
					subnet.ID.String(),
					subnet,
				); err != nil {
					return errors.Trace(domain.CoerceError(err))
				}
			}
		}
		return nil
	})
}

func (st *State) addSubnet(ctx context.Context, tx *sqlair.TX, subnetUUID string, subnetInfo network.SubnetInfo) error {
	spaceUUIDValue := subnetInfo.SpaceID
	if subnetInfo.SpaceID == "" {
		spaceUUIDValue = network.AlphaSpaceId
	}

	insertSubnetStmt, err := st.Prepare(`
INSERT INTO subnet (uuid, cidr, vlan_tag, space_uuid)
VALUES ($Subnet.uuid, $Subnet.cidr, $Subnet.vlan_tag, $Subnet.space_uuid)`, Subnet{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}
	insertSubnetProviderIDStmt, err := st.Prepare(`
INSERT INTO provider_subnet (provider_id, subnet_uuid)
VALUES ($ProviderSubnet.provider_id, $ProviderSubnet.subnet_uuid)`, ProviderSubnet{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}
	retrieveProviderNetworkUUIDStmt, err := st.Prepare(`
SELECT &ProviderNetwork.uuid
FROM   provider_network
WHERE  provider_network_id = $ProviderNetwork.provider_network_id`, ProviderNetwork{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}
	insertSubnetProviderNetworkIDStmt, err := st.Prepare(`
INSERT INTO provider_network (uuid, provider_network_id)
VALUES ($ProviderNetwork.uuid, $ProviderNetwork.provider_network_id)`, ProviderNetwork{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}
	insertSubnetProviderNetworkSubnetStmt, err := st.Prepare(`
INSERT INTO provider_network_subnet (provider_network_uuid, subnet_uuid)
VALUES ($ProviderNetworkSubnet.provider_network_uuid, $ProviderNetworkSubnet.subnet_uuid)`, ProviderNetworkSubnet{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}
	// Add the subnet entity.
	if err := tx.Query(
		ctx,
		insertSubnetStmt,
		Subnet{
			UUID:      subnetUUID,
			CIDR:      subnetInfo.CIDR,
			VLANtag:   subnetInfo.VLANTag,
			SpaceUUID: spaceUUIDValue,
		},
	).Run(); err != nil {
		st.logger.Errorf("inserting subnet %q, %v", subnetInfo.CIDR, err)
		return errors.Trace(domain.CoerceError(err))
	}

	// Add the subnet uuid to the provider ids table.
	if err := tx.Query(
		ctx,
		insertSubnetProviderIDStmt,
		ProviderSubnet{SubnetUUID: subnetUUID, ProviderID: subnetInfo.ProviderId},
	).Run(); err != nil {
		if internaldatabase.IsErrConstraintPrimaryKey(err) || internaldatabase.IsErrConstraintUnique(err) {
			st.logger.Debugf("inserting provider id %q for subnet %q, %v", subnetInfo.ProviderId, subnetUUID, err)
			return errors.AlreadyExistsf("provider id %q for subnet %q", subnetInfo.ProviderId, subnetUUID)

		}
		st.logger.Errorf("inserting provider id %q for subnet %q, %v", subnetInfo.ProviderId, subnetUUID, err)
		return errors.Annotatef(domain.CoerceError(err), "inserting provider id %q for subnet %q", subnetInfo.ProviderId, subnetUUID)
	}

	var pnUUIDStr string
	var providerNetwork ProviderNetwork
	err = tx.Query(ctx, retrieveProviderNetworkUUIDStmt, ProviderNetwork{ProviderNetworkID: subnetInfo.ProviderNetworkId}).Get(&providerNetwork)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		st.logger.Errorf("retrieving provider network ID %q for subnet %q, %v", subnetInfo.ProviderNetworkId, subnetUUID, err)
		return errors.Annotatef(domain.CoerceError(err), "retrieving provider network ID %q for subnet %q", subnetInfo.ProviderNetworkId, subnetUUID)
	} else if errors.Is(err, sql.ErrNoRows) {
		// If the provider network doesn't exist, insert it.
		pnUUID, err := uuid.NewV7()
		if err != nil {
			return errors.Trace(err)
		}
		pnUUIDStr = pnUUID.String()

		// Add the provider network id and its uuid to the
		// provider_network table.
		if err := tx.Query(
			ctx,
			insertSubnetProviderNetworkIDStmt,
			ProviderNetwork{ProviderNetworkUUID: pnUUIDStr, ProviderNetworkID: subnetInfo.ProviderNetworkId},
		).Run(); err != nil {
			st.logger.Errorf("inserting provider network id %q for subnet %q, %v", subnetInfo.ProviderNetworkId, subnetUUID, err)
			return errors.Annotatef(domain.CoerceError(err), "inserting provider network id %q for subnet %q", subnetInfo.ProviderNetworkId, subnetUUID)
		}
	} else {
		pnUUIDStr = providerNetwork.ProviderNetworkUUID
	}

	// Insert the providerNetworkUUID into provider network to
	// subnets mapping table.
	if err := tx.Query(
		ctx,
		insertSubnetProviderNetworkSubnetStmt,
		ProviderNetworkSubnet{SubnetUUID: subnetUUID, ProviderNetworkUUID: pnUUIDStr},
	).Run(); err != nil {
		st.logger.Errorf("inserting association between provider network id %q and subnet %q, %v", subnetInfo.ProviderNetworkId, subnetUUID, err)
		return errors.Annotatef(domain.CoerceError(err), "inserting association between provider network id (%q) %q and subnet %q", pnUUIDStr, subnetInfo.ProviderNetworkId, subnetUUID)
	}

	return st.addAvailabilityZones(ctx, tx, subnetUUID, subnetInfo)
}

// addAvailabilityZones adds the availability zones of a subnet if they don't exist, and
// update the availability_zone_subnet table with the subnet's id.
func (st *State) addAvailabilityZones(ctx context.Context, tx *sqlair.TX, subnetUUID string, subnet network.SubnetInfo) error {
	retrieveAvailabilityZoneStmt, err := st.Prepare(`
SELECT &M.uuid
FROM   availability_zone
WHERE  name = $M.name`, sqlair.M{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}
	insertAvailabilityZoneStmt, err := st.Prepare(`
INSERT INTO availability_zone (uuid, name)
VALUES ($M.uuid, $M.name)`, sqlair.M{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}
	insertAvailabilityZoneSubnetStmt, err := st.Prepare(`
INSERT INTO availability_zone_subnet (availability_zone_uuid, subnet_uuid)
VALUES ($M.availability_zone_uuid, $M.subnet_uuid)`, sqlair.M{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	for _, az := range subnet.AvailabilityZones {
		// Retrieve the availability zone.
		m := sqlair.M{}
		err := tx.Query(ctx, retrieveAvailabilityZoneStmt, sqlair.M{"name": az}).Get(m)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			st.logger.Errorf("retrieving availability zone %q for subnet %q, %v", az, subnetUUID, err)
			return errors.Annotatef(domain.CoerceError(err), "retrieving availability zone %q for subnet %q", az, subnetUUID)
		}
		azUUIDStr, _ := m["uuid"]

		// If it doesn't exist, add the availability zone.
		if errors.Is(err, sql.ErrNoRows) {
			azUUID, err := uuid.NewV7()
			if err != nil {
				return errors.Annotatef(err, "generating UUID for availability zone %q for subnet %q", az, subnetUUID)
			}
			azUUIDStr = azUUID.String()
			if err := tx.Query(
				ctx,
				insertAvailabilityZoneStmt,
				sqlair.M{"uuid": azUUIDStr, "name": az},
			).Run(); err != nil {
				st.logger.Errorf("inserting availability zone %q for subnet %q, %v", az, subnetUUID, err)
				return errors.Annotatef(domain.CoerceError(err), "inserting availability zone %q for subnet %q", az, subnetUUID)
			}
		}
		// Add the subnet id along with the az uuid into the
		// availability_zone_subnet mapping table.
		if err := tx.Query(
			ctx,
			insertAvailabilityZoneSubnetStmt,
			sqlair.M{"availability_zone_uuid": azUUIDStr, "subnet_uuid": subnetUUID},
		).Run(); err != nil {
			st.logger.Errorf("inserting availability zone %q association with subnet %q, %v", az, subnetUUID, err)
			return errors.Annotatef(domain.CoerceError(err), "inserting availability zone %q association with subnet %q", az, subnetUUID)
		}
	}
	return nil
}

// AddSubnet creates a subnet.
func (st *State) AddSubnet(
	ctx context.Context,
	subnet network.SubnetInfo,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	return errors.Trace(
		db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			return st.addSubnet(ctx, tx, subnet.ID.String(), subnet)
		}),
	)
}

// GetAllSubnets returns all known subnets in the model.
func (st *State) GetAllSubnets(
	ctx context.Context,
) (network.SubnetInfos, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	// Append the space uuid condition to the query only if it's passed to the function.
	q := `
SELECT &SubnetRow.*
FROM   v_subnet
`

	s, err := st.Prepare(q, SubnetRow{})
	if err != nil {
		return nil, errors.Annotatef(domain.CoerceError(err), "preparing %q", q)
	}

	var rows SubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s).GetAll(&rows))
	}); errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		st.logger.Errorf("querying subnets, %v", err)
		return nil, errors.Annotate(domain.CoerceError(err), "querying subnets")
	}

	return rows.ToSubnetInfos(), nil
}

// GetSubnet returns the subnet by UUID.
func (st *State) GetSubnet(
	ctx context.Context,
	uuid string,
) (*network.SubnetInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	// Append the space uuid condition to the query only if it's passed to the function.
	q := `
SELECT &SubnetRow.*
FROM   v_subnet
WHERE  subnet_uuid = $M.id;`

	stmt, err := st.Prepare(q, SubnetRow{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(domain.CoerceError(err), "preparing %q", q)
	}

	var rows SubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, stmt, sqlair.M{"id": uuid}).GetAll(&rows))
	}); errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.NotFoundf("subnet %q", uuid)
	} else if err != nil {
		st.logger.Errorf("retrieving subnet %q, %v", uuid, err)
		return nil, errors.Annotatef(domain.CoerceError(err), "retrieving subnet %q", uuid)
	}

	return &rows.ToSubnetInfos()[0], nil
}

// GetSubnetsByCIDR returns the subnets by CIDR.
// Deprecated, this method should be removed when we re-work the API for moving
// subnets.
func (st *State) GetSubnetsByCIDR(
	ctx context.Context,
	cidrs ...string,
) (network.SubnetInfos, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}

	// Append the where clause to the query.
	q := `
SELECT &SubnetRow.*
FROM   v_subnet
WHERE  subnet_cidr = $M.cidr`

	s, err := st.Prepare(q, SubnetRow{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(domain.CoerceError(err), "preparing %q", q)
	}

	var resultSubnets SubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, cidr := range cidrs {
			var rows SubnetRows
			if err := tx.Query(ctx, s, sqlair.M{"cidr": cidr}).GetAll(&rows); err != nil {
				return errors.Trace(domain.CoerceError(err))
			}
			resultSubnets = append(resultSubnets, rows...)
		}
		return nil
	}); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		st.logger.Errorf("retrieving subnets by CIDRs %v, %v", cidrs, err)
		return nil, errors.Annotatef(domain.CoerceError(err), "retrieving subnets by CIDRs %v", cidrs)
	}

	return resultSubnets.ToSubnetInfos(), nil
}

func (st *State) updateSubnetSpaceID(
	ctx context.Context,
	tx *sqlair.TX,
	uuid string,
	spaceID string,
) error {
	updateSubnetSpaceIDStmt, err := st.Prepare(`
UPDATE subnet
SET    space_uuid = $M.space_uuid
WHERE  uuid = $M.uuid;`, sqlair.M{})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	var outcome sqlair.Outcome

	if err = tx.Query(ctx, updateSubnetSpaceIDStmt, sqlair.M{"space_uuid": spaceID, "uuid": uuid}).Get(&outcome); err != nil {
		st.logger.Errorf("updating subnet %q space ID %v, %v", uuid, spaceID, err)
		return errors.Trace(domain.CoerceError(err))
	}
	affected, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}
	if affected != 1 {
		return errors.NotFoundf("subnet %q", uuid)
	}

	return nil
}

// UpdateSubnet updates the subnet identified by the passed uuid.
func (st *State) UpdateSubnet(
	ctx context.Context,
	uuid string,
	spaceID string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.updateSubnetSpaceID(ctx, tx, uuid, spaceID)
	})
}

// DeleteSubnet deletes the subnet identified by the passed uuid.
func (st *State) DeleteSubnet(
	ctx context.Context,
	uuid string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	deleteSubnetStmt := "DELETE FROM subnet WHERE uuid = ?;"
	selectProviderNetworkStmt := "SELECT provider_network_uuid FROM provider_network_subnet WHERE subnet_uuid = ?;"
	deleteProviderNetworkStmt := "DELETE FROM provider_network WHERE uuid = ?;"
	deleteProviderNetworkSubnetStmt := "DELETE FROM provider_network_subnet WHERE subnet_uuid = ?;"
	deleteProviderSubnetStmt := "DELETE FROM provider_subnet WHERE subnet_uuid = ?;"
	deleteAvailabilityZoneSubnetStmt := "DELETE FROM availability_zone_subnet WHERE subnet_uuid = ?;"

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, selectProviderNetworkStmt, uuid)
		var providerNetworkUUID string
		err = row.Scan(&providerNetworkUUID)
		if err != nil {
			st.logger.Errorf("retrieving provider network corresponding to subnet %q, %v", uuid, err)
			return errors.Trace(domain.CoerceError(err))
		}

		delProviderNetworkSubnetResult, err := tx.ExecContext(ctx, deleteProviderNetworkSubnetStmt, uuid)
		if err != nil {
			st.logger.Errorf("removing the provider network entry for subnet %q, %v", uuid, err)
			return errors.Trace(domain.CoerceError(err))
		}
		if delProviderNetworkSubnetAffected, err := delProviderNetworkSubnetResult.RowsAffected(); err != nil {
			return errors.Trace(domain.CoerceError(err))
		} else if delProviderNetworkSubnetAffected != 1 {
			return fmt.Errorf("provider network subnets for subnet %s not found", uuid)
		}

		delProviderNetworkResult, err := tx.ExecContext(ctx, deleteProviderNetworkStmt, providerNetworkUUID)
		if err != nil {
			st.logger.Errorf("removing the provider network entry %q, %v", providerNetworkUUID, err)
			return errors.Trace(domain.CoerceError(err))
		}
		if delProviderNetworkAffected, err := delProviderNetworkResult.RowsAffected(); err != nil {
			return errors.Trace(domain.CoerceError(err))
		} else if delProviderNetworkAffected != 1 {
			return fmt.Errorf("provider network for subnet %s not found", uuid)
		}

		if _, err := tx.ExecContext(ctx, deleteAvailabilityZoneSubnetStmt, uuid); err != nil {
			st.logger.Errorf("removing the availability zone entry for subnet %q, %v", uuid, err)
			return errors.Trace(domain.CoerceError(err))
		}

		delProviderSubnetResult, err := tx.ExecContext(ctx, deleteProviderSubnetStmt, uuid)
		st.logger.Errorf("removing the provider subnet entry for subnet %q, %v", uuid, err)
		if err != nil {
			return errors.Trace(domain.CoerceError(err))
		}
		if delProviderSubnetAffected, err := delProviderSubnetResult.RowsAffected(); err != nil {
			return errors.Trace(domain.CoerceError(err))
		} else if delProviderSubnetAffected != 1 {
			return fmt.Errorf("provider subnet for subnet %s not found", uuid)
		}

		delSubnetResult, err := tx.ExecContext(ctx, deleteSubnetStmt, uuid)
		if err != nil {
			st.logger.Errorf("removing subnet %q, %v", uuid, err)
			return errors.Trace(domain.CoerceError(err))
		}
		if delSubnetAffected, err := delSubnetResult.RowsAffected(); err != nil {
			return errors.Trace(domain.CoerceError(err))
		} else if delSubnetAffected != 1 {
			return fmt.Errorf("subnet %s not found", uuid)
		}

		return nil
	})
}
