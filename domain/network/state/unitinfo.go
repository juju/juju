// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/network"
	networkinternal "github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
)

// GetUnitEndpointNetworkAddresses retrieves raw unit addresses for the
// specified endpoints.
func (st *State) GetUnitEndpointNetworkAddresses(ctx context.Context, unitUUID string,
	endpointNames []string) ([]networkinternal.EndpointAddresses, error) {

	// Determine the set of unique spaces to which the endpoints are bound.
	spaces, err := st.getAllSpacesForEndpoints(ctx, unitUUID, endpointNames)
	if err != nil {
		return nil, errors.Capture(err)
	}
	uniqueSpaces := set.NewStrings()
	for _, space := range spaces {
		uniqueSpaces.Add(space.SpaceUUID)
	}

	// Then get the unit's addresses in those spaces.
	addresses, err := st.getAllUnitAddressesInSpaces(ctx, unitUUID, uniqueSpaces.Values())
	if err != nil {
		return nil, errors.Errorf("getting all unit addresses in spaces: %w", err)
	}

	addressesBySpace, _ := accumulateToMap(addresses, func(address networkinternal.UnitAddress) (string, networkinternal.UnitAddress, error) {
		return address.SpaceID.String(), address, nil
	})

	// Transform in order to dispatch addresses by endpoint.
	spaceEp := transform.SliceToMap(spaces, func(endpoint spaceEndpoint) (string, string) {
		return endpoint.EndpointName, endpoint.SpaceUUID
	})
	infos, _ := transform.SliceOrErr(endpointNames, func(name string) (networkinternal.EndpointAddresses, error) {
		return networkinternal.EndpointAddresses{
			EndpointName: name,
			Addresses:    addressesBySpace[spaceEp[name]],
		}, nil
	})

	return infos, nil
}

// GetUnitNetworkAddresses retrieves all raw unit addresses for the specified
// unit.
func (st *State) GetUnitNetworkAddresses(
	ctx context.Context,
	unitUUID string,
) ([]networkinternal.UnitAddress, error) {
	addresses, err := st.getAllUnitAddresses(ctx, unitUUID)
	if err != nil {
		return nil, errors.Errorf("getting all unit addresses: %w", err)
	}

	return addresses, nil
}

// GetUnitEgressSubnets retrieves the egress subnets for the specified unit.
func (st *State) GetUnitEgressSubnets(ctx context.Context, unitUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := entityUUID{UUID: unitUUID}
	stmt, err := st.Prepare(`
SELECT DISTINCT rne.cidr AS &egressCIDR.cidr
FROM   relation_network_egress AS rne
JOIN   relation AS r ON rne.relation_uuid = r.uuid
JOIN   relation_endpoint AS re ON r.uuid = re.relation_uuid
JOIN   relation_unit AS ru ON re.uuid = ru.relation_endpoint_uuid
WHERE  ru.unit_uuid = $entityUUID.uuid
`, egressCIDR{}, entityUUID{})
	if err != nil {
		return nil, errors.Errorf("preparing select egress subnets statement: %w", err)
	}

	var cidrs []egressCIDR
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).GetAll(&cidrs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying egress subnets for unit %q: %w", unitUUID, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	if len(cidrs) == 0 {
		return nil, nil
	}
	return transform.Slice(cidrs, func(c egressCIDR) string { return c.CIDR }), nil
}

// getAllSpacesForEndpoints retrieves all spaces matching with every endpoint
// of the unit passed as parameters.
func (st *State) getAllSpacesForEndpoints(
	ctx context.Context,
	unitUUID string,
	endpointNames []string,
) ([]spaceEndpoint, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type names []string
	currentUnitUUID := entityUUID{UUID: unitUUID}
	endpoints := names(endpointNames)
	getSpacesStmt, err := st.Prepare(`
WITH ep AS (
	SELECT ae.application_uuid,
		   cr.name,
		   ae.space_uuid
	FROM   application_endpoint ae
	JOIN   charm_relation cr ON ae.charm_relation_uuid = cr.uuid
	UNION ALL
	SELECT aee.application_uuid,
	       ceb.name,
		   aee.space_uuid
	FROM   application_extra_endpoint aee
	JOIN   charm_extra_binding ceb ON aee.charm_extra_binding_uuid = ceb.uuid
)
SELECT ep.name AS &spaceEndpoint.endpoint_name,
	   IFNULL(ep.space_uuid, a.space_uuid) AS &spaceEndpoint.space_uuid
FROM   ep 
JOIN   unit AS u ON ep.application_uuid = u.application_uuid
JOIN   application a ON u.application_uuid = a.uuid
WHERE  u.uuid = $entityUUID.uuid
AND    ep.name IN ($names[:])
`, currentUnitUUID, endpoints, spaceEndpoint{})
	if err != nil {
		return nil, errors.Errorf("preparing select spaces for endpoints statement: %w", err)
	}

	var spaces []spaceEndpoint
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, getSpacesStmt, currentUnitUUID, endpoints).GetAll(&spaces)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying spaces for endpoints: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("querying spaces for endpoints: %w", err)
	}
	return spaces, nil
}

// getAllUnitAddresses retrieves all unit addresses tied to a specific unit
// from the database.
func (st *State) getAllUnitAddresses(
	ctx context.Context,
	unitUUID string,
) ([]networkinternal.UnitAddress, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// This is a trick to embed unexported spaceAddress to fetch through SQLAIR
	type InSpaceAddress spaceAddress
	type allUnitAddressWithDevice struct {
		InSpaceAddress
		Device     string `db:"name"`
		MAC        string `db:"mac_address"`
		DeviceType string `db:"device_type"`
	}

	var address []allUnitAddressWithDevice
	ident := entityUUID{UUID: unitUUID}
	stmt, err := st.Prepare(`
WITH lld AS (
	SELECT lld.uuid, lld.name, lld.mac_address, lldt.name AS device_type
	FROM   link_layer_device AS lld
	JOIN   link_layer_device_type AS lldt ON lld.device_type_id = lldt.id
)
SELECT &allUnitAddressWithDevice.*
FROM   v_all_unit_address AS ua
JOIN   lld ON ua.device_uuid = lld.uuid
WHERE  ua.unit_uuid = $entityUUID.uuid
`, allUnitAddressWithDevice{}, entityUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).GetAll(&address)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying addresses for unit %q (and its services): %w", unitUUID, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.SliceOrErr(address, func(f allUnitAddressWithDevice) (networkinternal.UnitAddress, error) {
		encodedIP, err := encodeIPAddress(spaceAddress(f.InSpaceAddress))
		return networkinternal.UnitAddress{
			SpaceAddress: encodedIP,
			DeviceName:   f.Device,
			MACAddress:   f.MAC,
			DeviceType:   network.LinkLayerDeviceType(f.DeviceType),
		}, err
	})
}

// getAllUnitAddressesInSpaces retrieves all unit addresses tied to a specific
// unit and a list of space UUIDs from the database.
func (st *State) getAllUnitAddressesInSpaces(
	ctx context.Context,
	unitUUID string,
	spaceUUIDs []string,
) ([]networkinternal.UnitAddress, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// This is a trick to embed unexported spaceAddress to fetch through SQLAIR
	type InSpaceAddress spaceAddress
	type spaceAddressWithDevice struct {
		InSpaceAddress
		Device     string `db:"name"`
		MAC        string `db:"mac_address"`
		DeviceType string `db:"device_type"`
	}

	var address []spaceAddressWithDevice
	ident := entityUUID{UUID: unitUUID}
	stmt, err := st.Prepare(`
WITH lld AS (
	SELECT lld.uuid, lld.name, lld.mac_address, lldt.name AS device_type
	FROM   link_layer_device AS lld
	JOIN   link_layer_device_type AS lldt ON lld.device_type_id = lldt.id
)
SELECT &spaceAddressWithDevice.*
FROM   v_all_unit_address AS ua
JOIN   lld ON ua.device_uuid = lld.uuid
WHERE  ua.unit_uuid = $entityUUID.uuid
AND    space_uuid IN ($uuids[:])
`, spaceAddressWithDevice{}, entityUUID{}, uuids{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident, uuids(spaceUUIDs)).GetAll(&address)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying addresses for unit %q (and its services): %w", unitUUID, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.SliceOrErr(address, func(f spaceAddressWithDevice) (networkinternal.UnitAddress, error) {
		encodedIP, err := encodeIPAddress(spaceAddress(f.InSpaceAddress))
		return networkinternal.UnitAddress{
			SpaceAddress: encodedIP,
			DeviceName:   f.Device,
			MACAddress:   f.MAC,
			DeviceType:   network.LinkLayerDeviceType(f.DeviceType),
		}, err
	})
}

// IsCaasUnit determines if a unit identified by the given UUID is tied to a
// Kubernetes service.
func (st *State) IsCaasUnit(ctx context.Context, uuid string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	unitUUID := entityUUID{UUID: uuid}

	stmt, err := st.Prepare(`
SELECT u.uuid AS &entityUUID.uuid
FROM   k8s_service AS ks
JOIN   unit AS u ON ks.application_uuid = u.application_uuid
WHERE  u.uuid = $entityUUID.uuid`, entityUUID{})
	if err != nil {
		return false, errors.Errorf("preparing query: %w", err)
	}

	var result bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, unitUUID).Get(&unitUUID)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying k8s_service: %w", err)
		}
		result = err == nil
		return nil
	})

	return result, err
}
