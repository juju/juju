// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	porterrors "github.com/juju/juju/domain/port/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// UpdateUnitPorts opens and closes ports for the endpoints of a given unit.
// The service layer must ensure that opened and closed ports for the same
// endpoints must not conflict.
func (st *State) UpdateUnitPorts(
	ctx context.Context, unit coreunit.UUID, openPorts, closePorts network.GroupedPortRanges,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	unitUUID := unitUUID{UUID: unit}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		openPorts, closePorts, err := st.resolveWildcardEndpoints(ctx, tx, unitUUID, openPorts, closePorts)
		if err != nil {
			return errors.Errorf("resolving wildcard endpoints: %w", err)
		}

		endpointsUnderActionSet := set.NewStrings()
		for endpoint := range openPorts {
			endpointsUnderActionSet.Add(endpoint)
		}
		for endpoint := range closePorts {
			endpointsUnderActionSet.Add(endpoint)
		}
		endpointsUnderActionSet.Remove(network.WildcardEndpoint)
		endpointsUnderAction := endpoints(endpointsUnderActionSet.Values())

		endpoints, err := st.lookupRelationUUIDs(ctx, tx, unitUUID, endpointsUnderAction)
		if err != nil {
			return errors.Errorf("looking up relation endpoint uuids for unit %q: %w", unit, err)
		}

		currentUnitOpenedPorts, err := st.getUnitOpenedPorts(ctx, tx, unitUUID)
		if err != nil {
			return errors.Errorf("getting opened ports for unit %q: %w", unit, err)
		}

		err = st.openPorts(ctx, tx, openPorts, currentUnitOpenedPorts, unitUUID, endpoints)
		if err != nil {
			return errors.Errorf("opening ports for unit %q: %w", unit, err)
		}

		err = st.closePorts(ctx, tx, closePorts, currentUnitOpenedPorts)
		if err != nil {
			return errors.Errorf("closing ports for unit %q: %w", unit, err)
		}

		return nil
	})
}

// resolveWildcardEndpoints returns a new set of open and close ports that have
// been resolved against any wildcard endpoint, either in our operation, or existing
// in the database.
//
// There are a few rules to consider:
//  1. If we're opening a port range on the wildcard endpoint, we need to clean it
//     up it on all other endpoints.
//  2. If we're closing a port range for a specific endpoint which is open on the
//     wildcard endpoint, it must be closed on the wildcard endpoint as well, but
//     remain open on every other endpoint except the targeted endpoint.
//  3. If we open a port range already open on the wildcard endpoint, this is a
//     no-op.
//  4. If we close a port range on the wildcard endpoint, this should be applied
//     to all other endpoints.
func (st *State) resolveWildcardEndpoints(
	ctx context.Context, tx *sqlair.TX, unitUUID unitUUID, openPorts, closePorts network.GroupedPortRanges,
) (network.GroupedPortRanges, network.GroupedPortRanges, error) {

	// Clone input vars so they can be safely mutated
	openPorts = openPorts.Clone()
	closePorts = closePorts.Clone()

	allInputPortRanges := append(openPorts.UniquePortRanges(), closePorts.UniquePortRanges()...)

	// construct a map of all port ranges being closed to the endpoint they're
	// being closed on. Except the wildcard endpoint.
	closePortsToEndpointMap := make(map[network.PortRange]string)
	for endpoint, endpointClosePorts := range closePorts {
		for _, portRange := range endpointClosePorts {
			closePortsToEndpointMap[portRange] = endpoint
		}
	}
	// Ensure endpoints closed on the wildcard endpoint are not in the map.
	for _, wildcardClosePorts := range closePorts[network.WildcardEndpoint] {
		delete(closePortsToEndpointMap, wildcardClosePorts)
	}

	// Verify input port ranges do not conflict with any port ranges
	// co-located with the unit.
	colocatedOpened, err := st.getColocatedOpenedPorts(ctx, tx, unitUUID)
	if err != nil {
		return nil, nil, errors.Errorf("failed to get opened ports co-located with unit %s: %w", unitUUID, err)
	}
	err = verifyNoPortRangeConflicts(allInputPortRanges, colocatedOpened)
	if err != nil {
		return nil, nil, errors.Errorf("cannot update unit ports with conflict(s) on co-located units: %w", err)
	}

	wildcardOpen := openPorts[network.WildcardEndpoint]
	wildcardClose := closePorts[network.WildcardEndpoint]

	wildcardOpened, err := st.getWildcardEndpointOpenedPorts(ctx, tx, unitUUID)
	if err != nil {
		return nil, nil, errors.Errorf("failed to get opened ports for wildcard endpoint: %w", err)
	}

	// Remove openPorts ranges that are already open on the wildcard endpoint,
	// or are about to be opened on the wildcard endpoint
	wildcardOpenedSet := map[network.PortRange]bool{}
	for _, portRange := range append(wildcardOpened, wildcardOpen...) {
		wildcardOpenedSet[portRange] = true
	}
	for endpoint, endpointOpenPorts := range openPorts {
		if endpoint == network.WildcardEndpoint {
			continue
		}
		for i, portRange := range endpointOpenPorts {
			if _, ok := wildcardOpenedSet[portRange]; ok {
				openPorts[endpoint] = append(openPorts[endpoint][:i], openPorts[endpoint][i+1:]...)
			}
		}
		if len(openPorts[endpoint]) == 0 {
			delete(openPorts, endpoint)
		}
	}

	// cache for endpoints. We may need to list the existing endpoints 0, 1,
	// or n times. Cache the result and only fill it when we need it, to avoid
	// unnecessary calls.
	var endpoints []string

	// If we're opening a port range on the wildcard endpoint, we need to
	// close it on all other endpoints.
	//
	// NOTE: This ensures that if a port range is open on the wildcard
	// endpoint, it is closed on all other endpoints.
	for _, openPortRange := range wildcardOpen {
		if endpoints == nil {
			endpoints, err = st.getEndpoints(ctx, tx, unitUUID)
			if err != nil {
				return nil, nil, errors.Errorf("failed to get unit endpoints: %w", err)
			}
		}

		for _, endpoint := range endpoints {
			closePorts[endpoint] = append(closePorts[endpoint], openPortRange)
		}
	}

	// Close port ranges closed on the wildcard endpoint on all other endpoints.
	for _, closePortRange := range wildcardClose {
		if endpoints == nil {
			endpoints, err = st.getEndpoints(ctx, tx, unitUUID)
			if err != nil {
				return nil, nil, errors.Errorf("failed to get unit endpoints: %w", err)
			}
		}

		for _, endpoint := range endpoints {
			closePorts[endpoint] = append(closePorts[endpoint], closePortRange)
		}
	}

	// If we're closing a port range for a specific endpoint which is open
	// on the wildcard endpoint, we need to close it on the wildcard endpoint
	// and open it on all other endpoints except the targeted endpoint.
	for _, portRange := range wildcardOpened {
		if endpoint, ok := closePortsToEndpointMap[portRange]; ok {
			if endpoints == nil {
				endpoints, err = st.getEndpoints(ctx, tx, unitUUID)
				if err != nil {
					return nil, nil, errors.Errorf("failed to get unit endpoints: %w", err)
				}
			}

			// This port range, open on the wildcard endpoint, is being closed
			// on some endpoint. We need to close it on the wildcard, and open
			// it on all endpoints other than the wildcard & targeted endpoint.
			closePorts[network.WildcardEndpoint] = append(closePorts[network.WildcardEndpoint], portRange)

			for _, otherEndpoint := range endpoints {
				if otherEndpoint == endpoint {
					continue
				}
				openPorts[otherEndpoint] = append(openPorts[otherEndpoint], portRange)
			}

			// Remove the port range from openPorts for the targeted endpoint.
			for i, otherPortRange := range openPorts[endpoint] {
				if otherPortRange == portRange {
					openPorts[endpoint] = append(openPorts[endpoint][:i], openPorts[endpoint][i+1:]...)
					break
				}
			}
			if len(openPorts[endpoint]) == 0 {
				delete(openPorts, endpoint)
			}
		}
	}

	return openPorts, closePorts, nil
}

// verifyNoPortRangeConflicts verifies the provided port ranges do not conflict
// with each other.
//
// A conflict occurs when two (or more) port ranges across all endpoints overlap,
// but are not equal.
func verifyNoPortRangeConflicts(rangesA, rangesB []network.PortRange) error {
	var conflicts []error
	for _, portRange := range rangesA {
		for _, otherPortRange := range rangesB {
			if portRange != otherPortRange && portRange.ConflictsWith(otherPortRange) {
				conflicts = append(conflicts, errors.Errorf("[%s, %s]", portRange, otherPortRange))
			}
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	return errors.Errorf("%w: %w", porterrors.PortRangeConflict, errors.Join(conflicts...))
}

// getColocatedOpenedPorts returns all the open ports for all units co-located with
// the given unit. Units are considered co-located if they share the same net-node.
func (st *State) getColocatedOpenedPorts(ctx context.Context, tx *sqlair.TX, unitUUID unitUUID) ([]network.PortRange, error) {
	getOpenedPorts, err := st.Prepare(`
SELECT &portRange.*
FROM v_port_range AS pr
JOIN unit AS u ON unit_uuid = u.uuid
JOIN unit AS u2 on u2.net_node_uuid = u.net_node_uuid
WHERE u2.uuid = $unitUUID.unit_uuid
`, portRange{}, unitUUID)
	if err != nil {
		return nil, errors.Errorf("preparing get colocated opened ports statement: %w", err)
	}

	portRanges := []portRange{}
	err = tx.Query(ctx, getOpenedPorts, unitUUID).GetAll(&portRanges)
	if errors.Is(err, sqlair.ErrNoRows) {
		return []network.PortRange{}, nil
	}
	if err != nil {
		return nil, errors.Errorf("getting opened ports for colocated units with %q: %w", unitUUID, err)
	}

	ret := transform.Slice(portRanges, portRange.decode)
	network.SortPortRanges(ret)
	return ret, nil
}

// getWildcardEndpointOpenedPorts returns the opened ports for the wildcard endpoint of a
// given unit.
func (st *State) getWildcardEndpointOpenedPorts(ctx context.Context, tx *sqlair.TX, unitUUID unitUUID) ([]network.PortRange, error) {
	query, err := st.Prepare(`
SELECT &portRange.*
FROM v_port_range
WHERE unit_uuid = $unitUUID.unit_uuid
AND endpoint IS NULL
`, portRange{}, unitUUID)
	if err != nil {
		return nil, errors.Errorf("preparing get endpoint opened ports statement for wildcard ep: %w", err)
	}

	var portRanges []portRange
	err = tx.Query(ctx, query, unitUUID).GetAll(&portRanges)
	if errors.Is(err, sqlair.ErrNoRows) {
		return []network.PortRange{}, nil
	}
	if err != nil {
		return nil, errors.Errorf("getting opened ports for wildcard endpoint of unit %q: %w", unitUUID, err)
	}

	decodedPortRanges := make([]network.PortRange, len(portRanges))
	for i, pr := range portRanges {
		decodedPortRanges[i] = pr.decode()
	}
	network.SortPortRanges(decodedPortRanges)
	return decodedPortRanges, nil
}

// getEndpoints returns all the valid relation endpoints for a given unit. This
// does not include the special wildcard endpoint.
func (st *State) getEndpoints(ctx context.Context, tx *sqlair.TX, unitUUID unitUUID) ([]string, error) {
	getEndpoints, err := st.Prepare(`
SELECT &endpointName.*
FROM v_endpoint
WHERE unit_uuid = $unitUUID.unit_uuid
`, endpointName{}, unitUUID)
	if err != nil {
		return nil, errors.Errorf("preparing get endpoints statement: %w", err)
	}

	var endpointNames []endpointName
	err = tx.Query(ctx, getEndpoints, unitUUID).GetAll(&endpointNames)
	if errors.Is(err, sqlair.ErrNoRows) {
		return []string{}, nil
	}
	if err != nil {
		return nil, errors.Errorf("getting endpoints for unit %q: %w", unitUUID, err)
	}

	endpoints := make([]string, len(endpointNames))
	for i, ep := range endpointNames {
		endpoints[i] = ep.Endpoint
	}
	return endpoints, nil
}

// lookupRelationUUIDs returns the UUIDs of the given endpoints/relations on a given unit.
// We return a slice of `endpoint` structs, allowing us to identify which endpoint/relation
// uuid is which, as order is not guaranteed.
func (st *State) lookupRelationUUIDs(
	ctx context.Context, tx *sqlair.TX, unitUUID unitUUID, endpointNames endpoints,
) ([]endpoint, error) {
	getEndpoints, err := st.Prepare(`
SELECT &endpoint.*
FROM v_endpoint
WHERE unit_uuid = $unitUUID.unit_uuid
AND endpoint IN ($endpoints[:])
`, endpoint{}, unitUUID, endpointNames)
	if err != nil {
		return nil, errors.Errorf("preparing get endpoints statement: %w", err)
	}

	endpoints := []endpoint{}
	err = tx.Query(ctx, getEndpoints, unitUUID, endpointNames).GetAll(&endpoints)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	if len(endpoints) != len(endpointNames) {
		endpointNamesSet := set.NewStrings([]string(endpointNames)...)
		for _, ep := range endpoints {
			endpointNamesSet.Remove(ep.Endpoint)
		}
		return nil, errors.Errorf(
			"%w; %v does exist on unit %v",
			porterrors.InvalidEndpoint, endpointNamesSet.Values(), unitUUID.UUID,
		)
	}

	return endpoints, nil
}

// getUnitOpenedPorts returns the opened ports for the given unit.
//
// NOTE: This differs from GetUnitOpenedPorts in that it returns port ranges with
// their UUIDs, which are not needed by GetUnitOpenedPorts.
func (st *State) getUnitOpenedPorts(ctx context.Context, tx *sqlair.TX, unitUUID unitUUID) ([]endpointPortRangeUUID, error) {
	getOpenedPorts, err := st.Prepare(`
SELECT &endpointPortRangeUUID.*
FROM v_port_range
WHERE unit_uuid = $unitUUID.unit_uuid
`, endpointPortRangeUUID{}, unitUUID)
	if err != nil {
		return nil, errors.Errorf("preparing get opened ports statement: %w", err)
	}

	var openedPorts []endpointPortRangeUUID
	err = tx.Query(ctx, getOpenedPorts, unitUUID).GetAll(&openedPorts)
	if errors.Is(err, sqlair.ErrNoRows) {
		return []endpointPortRangeUUID{}, nil
	}
	if err != nil {
		return nil, errors.Capture(err)
	}
	return openedPorts, nil
}

// openPorts inserts the given port ranges into the database, unless they're already open.
func (st *State) openPorts(
	ctx context.Context, tx *sqlair.TX,
	openPorts network.GroupedPortRanges, currentOpenedPorts []endpointPortRangeUUID, unitUUID unitUUID, endpoints []endpoint,
) error {
	insertPortRange, err := st.Prepare("INSERT INTO port_range (*) VALUES ($unitPortRange.*)", unitPortRange{})
	if err != nil {
		return errors.Errorf("preparing insert port range statement: %w", err)
	}

	protocolMap, err := st.getProtocolMap(ctx, tx)
	if err != nil {
		return errors.Errorf("getting protocol map: %w", err)
	}

	// construct a map from endpoint name to it's UUID.
	endpointUUIDMaps := make(map[string]string)
	for _, ep := range endpoints {
		endpointUUIDMaps[ep.Endpoint] = ep.UUID
	}

	// index the current opened ports by endpoint and port range
	currentOpenedPortRangeExistenceIndex := make(map[string]map[network.PortRange]bool)
	for _, openedPortRange := range currentOpenedPorts {
		if _, ok := currentOpenedPortRangeExistenceIndex[openedPortRange.Endpoint]; !ok {
			currentOpenedPortRangeExistenceIndex[openedPortRange.Endpoint] = make(map[network.PortRange]bool)
		}
		currentOpenedPortRangeExistenceIndex[openedPortRange.Endpoint][openedPortRange.decode()] = true
	}

	for ep, ports := range openPorts {
		for _, portRange := range ports {
			// skip port range if it's already open on this endpoint
			if _, ok := currentOpenedPortRangeExistenceIndex[ep][portRange]; ok {
				continue
			}
			uuid, err := uuid.NewUUID()
			if err != nil {
				return errors.Errorf("generating UUID for port range: %w", err)
			}
			var relationUUID string
			if ep != network.WildcardEndpoint {
				relationUUID = endpointUUIDMaps[ep]
			}
			unitPortRange := unitPortRange{
				UUID:         uuid.String(),
				ProtocolID:   protocolMap[portRange.Protocol],
				FromPort:     portRange.FromPort,
				ToPort:       portRange.ToPort,
				UnitUUID:     unitUUID.UUID,
				RelationUUID: relationUUID,
			}
			err = tx.Query(ctx, insertPortRange, unitPortRange).Run()
			if err != nil {
				return errors.Capture(err)
			}
		}
	}

	return nil
}

// getProtocolMap returns a map of protocol names to their IDs in DQLite.
func (st *State) getProtocolMap(ctx context.Context, tx *sqlair.TX) (map[string]int, error) {
	getProtocols, err := st.Prepare("SELECT &protocol.* FROM protocol", protocol{})
	if err != nil {
		return nil, errors.Errorf("preparing get protocol ID statement: %w", err)
	}

	protocols := []protocol{}
	err = tx.Query(ctx, getProtocols).GetAll(&protocols)
	if err != nil {
		return nil, errors.Capture(err)
	}

	protocolMap := map[string]int{}
	for _, protocol := range protocols {
		protocolMap[protocol.Name] = protocol.ID
	}

	return protocolMap, nil
}

// closePorts removes the given port ranges from the database, if they exist.
func (st *State) closePorts(
	ctx context.Context, tx *sqlair.TX, closePorts network.GroupedPortRanges, currentOpenedPorts []endpointPortRangeUUID,
) error {
	closePortRanges, err := st.Prepare(`
DELETE FROM port_range
WHERE uuid IN ($portRangeUUIDs[:])
`, portRangeUUIDs{})
	if err != nil {
		return errors.Errorf("preparing close port range statement: %w", err)
	}

	// index the uuids of current opened ports by endpoint and port range
	openedPortRangeUUIDIndex := make(map[string]map[network.PortRange]string)
	for _, openedPortRange := range currentOpenedPorts {
		if _, ok := openedPortRangeUUIDIndex[openedPortRange.Endpoint]; !ok {
			openedPortRangeUUIDIndex[openedPortRange.Endpoint] = make(map[network.PortRange]string)
		}
		openedPortRangeUUIDIndex[openedPortRange.Endpoint][openedPortRange.decode()] = openedPortRange.UUID
	}

	// Find the uuids of port ranges to close
	var closePortRangeUUIDs portRangeUUIDs
	for endpoint, portRanges := range closePorts {
		index, ok := openedPortRangeUUIDIndex[endpoint]
		if !ok {
			continue
		}

		for _, closePortRange := range portRanges {
			openedRangeUUID, ok := index[closePortRange]
			if !ok {
				continue
			}
			closePortRangeUUIDs = append(closePortRangeUUIDs, openedRangeUUID)
		}
	}

	if len(closePortRangeUUIDs) > 0 {
		err = tx.Query(ctx, closePortRanges, closePortRangeUUIDs).Run()
		if err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}
