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

type subnetType int

const (
	base subnetType = iota
	fanOverlaySegment
)

// AddSubnet creates a subnet.
func (st *State) AddSubnet(
	ctx context.Context,
	uuid utils.UUID,
	cidr string,
	providerID network.Id,
	providerNetworkID network.Id,
	VLANTag int,
	availabilityZones []string,
	spaceUUID string,
	fanInfo *network.FanCIDRs,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	var subnetType subnetType
	if fanInfo != nil {
		subnetType = fanOverlaySegment
	}
	var spaceUUIDValue any
	spaceUUIDValue = spaceUUID
	if spaceUUID == "" {
		spaceUUIDValue = nil
	}

	insertSubnetStmt := `
INSERT INTO subnet (uuid, cidr, vlan_tag, space_uuid, subnet_type_id)
VALUES (?, ?, ?, ?, ?)`
	insertSubnetAssociationStmt := `
INSERT INTO subnet_association (subject_subnet_uuid, associated_subnet_uuid, association_type_id)
VALUES (?, ?, 0)` // For the moment the only allowed association is 'overlay_of' and therefore its ID is hard-coded here.
	retrieveUnderlaySubnetUUIDStmt := `
SELECT uuid
FROM   subnet
WHERE  cidr = ?`
	insertSubnetProviderIDStmt := `
INSERT INTO provider_subnet (provider_id, subnet_uuid)
VALUES (?, ?)`
	insertSubnetProviderNetworkIDStmt := `
INSERT INTO provider_network (uuid, provider_network_id)
VALUES (?, ?)`
	insertSubnetProviderNetworkSubnetStmt := `
INSERT INTO provider_network_subnet (provider_network_uuid, subnet_uuid)
VALUES (?, ?)`
	insertAvailabilityZoneStmt := `
INSERT INTO availability_zone (uuid, name)
VALUES (?, ?)`
	insertAvailabilityZoneSubnetStmt := `
INSERT INTO availability_zone_subnet (availability_zone_uuid, subnet_uuid)
VALUES (?, ?)`
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Add the subnet entity.
		if _, err := tx.ExecContext(
			ctx,
			insertSubnetStmt,
			uuid.String(),
			cidr,
			VLANTag,
			spaceUUIDValue,
			subnetType,
		); err != nil {
			return errors.Trace(err)
		}

		if subnetType == fanOverlaySegment {
			// Retrieve the underlay subnet uuid.
			var underlaySubnetUUID string
			row := tx.QueryRowContext(ctx, retrieveUnderlaySubnetUUIDStmt, fanInfo.FanLocalUnderlay)
			if err := row.Scan(&underlaySubnetUUID); err != nil {
				return errors.Trace(err)
			}
			// Add the association of the underlay and the newly
			// created subnet to the associations table.
			if _, err := tx.ExecContext(
				ctx,
				insertSubnetAssociationStmt,
				uuid.String(),
				underlaySubnetUUID,
			); err != nil {
				return errors.Trace(err)
			}
		}
		// Add the subnet uuid to the provider ids table.
		if _, err := tx.ExecContext(
			ctx,
			insertSubnetProviderIDStmt,
			providerID,
			uuid.String(),
		); err != nil {
			return errors.Trace(err)
		}
		// Add the subnet and provider network uuids to the
		// provider_network_subnet table.
		pnUUID, err := utils.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		if _, err := tx.ExecContext(
			ctx,
			insertSubnetProviderNetworkIDStmt,
			pnUUID.String(),
			providerNetworkID,
		); err != nil {
			return errors.Trace(err)
		}
		// Insert the providerNetworkUUID into provider network to
		// subnets mapping table.
		if _, err := tx.ExecContext(
			ctx,
			insertSubnetProviderNetworkSubnetStmt,
			pnUUID.String(),
			uuid.String(),
		); err != nil {
			return errors.Trace(err)
		}
		for _, az := range availabilityZones {
			// Add the availability zone.
			azUUID, err := utils.NewUUID()
			if err != nil {
				return errors.Trace(err)
			}
			if _, err := tx.ExecContext(
				ctx,
				insertAvailabilityZoneStmt,
				azUUID.String(),
				az,
			); err != nil {
				return errors.Trace(err)
			}
			// Add the subnet id along with the az uuid into the
			// availability_zone_subnet mapping table.
			if _, err := tx.ExecContext(
				ctx,
				insertAvailabilityZoneSubnetStmt,
				azUUID.String(),
				uuid.String(),
			); err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	})
	return errors.Trace(err)
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
    subnet.uuid                          AS &Subnet.uuid,
    subnet.cidr                          AS &Subnet.cidr,
    subnet.vlan_tag                      AS &Subnet.vlan_tag,
    subnet.space_uuid                    AS &Subnet.space_uuid,
    provider_subnet.provider_id          AS &Subnet.provider_id,
    provider_network.provider_network_id AS &Subnet.provider_network_id,
    fan_subnet.cidr                      AS &Subnet.underlay_cidr,
    availability_zone.name               AS &Subnet.az,
    provider_space.provider_id           AS &Subnet.provider_space_uuid
FROM subnet 
    LEFT JOIN fan_subnet 
    ON subnet.uuid = fan_subnet.subject_subnet_uuid
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

	s, err := sqlair.Prepare(q, Subnet{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var rows Subnets
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
	uuid utils.UUID,
) (*network.SubnetInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Append the space uuid condition to the query only if it's passed to the function.
	q := retrieveSubnetsStmt + " WHERE subnet.uuid = $M.id;"

	s, err := sqlair.Prepare(q, Subnet{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var rows Subnets
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(tx.Query(ctx, s, sqlair.M{"id": uuid.String()}).GetAll(&rows))
	}); err != nil {
		return nil, errors.Annotate(err, "querying subnets")
	}

	if len(rows) == 0 {
		return nil, errors.NotFoundf("subnet %q", uuid.String())
	}

	return &rows.ToSubnetInfos()[0], nil
}

// GetSubnetsByCIDR returns the subnets by CIDR.
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

	s, err := sqlair.Prepare(q, Subnet{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var resultSubnets Subnets
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, cidr := range cidrs {
			var rows Subnets
			if err := tx.Query(ctx, s, sqlair.M{"cidrs": cidr}).GetAll(&rows); err != nil {
				return errors.Trace(err)
			}
			resultSubnets = append(resultSubnets, rows...)
		}
		return nil
	}); err != nil {
		return nil, errors.Annotate(err, "querying subnets")
	}

	// Make sure that the combination CIDR + ProviderNetworkID is
	// unique among subnets.
	uniqueSubnets := make(map[string]map[string]any)
	for _, subnet := range resultSubnets {
		uniqueSubnetsByCIDR, ok := uniqueSubnets[subnet.CIDR]
		if !ok {
			uniqueSubnets[subnet.CIDR] = make(map[string]any)
		}
		_, ok = uniqueSubnetsByCIDR[subnet.ProviderNetworkID]
		// Fail if duplicated subnets are returned.
		if ok {
			return nil, errors.Errorf("multiple subnets matching cidr %q and providerNetworkID %q", subnet.CIDR, subnet.ProviderNetworkID)
		}
		uniqueSubnetsByCIDR[subnet.ProviderNetworkID] = struct{}{}
	}

	return resultSubnets.ToSubnetInfos(), nil
}

func updateSubnetSpaceIDTx(
	ctx context.Context,
	tx *sql.Tx,
	uuid string,
	spaceID string,
) error {
	q := `
UPDATE subnet
SET    space_uuid = ?
WHERE  uuid = ?;`

	rows, err := tx.ExecContext(ctx, q, spaceID, uuid)
	if err != nil {
		return errors.Trace(err)
	}
	affected, err := rows.RowsAffected()
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

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return updateSubnetSpaceIDTx(ctx, tx, uuid, spaceID)
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
		delProviderNetworkSubnetAffected, err := delProviderNetworkSubnetResult.RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if delProviderNetworkSubnetAffected != 1 {
			return fmt.Errorf("provider network subnets for subnet %s not found", uuid)
		}

		delProviderNetworkResult, err := tx.ExecContext(ctx, deleteProviderNetworkStmt, providerNetworkUUID)
		if err != nil {
			return errors.Trace(err)
		}
		delProviderNetworkAffected, err := delProviderNetworkResult.RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if delProviderNetworkAffected != 1 {
			return fmt.Errorf("provider network for subnet %s not found", uuid)
		}

		if _, err := tx.ExecContext(ctx, deleteAvailabilityZoneSubnetStmt, uuid); err != nil {
			return errors.Trace(err)
		}

		delProviderSubnetResult, err := tx.ExecContext(ctx, deleteProviderSubnetStmt, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		delProviderSubnetAffected, err := delProviderSubnetResult.RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if delProviderSubnetAffected != 1 {
			return fmt.Errorf("provider subnet for subnet %s not found", uuid)
		}

		delSubnetResult, err := tx.ExecContext(ctx, deleteSubnetStmt, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		delSubnetAffected, err := delSubnetResult.RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if delSubnetAffected != 1 {
			return fmt.Errorf("subnet %s not found", uuid)
		}

		return nil
	})
}
