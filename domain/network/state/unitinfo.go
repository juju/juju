// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"maps"
	"slices"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/network"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// GetUnitEndpointNetworks retrieves network information for the specified unit
// and endpoints, including device and ingress details.
func (st *State) GetUnitEndpointNetworks(ctx context.Context, unitUUID string,
	endpointNames []string) ([]domainnetwork.UnitNetwork, error) {

	isCaas, err := st.isCaasUnit(ctx, unitUUID)
	if err != nil {
		return nil, errors.Errorf("checking if unit is caas: %w", err)
	}
	// get all spaces for all endpoints
	spaces, err := st.getAllSpacesForEndpoints(ctx, unitUUID, endpointNames)
	if err != nil {
		return nil, errors.Errorf("getting all spaces for endpoints: %w", err)
	}

	// Unique spaces
	uniqueSpaces := set.NewStrings()
	for _, space := range spaces {
		uniqueSpaces.Add(space.SpaceUUID)
	}

	// Get all addresses for spaces
	addresses, err := st.getAllUnitAddressesInSpaces(ctx, unitUUID, uniqueSpaces.Values())
	if err != nil {
		return nil, errors.Errorf("getting all unit addresses in spaces: %w", err)
	}

	addressesBySpace, _ := accumulateToMap(addresses, func(address unitAddress) (string, unitAddress, error) {
		return address.SpaceID.String(), address, nil
	})
	infoBySpaces := transform.Map(addressesBySpace, func(spaceUUID string, addrs []unitAddress) (string,
		domainnetwork.UnitNetwork) {
		byDevice := map[string]domainnetwork.DeviceInfo{}
		var ingressAddresses network.SpaceAddresses

		for _, addr := range addrs {
			devInfo, ok := byDevice[addr.Device]
			if !ok {
				devInfo.Name = addr.Device
				devInfo.MACAddress = addr.MAC
			}

			if !isCaas || addr.Scope == network.ScopeMachineLocal {
				devInfo.Addresses = append(devInfo.Addresses, domainnetwork.AddressInfo{
					Hostname: addr.Host(),
					Value:    addr.IP().String(),
					CIDR:     addr.AddressCIDR(),
				})
			}
			if !isCaas || addr.Scope != network.ScopeMachineLocal {
				ingressAddresses = append(ingressAddresses, addr.SpaceAddress)
			}

			byDevice[addr.Device] = devInfo
		}

		// We use the same sorting algorithm than in GetUnitAddresses (this is very important,
		//   otherwise the unit may use different valid IP addresses in different places)
		sortedIngressAddresses := ingressAddresses.AllMatchingScope(network.ScopeMatchCloudLocal).Values()
		return spaceUUID, domainnetwork.UnitNetwork{
			DeviceInfos:      slices.Collect(maps.Values(byDevice)),
			IngressAddresses: sortedIngressAddresses,
		}
	})

	spaceEp := transform.SliceToMap(spaces, func(endpoint spaceEndpoint) (string, string) {
		return endpoint.EndpointName, endpoint.SpaceUUID
	})
	// reformate to dispatch info by endpoint.
	infos, _ := transform.SliceOrErr(endpointNames, func(name string) (domainnetwork.UnitNetwork, error) {
		spaceUUID := spaceEp[name]
		info := infoBySpaces[spaceUUID]
		return domainnetwork.UnitNetwork{
			EndpointName:     name,
			DeviceInfos:      info.DeviceInfos,
			IngressAddresses: info.IngressAddresses,
		}, nil
	})

	return infos, nil
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
SELECT  
    epview.name AS &spaceEndpoint.endpoint_name,
	epview.space_uuid AS &spaceEndpoint.space_uuid
FROM (
	SELECT 
		ae.application_uuid,
		cr.name,
		COALESCE(ae.space_uuid, a.space_uuid) AS space_uuid
	FROM 
		application_endpoint ae
	JOIN 
		charm_relation cr ON ae.charm_relation_uuid = cr.uuid
	JOIN 
		application a ON ae.application_uuid = a.uuid
	UNION
	SELECT 
		aee.application_uuid,
		ceb.name,
		COALESCE(aee.space_uuid, a.space_uuid) AS space_uuid
	FROM 
		application_extra_endpoint aee
	JOIN 
		charm_extra_binding ceb ON aee.charm_extra_binding_uuid = ceb.uuid
	JOIN 
		application a ON aee.application_uuid = a.uuid
) as epview
JOIN unit AS u ON epview.application_uuid = u.application_uuid
WHERE u.uuid = $entityUUID.uuid
AND epview.name IN ($names[:])
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

// getAllUnitAddressesInSpaces retrieves all unit addresses tied to a specific
// unit and a list of space UUIDs from the database.
func (st *State) getAllUnitAddressesInSpaces(
	ctx context.Context,
	unitUUID string,
	spaceUUIDs []string,
) ([]unitAddress, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// This is a trick to embed unexported spaceAddress to fetch through SQLAIR
	type InSpaceAddress spaceAddress
	type spaceAddressWithDevice struct {
		InSpaceAddress
		Device string `db:"name"`
		MAC    string `db:"mac_address"`
	}

	var address []spaceAddressWithDevice
	ident := entityUUID{UUID: unitUUID}
	stmt, err := st.Prepare(`
SELECT &spaceAddressWithDevice.*
FROM v_all_unit_address AS ua
JOIN link_layer_device AS lld ON ua.device_uuid = lld.uuid
WHERE     ua.unit_uuid = $entityUUID.uuid
AND space_uuid IN ($uuids[:])
`, spaceAddressWithDevice{}, entityUUID{}, uuids{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident, uuids(spaceUUIDs)).GetAll(&address)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying addresses for unit %q (and it's services): %w", unitUUID, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return transform.SliceOrErr(address, func(f spaceAddressWithDevice) (unitAddress, error) {
		encodedIP, err := encodeIPAddress(spaceAddress(f.InSpaceAddress))
		return unitAddress{
			SpaceAddress: encodedIP,
			Device:       f.Device,
			MAC:          f.MAC,
		}, err
	})
}

// isCaasUnit determines if a unit identified by the given UUID is tied to a
// Kubernetes service.
func (st *State) isCaasUnit(ctx context.Context, uuid string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	unitUUID := entityUUID{UUID: uuid}

	stmt, err := st.Prepare(`
SELECT u.uuid AS &entityUUID.uuid
FROM k8s_service AS ks
JOIN unit AS u ON ks.application_uuid = u.application_uuid
WHERE u.uuid = $entityUUID.uuid`, entityUUID{})
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

// unitAddress represents a network address assigned to a specific unit,
// including additional information like device and MAC.
type unitAddress struct {
	network.SpaceAddress
	Device string
	MAC    string
}
