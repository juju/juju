// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"net"
	"sort"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/unitstate/internal"
	"github.com/juju/juju/internal/errors"
)

// GetUnitRelationNetworkInfosNetworkingNotSupported retrieves egress subnets
// and ingress addresses for the specified unit by selecting the best candidate
// from *all* unit addresses. These addresses are linked with all relations
// where the given unit is in scope.
// This is used on providers that do not support networking, and therefore
// can not factor endpoint bindings.
func (st *State) GetUnitRelationNetworkInfosNetworkingNotSupported(
	ctx context.Context,
	unitUUID coreunit.UUID,
) ([]internal.RelationNetworkInfo, error) {
	isCaas, err := st.isCaasUnit(ctx, unitUUID.String())
	if err != nil {
		return nil, errors.Errorf("checking if unit is caas: %w", err)
	}

	addresses, err := st.getAllUnitAddresses(ctx, unitUUID.String())
	if err != nil {
		return nil, errors.Errorf("getting all unit addresses: %w", err)
	}

	info := buildRelationNetworkInfoFromAddresses(addresses, isCaas)
	info.EgressSubnets, err = st.getUnitEgressSubnets(ctx, unitUUID.String())
	if err != nil {
		return nil, errors.Errorf("getting unit egress subnets: %w", err)
	}

	relationUUIDs, err := st.getRelationUUIDsforUnit(ctx, unitUUID.String())
	if err != nil {
		return nil, errors.Errorf("getting relation UUIDs for unit %q: %w", unitUUID, err)
	}

	result := make([]internal.RelationNetworkInfo, len(relationUUIDs))
	for i, relationUUID := range relationUUIDs {
		result[i] = info
		result[i].RelationUUID = relationUUID
	}

	return result, nil
}

// getAllUnitAddresses retrieves all unit addresses tied to a specific unit
// from the database.
func (st *State) getAllUnitAddresses(
	ctx context.Context,
	uUUID string,
) ([]network.SpaceAddress, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var address []spaceAddress
	ident := entityUUID{UUID: uUUID}
	stmt, err := st.Prepare(`
SELECT &spaceAddress.*
FROM   v_all_unit_address
WHERE  unit_uuid = $entityUUID.uuid
`, spaceAddress{}, entityUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).GetAll(&address)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying addresses for unit %q (and its services): %w", uUUID, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.SliceOrErr(address, func(f spaceAddress) (network.SpaceAddress, error) {
		return encodeIPAddress(f)
	})
}

func (st *State) getUnitEgressSubnets(ctx context.Context, unitUUID string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
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
		return "", errors.Errorf("preparing select egress subnets statement: %w", err)
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
		return "", errors.Capture(err)
	}

	if len(cidrs) == 0 {
		return "", nil
	}
	cidrStrs := transform.Slice(cidrs, func(c egressCIDR) string { return c.CIDR })
	sort.Strings(cidrStrs)
	return strings.Join(cidrStrs, ", "), nil
}

func buildRelationNetworkInfoFromAddresses(addresses []network.SpaceAddress, isCaas bool) internal.RelationNetworkInfo {
	var ingressAddresses network.SpaceAddresses
	for _, addr := range addresses {
		// The purpose of the method is to get connectivity information for
		// the unit. Skip loopback addresses to focus on external connectivity.
		if addr.IP().IsLoopback() {
			continue
		}

		if !isCaas || addr.Scope != network.ScopeMachineLocal {
			ingressAddresses = append(ingressAddresses, addr)
		}
	}

	// We use the same sorting algorithm as in GetUnitAddresses.
	// It is important that the selected address is the same every time for a
	// given set of addresses.
	sortedIngressAddresses := ingressAddresses.AllMatchingScope(network.ScopeMatchCloudLocal).Values()
	result := internal.RelationNetworkInfo{}
	if len(sortedIngressAddresses) > 0 {
		result.IngressAddress = sortedIngressAddresses[0]
	}
	return result
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

func encodeIPAddress(address spaceAddress) (network.SpaceAddress, error) {
	spaceUUID := network.AlphaSpaceId
	if address.SpaceUUID.Valid {
		spaceUUID = network.SpaceUUID(address.SpaceUUID.String)
	}
	// The saved address value is in the form 192.0.2.1/24,
	// parse the parts for the MachineAddress
	ipAddr, ipNet, err := net.ParseCIDR(address.Value)
	if err != nil {
		// Note: IP addresses from Kubernetes do not contain subnet
		// mask suffixes yet. Handle that scenario here. Eventually
		// an error should be returned instead.
		ipAddr = net.ParseIP(address.Value)
	}
	cidr := ipNet.String()
	// Prefer the subnet cidr if one exists.
	if address.SubnetCIDR.Valid {
		cidr = address.SubnetCIDR.String
	}
	return network.SpaceAddress{
		SpaceID: spaceUUID,
		Origin:  network.Origin(address.Origin),
		MachineAddress: network.MachineAddress{
			Value:      ipAddr.String(),
			CIDR:       cidr,
			Type:       network.AddressType(address.Type),
			Scope:      network.Scope(address.Scope),
			ConfigType: network.AddressConfigType(address.ConfigType),
		},
	}, nil
}

func (st *State) getRelationUUIDsforUnit(ctx context.Context, unitUUID string) ([]relation.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `
SELECT re.relation_uuid AS &entityUUID.uuid
FROM   relation_unit AS ru
JOIN   relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
WHERE  ru.unit_uuid = $entityUUID.uuid
`
	stmt, err := st.Prepare(query, entityUUID{})
	if err != nil {
		return nil, errors.Errorf("preparing relation UUIDs query for unit %q: %w", unitUUID, err)
	}

	type relationUUIDs []entityUUID
	var results relationUUIDs

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		qErr := tx.Query(ctx, stmt, entityUUID{UUID: unitUUID}).GetAll(&results)
		if qErr != nil && !errors.Is(qErr, sqlair.ErrNoRows) {
			return errors.Errorf("querying relation UUIDs for unit %q: %w", unitUUID, qErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return transform.Slice(results, func(in entityUUID) relation.UUID { return relation.UUID(in.UUID) }), nil
}

// GetUnitRelationNetworkInfos retrieves network info for all relations
// where the unit is in scope.
func (st *State) GetUnitRelationNetworkInfos(
	ctx context.Context, unitUUID coreunit.UUID,
) ([]internal.RelationNetworkInfo, error) {

	// Determine the set of unique spaces to which the endpoints are bound.
	relationSpaces, err := st.getAllSpacesForUnitsRelations(ctx, unitUUID.String())
	if err != nil {
		return nil, errors.Capture(err)
	}
	uniqueSpaces := set.NewStrings()
	for _, spaces := range relationSpaces {
		for _, space := range spaces {
			uniqueSpaces.Add(space)
		}
	}

	// Then get the unit's addresses in those spaces.
	addresses, err := st.getAllUnitAddressesInSpaces(ctx, unitUUID.String(), uniqueSpaces.Values())
	if err != nil {
		return nil, errors.Errorf("getting all unit addresses in spaces: %w", err)
	}

	isCaas, err := st.isCaasUnit(ctx, unitUUID.String())
	if err != nil {
		return nil, errors.Errorf("checking if unit is caas: %w", err)
	}
	addressesBySpace, _ := accumulateToMap(addresses, func(address network.SpaceAddress) (string, network.SpaceAddress, error) {
		return address.SpaceID.String(), address, nil
	})
	infoBySpaces := transform.Map(
		addressesBySpace,
		func(spaceUUID string, addrs []network.SpaceAddress) (string, internal.RelationNetworkInfo) {
			return spaceUUID, buildRelationNetworkInfoFromAddresses(addrs, isCaas)
		})

	// All endpoints of the unit will have the same egress subnet.
	egressCIDRs, err := st.getUnitEgressSubnets(ctx, unitUUID.String())
	if err != nil {
		return nil, errors.Errorf("getting unit egress subnets: %w", err)
	}

	// For every relation space pair - find the space ingress address, create a
	// RelationNetworkInfo with the relation uuid, the unit egress subnets and
	// the space ingress address.
	result := make([]internal.RelationNetworkInfo, 0)
	for relationUUID, spaceUUIDs := range relationSpaces {
		for _, spaceUUID := range spaceUUIDs {
			info := infoBySpaces[spaceUUID]
			info.RelationUUID = relationUUID
			info.EgressSubnets = egressCIDRs
			result = append(result, info)
		}
	}

	return result, nil
}

// getAllSpacesForUnitsRelations retrieves all spaces matching with every relation
// of the unit.
func (st *State) getAllSpacesForUnitsRelations(
	ctx context.Context,
	unitUUID string,
) (map[relation.UUID][]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	currentUnitUUID := entityUUID{UUID: unitUUID}
	getSpacesStmt, err := st.Prepare(`
SELECT re.relation_uuid AS &spaceRelation.relation_uuid,
	   IFNULL(ae.space_uuid, a.space_uuid) AS &spaceRelation.space_uuid
FROM   unit AS u
JOIN   application a ON u.application_uuid = a.uuid
JOIN   relation_unit AS ru ON u.uuid = ru.unit_uuid
JOIN   relation_endpoint AS re ON ru.relation_endpoint_uuid = re.uuid
JOIN   application_endpoint AS ae ON a.uuid = ae.application_uuid AND re.endpoint_uuid = ae.uuid
WHERE  u.uuid = $entityUUID.uuid
`, currentUnitUUID, spaceRelation{})
	if err != nil {
		return nil, errors.Errorf("preparing select spaces for endpoints statement: %w", err)
	}

	var spaces []spaceRelation
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, getSpacesStmt, currentUnitUUID).GetAll(&spaces)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying spaces for endpoints: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("querying spaces for endpoints: %w", err)
	}
	result := make(map[relation.UUID][]string)
	for _, space := range spaces {
		v, ok := result[relation.UUID(space.RelationUUID)]
		if ok {
			result[relation.UUID(space.RelationUUID)] = append(v, space.SpaceUUID)
			continue
		}
		result[relation.UUID(space.RelationUUID)] = []string{space.SpaceUUID}
	}
	return result, nil
}

// getAllUnitAddressesInSpaces retrieves all unit addresses tied to a specific
// unit and a list of space UUIDs from the database.
func (st *State) getAllUnitAddressesInSpaces(
	ctx context.Context,
	unitUUID string,
	spaceUUIDs []string,
) ([]network.SpaceAddress, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var address []spaceAddress
	ident := entityUUID{UUID: unitUUID}
	stmt, err := st.Prepare(`
SELECT &spaceAddress.*
FROM   v_all_unit_address
WHERE  unit_uuid = $entityUUID.uuid
AND    space_uuid IN ($uuids[:])
`, spaceAddress{}, entityUUID{}, uuids{})
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
	return transform.SliceOrErr(address, func(f spaceAddress) (network.SpaceAddress, error) {
		return encodeIPAddress(f)
	})
}

// accumulateToMap transforms a slice of elements into a map of keys to slices
// of values using the provided transform function.
// If the transformation function results in an error, end the loop and return
// the error
func accumulateToMap[F any, K comparable, V any](from []F, transform func(F) (K, V, error)) (map[K][]V, error) {
	to := make(map[K][]V)
	for _, oneFrom := range from {
		k, v, err := transform(oneFrom)
		if err != nil {
			return nil, errors.Capture(err)
		}
		to[k] = append(to[k], v)
	}
	return to, nil
}
