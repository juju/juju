// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/google/uuid"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// AllSubnetsQuery returns the SQL query that finds all subnet UUIDs from the
// subnet table, needed for the subnets' watcher.
func (st *State) AllSubnetsQuery(ctx context.Context, db database.TxnRunner) ([]string, error) {
	var subnets []Subnet
	stmt, err := st.Prepare(`
SELECT &Subnet.uuid
FROM   subnet`, Subnet{})
	if err != nil {
		return nil, errors.Errorf("preparing select subnet statement %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt).GetAll(&subnets)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.Slice(subnets, func(s Subnet) string { return s.UUID }), nil
}

// UpsertSubnets updates or adds each one of the provided subnets in one
// transaction.
func (st *State) UpsertSubnets(ctx context.Context, subnets []network.SubnetInfo) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, subnet := range subnets {
			err := st.updateSubnetSpaceID(
				ctx,
				tx,
				Subnet{
					UUID:      string(subnet.ID),
					SpaceUUID: subnet.SpaceID,
				},
			)
			if err != nil && !errors.Is(err, coreerrors.NotFound) {
				return errors.Capture(err)
			}
			// If the subnet does not yet exist then we need to create it.
			if errors.Is(err, coreerrors.NotFound) {
				if err := st.addSubnet(
					ctx,
					tx,
					subnet,
				); err != nil {
					return errors.Capture(err)
				}
			}
		}
		return nil
	})
}

func (st *State) addSubnet(ctx context.Context, tx *sqlair.TX, subnetInfo network.SubnetInfo) error {
	spaceUUIDValue := subnetInfo.SpaceID
	if subnetInfo.SpaceID == "" {
		spaceUUIDValue = network.AlphaSpaceId
	}
	subnetUUID := subnetInfo.ID.String()

	subnet := Subnet{
		UUID:      subnetUUID,
		CIDR:      subnetInfo.CIDR,
		VLANtag:   subnetInfo.VLANTag,
		SpaceUUID: spaceUUIDValue,
	}
	providerSubnet := ProviderSubnet{
		SubnetUUID: subnetUUID,
		ProviderID: subnetInfo.ProviderId,
	}
	providerNetwork := ProviderNetwork{
		ProviderNetworkID: subnetInfo.ProviderNetworkId,
	}
	providerNetworkSubnet := ProviderNetworkSubnet{
		SubnetUUID: subnetUUID,
	}

	insertSubnetStmt, err := st.Prepare(`
INSERT INTO subnet (*)
VALUES ($Subnet.*)`, subnet)
	if err != nil {
		return errors.Capture(err)
	}
	insertSubnetProviderIDStmt, err := st.Prepare(`
INSERT INTO provider_subnet (*)
VALUES ($ProviderSubnet.*)`, providerSubnet)
	if err != nil {
		return errors.Capture(err)
	}
	retrieveProviderNetworkUUIDStmt, err := st.Prepare(`
SELECT uuid AS &ProviderNetworkSubnet.provider_network_uuid
FROM   provider_network
WHERE  provider_network_id = $ProviderNetwork.provider_network_id`, providerNetwork, providerNetworkSubnet)
	if err != nil {
		return errors.Capture(err)
	}
	insertSubnetProviderNetworkIDStmt, err := st.Prepare(`
INSERT INTO provider_network (*)
VALUES ($ProviderNetwork.*)`, providerNetwork)
	if err != nil {
		return errors.Capture(err)
	}
	insertSubnetProviderNetworkSubnetStmt, err := st.Prepare(`
INSERT INTO provider_network_subnet (*)
VALUES ($ProviderNetworkSubnet.*)`, providerNetworkSubnet)
	if err != nil {
		return errors.Capture(err)
	}
	// Add the subnet entity.
	if err := tx.Query(ctx, insertSubnetStmt, subnet).Run(); err != nil {
		st.logger.Errorf("inserting subnet %q, %v", subnetInfo.CIDR, err)
		return errors.Capture(err)
	}

	// Add the subnet uuid to the provider ids table.
	if err := tx.Query(ctx, insertSubnetProviderIDStmt, providerSubnet).Run(); err != nil {
		if internaldatabase.IsErrConstraintPrimaryKey(err) || internaldatabase.IsErrConstraintUnique(err) {
			st.logger.Debugf("inserting provider id %q for subnet %q, %v", subnetInfo.ProviderId, subnetUUID, err)
			return errors.Errorf("provider id %q for subnet %q %w", subnetInfo.ProviderId, subnetUUID, coreerrors.AlreadyExists)
		}
		st.logger.Errorf("inserting provider id %q for subnet %q, %v", subnetInfo.ProviderId, subnetUUID, err)
		return errors.Errorf("inserting provider id %q for subnet %q %w", subnetInfo.ProviderId, subnetUUID, err)
	}

	var pnUUIDStr string
	err = tx.Query(ctx, retrieveProviderNetworkUUIDStmt, providerNetwork).Get(&providerNetworkSubnet)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		st.logger.Errorf("retrieving provider network ID %q for subnet %q, %v", subnetInfo.ProviderNetworkId, subnetUUID, err)
		return errors.Errorf("retrieving provider network ID %q for subnet %q %w", subnetInfo.ProviderNetworkId, subnetUUID, err)
	} else if errors.Is(err, sqlair.ErrNoRows) {
		// If the provider network doesn't exist, insert it.
		pnUUID, err := uuid.NewV7()
		if err != nil {
			return errors.Capture(err)
		}

		// Record the new UUID in provider network and the provider network
		// subnet.
		pnUUIDStr := pnUUID.String()
		providerNetwork.ProviderNetworkUUID = pnUUIDStr
		providerNetworkSubnet.ProviderNetworkUUID = pnUUIDStr
		// Add the provider network id and its uuid to the
		// provider_network table.
		if err := tx.Query(ctx, insertSubnetProviderNetworkIDStmt, providerNetwork).Run(); err != nil {
			st.logger.Errorf("inserting provider network id %q for subnet %q, %v", subnetInfo.ProviderNetworkId, subnetUUID, err)
			return errors.Errorf("inserting provider network id %q for subnet %q %w", subnetInfo.ProviderNetworkId, subnetUUID, err)
		}
	}

	// Insert the providerNetworkUUID into provider network to
	// subnets mapping table.
	if err := tx.Query(ctx, insertSubnetProviderNetworkSubnetStmt, providerNetworkSubnet).Run(); err != nil {
		st.logger.Errorf("inserting association between provider network id %q and subnet %q, %v", subnetInfo.ProviderNetworkId, subnetUUID, err)
		return errors.Errorf("inserting association between provider network id (%q) %q and subnet %q %w", pnUUIDStr, subnetInfo.ProviderNetworkId, subnetUUID, err)
	}

	return st.addAvailabilityZones(ctx, tx, subnetUUID, subnetInfo)
}

// addAvailabilityZones adds the availability zones of a subnet if they don't exist, and
// update the availability_zone_subnet table with the subnets' id.
func (st *State) addAvailabilityZones(ctx context.Context, tx *sqlair.TX, subnetUUID string, subnet network.SubnetInfo) error {
	availabilityZone := AvailabilityZone{}
	availabilityZoneSubnet := AvailabilityZoneSubnet{
		SubnetUUID: subnetUUID,
	}
	retrieveAvailabilityZoneStmt, err := st.Prepare(`
SELECT &AvailabilityZone.uuid
FROM   availability_zone
WHERE  name = $AvailabilityZone.name`, availabilityZone)
	if err != nil {
		return errors.Capture(err)
	}
	insertAvailabilityZoneStmt, err := st.Prepare(`
INSERT INTO availability_zone (*)
VALUES ($AvailabilityZone.*)`, availabilityZone)
	if err != nil {
		return errors.Capture(err)
	}
	insertAvailabilityZoneSubnetStmt, err := st.Prepare(`
INSERT INTO availability_zone_subnet (*)
VALUES ($AvailabilityZoneSubnet.*)`, availabilityZoneSubnet)
	if err != nil {
		return errors.Capture(err)
	}

	for _, az := range subnet.AvailabilityZones {
		availabilityZone.Name = az
		availabilityZone.UUID = ""
		// Retrieve the availability zone.
		err := tx.Query(ctx, retrieveAvailabilityZoneStmt, availabilityZone).Get(&availabilityZone)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			st.logger.Errorf("retrieving availability zone %q for subnet %q, %v", az, subnetUUID, err)
			return errors.Errorf("retrieving availability zone %q for subnet %q %w", az, subnetUUID, err)
		}

		// If it doesn't exist, add the availability zone.
		if errors.Is(err, sqlair.ErrNoRows) {
			azUUID, err := uuid.NewV7()
			if err != nil {
				return errors.Errorf("generating UUID for availability zone %q for subnet %q %w", az, subnetUUID, err)
			}
			availabilityZone.UUID = azUUID.String()
			if err := tx.Query(ctx, insertAvailabilityZoneStmt, availabilityZone).Run(); err != nil {
				st.logger.Errorf("inserting availability zone %q for subnet %q, %v", az, subnetUUID, err)
				return errors.Errorf("inserting availability zone %q for subnet %q %w", az, subnetUUID, err)
			}
		}
		availabilityZoneSubnet.AZUUID = availabilityZone.UUID
		// Add the subnet id along with the availability zone uuid into the
		// availability_zone_subnet mapping table.
		if err := tx.Query(ctx, insertAvailabilityZoneSubnetStmt, availabilityZoneSubnet).Run(); err != nil {
			st.logger.Errorf("inserting availability zone %q association with subnet %q, %v", az, subnetUUID, err)
			return errors.Errorf("inserting availability zone %q association with subnet %q %w", az, subnetUUID, err)
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
		return errors.Capture(err)
	}

	return errors.Capture(
		db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			return st.addSubnet(ctx, tx, subnet)
		}))

}

// GetAllSubnets returns all known subnets in the model.
func (st *State) GetAllSubnets(
	ctx context.Context,
) (network.SubnetInfos, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Append the space uuid condition to the query only if it's passed to the function.
	q := `
SELECT &SubnetRow.*
FROM   v_space_subnet
`

	s, err := st.Prepare(q, SubnetRow{})
	if err != nil {
		return nil, errors.Errorf("preparing %q %w", q, err)
	}

	var rows SubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Capture(tx.Query(ctx, s).GetAll(&rows))
	}); errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		st.logger.Errorf("querying subnets, %v", err)
		return nil, errors.Errorf("querying subnets %w", err)
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
		return nil, errors.Capture(err)
	}

	// Append the space uuid condition to the query only if it's passed to the function.
	q := `
SELECT &SubnetRow.*
FROM   v_space_subnet
WHERE  subnet_uuid = $M.id;`

	stmt, err := st.Prepare(q, SubnetRow{}, sqlair.M{})
	if err != nil {
		return nil, errors.Errorf("preparing %q %w", q, err)
	}

	var rows SubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, sqlair.M{"id": uuid}).GetAll(&rows)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return networkerrors.SubnetNotFound
			}
			return errors.Errorf("retrieving subnet %q %w", uuid, err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
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
		return nil, errors.Capture(err)
	}

	// Append the where clause to the query.
	q := `
SELECT &SubnetRow.*
FROM   v_space_subnet
WHERE  subnet_cidr = $M.cidr`

	s, err := st.Prepare(q, SubnetRow{}, sqlair.M{})
	if err != nil {
		return nil, errors.Errorf("preparing %q %w", q, err)
	}

	var resultSubnets SubnetRows
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, cidr := range cidrs {
			var rows SubnetRows
			if err := tx.Query(ctx, s, sqlair.M{"cidr": cidr}).GetAll(&rows); err != nil {
				if errors.Is(err, sqlair.ErrNoRows) {
					continue
				}
				return errors.Errorf("retrieving subnets by CIDR %v %w", cidr, err)
			}
			resultSubnets = append(resultSubnets, rows...)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return resultSubnets.ToSubnetInfos(), nil
}

// updateSubnetSpaceID updates the space id of the subnet in the subnet table.
// The subnet passed as an argument should have the UUID and SpaceUUID set to the
// desired values.
func (st *State) updateSubnetSpaceID(
	ctx context.Context,
	tx *sqlair.TX,
	subnet Subnet,
) error {
	updateSubnetSpaceIDStmt, err := st.Prepare(`
UPDATE subnet
SET    space_uuid = $Subnet.space_uuid
WHERE  uuid = $Subnet.uuid;`, subnet)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome

	if err = tx.Query(ctx, updateSubnetSpaceIDStmt, subnet).Get(&outcome); err != nil {
		st.logger.Errorf("updating subnet %q space ID %v, %v", subnet.UUID, subnet.SpaceUUID, err)
		return errors.Capture(err)
	}
	affected, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Capture(err)
	}
	if affected != 1 {
		return errors.Errorf("subnet %q %w", subnet.UUID, coreerrors.NotFound)
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
		return errors.Capture(err)
	}
	subnet := Subnet{SpaceUUID: spaceID, UUID: uuid}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.updateSubnetSpaceID(ctx, tx, subnet)
	})
}

// DeleteSubnet deletes the subnet identified by the passed uuid.
func (st *State) DeleteSubnet(
	ctx context.Context,
	uuid string,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	subnet := Subnet{UUID: uuid}
	providerNetworkSubnet := ProviderNetworkSubnet{}

	deleteSubnetStmt, err := st.Prepare(`
DELETE FROM subnet WHERE uuid = $Subnet.uuid;`, subnet)
	if err != nil {
		return errors.Errorf("preparing delete subnet statement %w", err)
	}
	selectProviderNetworkStmt, err := st.Prepare(`
SELECT &ProviderNetworkSubnet.provider_network_uuid
FROM   provider_network_subnet
WHERE  subnet_uuid = $Subnet.uuid;`, subnet, providerNetworkSubnet)
	if err != nil {
		return errors.Errorf("preparing select provider network statement %w", err)
	}
	deleteProviderNetworkStmt, err := st.Prepare(`
DELETE FROM provider_network WHERE uuid = $ProviderNetworkSubnet.provider_network_uuid;`, providerNetworkSubnet)
	if err != nil {
		return errors.Errorf("preparing delete provider network statement %w", err)
	}
	deleteProviderNetworkSubnetStmt, err := st.Prepare(`
DELETE FROM provider_network_subnet WHERE subnet_uuid = $Subnet.uuid;`, subnet)
	if err != nil {
		return errors.Errorf("preparing delete provider network subnet statement %w", err)
	}
	deleteProviderSubnetStmt, err := st.Prepare(`
DELETE FROM provider_subnet WHERE subnet_uuid = $Subnet.uuid;`, subnet)
	if err != nil {
		return errors.Errorf("preparing delete provider subnet statement %w", err)
	}
	deleteAvailabilityZoneSubnetStmt, err := st.Prepare(`
DELETE FROM availability_zone_subnet WHERE subnet_uuid = $Subnet.uuid;`, subnet)
	if err != nil {
		return errors.Errorf("preparing delete availability zone subnet statement %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, selectProviderNetworkStmt, subnet).Get(&providerNetworkSubnet)
		if err != nil {
			st.logger.Errorf("retrieving provider network corresponding to subnet %q, %v", uuid, err)
			return errors.Capture(err)
		}

		var outcome sqlair.Outcome
		err = tx.Query(ctx, deleteProviderNetworkSubnetStmt, subnet).Get(&outcome)
		if err != nil {
			st.logger.Errorf("removing the provider network entry for subnet %q, %v", uuid, err)
			return errors.Capture(err)
		}
		if delProviderNetworkSubnetAffected, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Capture(err)
		} else if delProviderNetworkSubnetAffected != 1 {
			return errors.Errorf("provider network subnets for subnet %s not found", uuid)
		}

		err = tx.Query(ctx, deleteProviderNetworkStmt, providerNetworkSubnet).Get(&outcome)
		if err != nil {
			st.logger.Errorf("removing the provider network entry %q, %v", providerNetworkSubnet.ProviderNetworkUUID, err)
			return errors.Capture(err)
		}
		if delProviderNetworkAffected, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Capture(err)
		} else if delProviderNetworkAffected != 1 {
			return errors.Errorf("provider network for subnet %s not found", uuid)
		}

		if err := tx.Query(ctx, deleteAvailabilityZoneSubnetStmt, subnet).Run(); err != nil {
			st.logger.Errorf("removing the availability zone entry for subnet %q, %v", uuid, err)
			return errors.Capture(err)
		}

		err = tx.Query(ctx, deleteProviderSubnetStmt, subnet).Get(&outcome)
		st.logger.Errorf("removing the provider subnet entry for subnet %q, %v", uuid, err)
		if err != nil {
			return errors.Capture(err)
		}
		if delProviderSubnetAffected, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Capture(err)
		} else if delProviderSubnetAffected != 1 {
			return errors.Errorf("provider subnet for subnet %s not found", uuid)
		}

		err = tx.Query(ctx, deleteSubnetStmt, subnet).Get(&outcome)
		if err != nil {
			st.logger.Errorf("removing subnet %q, %v", uuid, err)
			return errors.Capture(err)
		}
		if delSubnetAffected, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Capture(err)
		} else if delSubnetAffected != 1 {
			return errors.Errorf("subnet %s not found", uuid)
		}

		return nil
	})
}
