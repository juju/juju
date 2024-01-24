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

	"github.com/juju/juju/core/network"
)

const (
	subnetTypeBase              = 0
	subnetTypeFanOverlaySegment = 1
)

// UpsertSubnets updates or adds each one of the provided subnets in one
// transaction.
func (st *State) UpsertSubnets(ctx context.Context, subnets []network.SubnetInfo) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, subnet := range subnets {
			err := updateSubnetSpaceID(
				ctx,
				tx,
				string(subnet.ID),
				subnet.SpaceID,
			)
			if err != nil && !errors.Is(err, errors.NotFound) {
				return errors.Trace(err)
			}
			// If the subnet doesn't exist yet we need to create it.
			if errors.Is(err, errors.NotFound) {
				if err := addSubnet(
					ctx,
					tx,
					subnet.ID.String(),
					subnet,
				); err != nil {
					return errors.Trace(err)
				}
			}
		}
		return nil
	})
}

func addSubnet(ctx context.Context, tx *sqlair.TX, uuid string, subnetInfo network.SubnetInfo) error {
	var subnetType int
	if subnetInfo.FanInfo != nil {
		subnetType = subnetTypeFanOverlaySegment
	}
	spaceUUIDValue := subnetInfo.SpaceID
	if subnetInfo.SpaceID == "" {
		spaceUUIDValue = network.AlphaSpaceId
	}
	pnUUID, err := utils.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	insertSubnetStmt, err := sqlair.Prepare(`
INSERT INTO subnet (uuid, cidr, vlan_tag, space_uuid, subnet_type_id)
VALUES ($Subnet.uuid, $Subnet.cidr, $Subnet.vlan_tag, $Subnet.space_uuid, $Subnet.subnet_type_id)`, Subnet{})
	if err != nil {
		return errors.Trace(err)
	}
	insertSubnetAssociationStmt, err := sqlair.Prepare(`
INSERT INTO subnet_association (subject_subnet_uuid, associated_subnet_uuid, association_type_id)
VALUES ($M.subject_subnet_uuid, $M.associated_subnet_uuid, 0)`, sqlair.M{}) // For the moment the only allowed association is 'overlay_of' and therefore its ID is hard-coded here.
	if err != nil {
		return errors.Trace(err)
	}
	retrieveUnderlaySubnetUUIDStmt, err := sqlair.Prepare(`
SELECT &Subnet.uuid
FROM   subnet
WHERE  cidr = $Subnet.cidr`, Subnet{})
	if err != nil {
		return errors.Trace(err)
	}
	insertSubnetProviderIDStmt, err := sqlair.Prepare(`
INSERT INTO provider_subnet (provider_id, subnet_uuid)
VALUES ($ProviderSubnet.provider_id, $ProviderSubnet.subnet_uuid)`, ProviderSubnet{})
	if err != nil {
		return errors.Trace(err)
	}
	insertSubnetProviderNetworkIDStmt, err := sqlair.Prepare(`
INSERT INTO provider_network (uuid, provider_network_id)
VALUES ($ProviderNetwork.uuid, $ProviderNetwork.provider_network_id)`, ProviderNetwork{})
	if err != nil {
		return errors.Trace(err)
	}
	insertSubnetProviderNetworkSubnetStmt, err := sqlair.Prepare(`
INSERT INTO provider_network_subnet (provider_network_uuid, subnet_uuid)
VALUES ($ProviderNetworkSubnet.provider_network_uuid, $ProviderNetworkSubnet.subnet_uuid)`, ProviderNetworkSubnet{})
	if err != nil {
		return errors.Trace(err)
	}
	// Add the subnet entity.
	if err := tx.Query(
		ctx,
		insertSubnetStmt,
		Subnet{
			UUID:       uuid,
			CIDR:       subnetInfo.CIDR,
			VLANtag:    subnetInfo.VLANTag,
			SpaceUUID:  spaceUUIDValue,
			SubnetType: subnetType,
		},
	).Run(); err != nil {
		return errors.Trace(err)
	}

	if subnetType == subnetTypeFanOverlaySegment {
		// Retrieve the underlay subnet uuid.
		var underlaySubnet Subnet

		if err := tx.Query(ctx, retrieveUnderlaySubnetUUIDStmt, Subnet{CIDR: subnetInfo.FanInfo.FanLocalUnderlay}).Get(&underlaySubnet); err != nil {
			return errors.Annotatef(err, "retrieving underlay subnet %q for subnet %q", subnetInfo.FanInfo.FanLocalUnderlay, uuid)
		}
		// Add the association of the underlay and the newly
		// created subnet to the associations table.
		if err := tx.Query(
			ctx,
			insertSubnetAssociationStmt,
			sqlair.M{"subject_subnet_uuid": uuid, "associated_subnet_uuid": underlaySubnet.UUID},
		).Run(); err != nil {
			return errors.Annotatef(err, "inserting subnet association between underlay %q and subnet %q", subnetInfo.FanInfo.FanLocalUnderlay, uuid)
		}
	}
	// Add the subnet uuid to the provider ids table.
	if err := tx.Query(
		ctx,
		insertSubnetProviderIDStmt,
		ProviderSubnet{SubnetUUID: uuid, ProviderNetworkId: subnetInfo.ProviderId},
	).Run(); err != nil {
		return errors.Annotatef(err, "inserting provider id %q for subnet %q", subnetInfo.ProviderId, uuid)
	}
	// Add the provider network id and its uuid to the
	// provider_network table.
	if err := tx.Query(
		ctx,
		insertSubnetProviderNetworkIDStmt,
		ProviderNetwork{ProviderNetworkUUID: pnUUID.String(), ProviderNetworkId: subnetInfo.ProviderNetworkId},
	).Run(); err != nil {
		return errors.Annotatef(err, "inserting provider network id %q for subnet %q", subnetInfo.ProviderNetworkId, uuid)
	}
	// Insert the providerNetworkUUID into provider network to
	// subnets mapping table.
	if err := tx.Query(
		ctx,
		insertSubnetProviderNetworkSubnetStmt,
		ProviderNetworkSubnet{SubnetUUID: uuid, ProviderNetworkUUID: pnUUID.String()},
	).Run(); err != nil {
		return errors.Annotatef(err, "inserting association between provider network id %q and subnet %q", subnetInfo.ProviderNetworkId, uuid)
	}

	return addAvailabilityZones(ctx, tx, uuid, subnetInfo)
}

// addAvailabilityZones adds the availability zones of a subnet if they don't exist, and
// update the availability_zone_subnet table with the subnet's id.
func addAvailabilityZones(ctx context.Context, tx *sqlair.TX, subnetUUID string, subnet network.SubnetInfo) error {
	retrieveAvailabilityZoneStmt, err := sqlair.Prepare(`
SELECT &M.uuid
FROM   availability_zone
WHERE  name = $M.name`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	insertAvailabilityZoneStmt, err := sqlair.Prepare(`
INSERT INTO availability_zone (uuid, name)
VALUES ($M.uuid, $M.name)`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	insertAvailabilityZoneSubnetStmt, err := sqlair.Prepare(`
INSERT INTO availability_zone_subnet (availability_zone_uuid, subnet_uuid)
VALUES ($M.availability_zone_uuid, $M.subnet_uuid)`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	for _, az := range subnet.AvailabilityZones {
		// Retrieve the availability zone.
		m := sqlair.M{}

		if err := tx.Query(ctx, retrieveAvailabilityZoneStmt, sqlair.M{"name": az}).Get(m); err != nil && err != sqlair.ErrNoRows {
			return errors.Annotatef(err, "retrieving availability zone %q for subnet %q", az, subnetUUID)
		}
		azUUIDStr, _ := m["uuid"]

		// If it doesn't exist, add the availability zone.
		if errors.Is(err, sql.ErrNoRows) {
			azUUID, err := utils.NewUUID()
			if err != nil {
				return errors.Annotatef(err, "generating UUID for availability zone %q for subnet %q", az, subnetUUID)
			}
			azUUIDStr = azUUID.String()
			if err := tx.Query(
				ctx,
				insertAvailabilityZoneStmt,
				sqlair.M{"uuid": azUUIDStr, "name": az},
			).Run(); err != nil {
				return errors.Annotatef(err, "inserting availability zone %q for subnet %q", az, subnetUUID)
			}
		}
		// Add the subnet id along with the az uuid into the
		// availability_zone_subnet mapping table.
		if err := tx.Query(
			ctx,
			insertAvailabilityZoneSubnetStmt,
			sqlair.M{"availability_zone_uuid": azUUIDStr, "subnet_uuid": subnetUUID},
		).Run(); err != nil {
			return errors.Annotatef(err, "inserting availability zone %q association with subnet %q", az, subnetUUID)
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
		return errors.Trace(err)
	}

	return errors.Trace(
		db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			return addSubnet(ctx, tx, subnet.ID.String(), subnet)
		}),
	)
}

const retrieveSubnetsStmt = `
WITH fan_subnet AS (
    SELECT * 
    FROM subnet  
        JOIN subnet_association  
           ON subnet.uuid = subnet_association.associated_subnet_uuid
           AND subnet_association.association_type_id = 0
        WHERE subnet.subnet_type_id = 0
    )
SELECT     
    subnet.uuid                          AS &SpaceSubnetRow.subnet_uuid,
    subnet.cidr                          AS &SpaceSubnetRow.subnet_cidr,
    subnet.vlan_tag                      AS &SpaceSubnetRow.subnet_vlan_tag,
    subnet.space_uuid                    AS &SpaceSubnetRow.subnet_space_uuid,
    space.name                           AS &SpaceSubnetRow.subnet_space_name,
    provider_subnet.provider_id          AS &SpaceSubnetRow.subnet_provider_id,
    provider_network.provider_network_id AS &SpaceSubnetRow.subnet_provider_network_id,
    fan_subnet.cidr                      AS &SpaceSubnetRow.subnet_underlay_cidr,
    availability_zone.name               AS &SpaceSubnetRow.subnet_az,
    provider_space.provider_id           AS &SpaceSubnetRow.subnet_provider_space_uuid
FROM subnet 
    LEFT JOIN fan_subnet 
    ON subnet.uuid = fan_subnet.subject_subnet_uuid
    LEFT JOIN space
    ON subnet.space_uuid = space.uuid
    JOIN provider_subnet
    ON subnet.uuid = provider_subnet.subnet_uuid
    JOIN provider_network_subnet
    ON subnet.uuid = provider_network_subnet.subnet_uuid
    JOIN provider_network
    ON provider_network_subnet.provider_network_uuid = provider_network.uuid
    LEFT JOIN availability_zone_subnet
    ON availability_zone_subnet.subnet_uuid = subnet.uuid
    LEFT JOIN availability_zone
    ON availability_zone_subnet.availability_zone_uuid = availability_zone.uuid
    LEFT JOIN provider_space
    ON subnet.space_uuid = provider_space.space_uuid`

// GetAllSubnets returns all known subnets in the model.
func (st *State) GetAllSubnets(
	ctx context.Context,
) (network.SubnetInfos, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Append the space uuid condition to the query only if it's passed to the function.
	q := retrieveSubnetsStmt + ";"

	s, err := sqlair.Prepare(q, SpaceSubnetRow{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var rows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s).GetAll(&rows))
	}); err != nil {
		return nil, errors.Annotate(err, "querying subnets")
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
		return nil, errors.Trace(err)
	}

	// Append the space uuid condition to the query only if it's passed to the function.
	q := retrieveSubnetsStmt + " WHERE subnet.uuid = $M.id;"

	stmt, err := sqlair.Prepare(q, SpaceSubnetRow{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var rows SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, stmt, sqlair.M{"id": uuid}).GetAll(&rows))
	}); err != nil {
		return nil, errors.Annotate(err, "querying subnets")
	}

	if len(rows) == 0 {
		return nil, errors.NotFoundf("subnet %q", uuid)
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
		return nil, errors.Trace(err)
	}

	// Append the where clause to the query.
	q := retrieveSubnetsStmt + " WHERE subnet.cidr = $M.cidr;"

	s, err := sqlair.Prepare(q, SpaceSubnetRow{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var resultSubnets SpaceSubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, cidr := range cidrs {
			var rows SpaceSubnetRows
			if err := tx.Query(ctx, s, sqlair.M{"cidr": cidr}).GetAll(&rows); err != nil {
				return errors.Trace(err)
			}
			resultSubnets = append(resultSubnets, rows...)
		}
		return nil
	}); err != nil {
		return nil, errors.Annotate(err, "querying subnets")
	}

	return resultSubnets.ToSubnetInfos(), nil
}

func updateSubnetSpaceID(
	ctx context.Context,
	tx *sqlair.TX,
	uuid string,
	spaceID string,
) error {
	updateSubnetSpaceIDStmt, err := sqlair.Prepare(`
UPDATE subnet
SET    space_uuid = $M.space_uuid
WHERE  uuid = $M.uuid;`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	var outcome sqlair.Outcome

	if err = tx.Query(ctx, updateSubnetSpaceIDStmt, sqlair.M{"space_uuid": spaceID, "uuid": uuid}).Get(&outcome); err != nil {
		return errors.Trace(err)
	}
	affected, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Trace(err)
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
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return updateSubnetSpaceID(ctx, tx, uuid, spaceID)
	})
}

// DeleteSubnet deletes the subnet identified by the passed uuid.
func (st *State) DeleteSubnet(
	ctx context.Context,
	uuid string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	deleteSubnetStmt := "DELETE FROM subnet WHERE uuid = ?;"
	deleteSubjectSubnetAssociationStmt := "DELETE FROM subnet_association WHERE subject_subnet_uuid = ?;"
	deleteAssociatedSubnetAssociationStmt := "DELETE FROM subnet_association WHERE associated_subnet_uuid = ?;"
	selectProviderNetworkStmt := "SELECT provider_network_uuid FROM provider_network_subnet WHERE subnet_uuid = ?;"
	deleteProviderNetworkStmt := "DELETE FROM provider_network WHERE uuid = ?;"
	deleteProviderNetworkSubnetStmt := "DELETE FROM provider_network_subnet WHERE subnet_uuid = ?;"
	deleteProviderSubnetStmt := "DELETE FROM provider_subnet WHERE subnet_uuid = ?;"
	deleteAvailabilityZoneSubnetStmt := "DELETE FROM availability_zone_subnet WHERE subnet_uuid = ?;"

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, deleteSubjectSubnetAssociationStmt, uuid); err != nil {
			return errors.Trace(err)
		}
		if _, err := tx.ExecContext(ctx, deleteAssociatedSubnetAssociationStmt, uuid); err != nil {
			return errors.Trace(err)
		}

		row := tx.QueryRowContext(ctx, selectProviderNetworkStmt, uuid)
		var providerNetworkUUID string
		err = row.Scan(&providerNetworkUUID)
		if err != nil {
			return errors.Trace(err)
		}

		delProviderNetworkSubnetResult, err := tx.ExecContext(ctx, deleteProviderNetworkSubnetStmt, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		if delProviderNetworkSubnetAffected, err := delProviderNetworkSubnetResult.RowsAffected(); err != nil {
			return errors.Trace(err)
		} else if delProviderNetworkSubnetAffected != 1 {
			return fmt.Errorf("provider network subnets for subnet %s not found", uuid)
		}

		delProviderNetworkResult, err := tx.ExecContext(ctx, deleteProviderNetworkStmt, providerNetworkUUID)
		if err != nil {
			return errors.Trace(err)
		}
		if delProviderNetworkAffected, err := delProviderNetworkResult.RowsAffected(); err != nil {
			return errors.Trace(err)
		} else if delProviderNetworkAffected != 1 {
			return fmt.Errorf("provider network for subnet %s not found", uuid)
		}

		if _, err := tx.ExecContext(ctx, deleteAvailabilityZoneSubnetStmt, uuid); err != nil {
			return errors.Trace(err)
		}

		delProviderSubnetResult, err := tx.ExecContext(ctx, deleteProviderSubnetStmt, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		if delProviderSubnetAffected, err := delProviderSubnetResult.RowsAffected(); err != nil {
			return errors.Trace(err)
		} else if delProviderSubnetAffected != 1 {
			return fmt.Errorf("provider subnet for subnet %s not found", uuid)
		}

		delSubnetResult, err := tx.ExecContext(ctx, deleteSubnetStmt, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		if delSubnetAffected, err := delSubnetResult.RowsAffected(); err != nil {
			return errors.Trace(err)
		} else if delSubnetAffected != 1 {
			return fmt.Errorf("subnet %s not found", uuid)
		}

		return nil
	})
}
