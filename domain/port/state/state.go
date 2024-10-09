// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	jujuerrors "github.com/juju/errors"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/port"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// State represents the persistence layer for opened ports.
type State struct {
	*domain.StateBase
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetUnitOpenedPorts returns the opened ports for a given unit uuid,
// grouped by endpoint.
func (st *State) GetUnitOpenedPorts(ctx context.Context, unit coreunit.UUID) (network.GroupedPortRanges, error) {
	db, err := st.DB()
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}

	unitUUID := unitUUID{UUID: unit.String()}

	query, err := st.Prepare(`
SELECT &endpointPortRange.*
FROM port_range
JOIN protocol ON port_range.protocol_id = protocol.id
JOIN unit_endpoint ON port_range.unit_endpoint_uuid = unit_endpoint.uuid
WHERE unit_endpoint.unit_uuid = $unitUUID.unit_uuid
`, endpointPortRange{}, unitUUID)
	if err != nil {
		return nil, errors.Errorf("preparing get unit opened ports statement: %w", err)
	}

	results := []endpointPortRange{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, unitUUID).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return nil, errors.Errorf("getting opened ports for unit %q: %w", unit, err)
	}

	groupedPortRanges := network.GroupedPortRanges{}
	for _, endpointPortRange := range results {
		endpointName := endpointPortRange.Endpoint
		if _, ok := groupedPortRanges[endpointName]; !ok {
			groupedPortRanges[endpointPortRange.Endpoint] = []network.PortRange{}
		}
		groupedPortRanges[endpointName] = append(groupedPortRanges[endpointName], endpointPortRange.decode())
	}

	for _, portRanges := range groupedPortRanges {
		network.SortPortRanges(portRanges)
	}

	return groupedPortRanges, nil
}

// GetMachineOpenedPorts returns the opened ports for all the units on the given
// machine. Opened ports are grouped first by unit and then by endpoint.
//
// NOTE: In the ddl machines and units both share 1-to-1 relations with net_nodes.
// So to join units to machines we go via their net_nodes.
//
// TODO: Once we have a core static machine uuid type, use it here.
func (st *State) GetMachineOpenedPorts(ctx context.Context, machine string) (map[coreunit.UUID]network.GroupedPortRanges, error) {
	db, err := st.DB()
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}

	machineUUID := machineUUID{UUID: machine}

	query, err := st.Prepare(`
SELECT &unitEndpointPortRange.*
FROM port_range
JOIN protocol ON port_range.protocol_id = protocol.id
JOIN unit_endpoint ON port_range.unit_endpoint_uuid = unit_endpoint.uuid
JOIN unit ON unit_endpoint.unit_uuid = unit.uuid
JOIN machine ON unit.net_node_uuid = machine.net_node_uuid
WHERE machine.uuid = $machineUUID.machine_uuid
`, unitEndpointPortRange{}, machineUUID)
	if err != nil {
		return nil, errors.Errorf("preparing get machine opened ports statement: %w", err)
	}

	results := []unitEndpointPortRange{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, machineUUID).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return nil, errors.Errorf("getting opened ports for machine %q: %w", machine, err)
	}

	decoded := port.UnitEndpointPortRanges(transform.Slice(results, func(p unitEndpointPortRange) port.UnitEndpointPortRange {
		return p.decodeToUnitEndpointPortRange()
	}))
	return decoded.ByUnitByEndpoint(), nil
}

// GetApplicationOpenedPorts returns the opened ports for all the units of the
// given application. We return opened ports paired with the unit UUIDs, grouped
// by endpoint. This is because some consumers do not care about the unit.
func (st *State) GetApplicationOpenedPorts(ctx context.Context, application coreapplication.ID) (port.UnitEndpointPortRanges, error) {
	db, err := st.DB()
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}

	applicationUUID := applicationUUID{UUID: application.String()}

	query, err := st.Prepare(`
SELECT &unitEndpointPortRange.*
FROM port_range
JOIN protocol ON port_range.protocol_id = protocol.id
JOIN unit_endpoint ON port_range.unit_endpoint_uuid = unit_endpoint.uuid
JOIN unit ON unit_endpoint.unit_uuid = unit.uuid
WHERE unit.application_uuid = $applicationUUID.application_uuid
`, unitEndpointPortRange{}, applicationUUID)
	if err != nil {
		return nil, errors.Errorf("preparing get application opened ports statement: %w", err)
	}

	results := []unitEndpointPortRange{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, applicationUUID).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return nil, errors.Errorf("getting opened ports for application %q: %w", application, err)
	}

	ret := transform.Slice(results, func(p unitEndpointPortRange) port.UnitEndpointPortRange {
		return p.decodeToUnitEndpointPortRange()
	})
	port.SortUnitEndpointPortRanges(ret)
	return ret, nil
}

// GetColocatedOpenedPorts returns all the open ports for all units co-located with
// the given unit. Units are considered co-located if they share the same net-node.
func (st *State) GetColocatedOpenedPorts(ctx domain.AtomicContext, unit coreunit.UUID) ([]network.PortRange, error) {
	unitUUID := unitUUID{UUID: unit.String()}

	getOpenedPorts, err := st.Prepare(`
SELECT &portRange.*
FROM port_range AS pr
JOIN protocol AS p ON pr.protocol_id = p.id
JOIN unit_endpoint AS ep ON pr.unit_endpoint_uuid = ep.uuid
JOIN unit AS u ON ep.unit_uuid = u.uuid
JOIN unit AS u2 on u2.net_node_uuid = u.net_node_uuid
WHERE u2.uuid = $unitUUID.unit_uuid
`, portRange{}, unitUUID)
	if err != nil {
		return nil, errors.Errorf("preparing get colocated opened ports statement: %w", err)
	}

	portRanges := []portRange{}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, getOpenedPorts, unitUUID).GetAll(&portRanges)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return nil, errors.Errorf("getting opened ports for colocated units with %q: %w", unit, err)
	}

	ret := transform.Slice(portRanges, func(p portRange) network.PortRange { return p.decode() })
	network.SortPortRanges(ret)
	return ret, nil
}

// GetEndpointOpenedPorts returns the opened ports for a given endpoint of a
// given unit.
func (st *State) GetEndpointOpenedPorts(ctx domain.AtomicContext, unit coreunit.UUID, endpoint string) ([]network.PortRange, error) {
	unitUUID := unitUUID{UUID: unit.String()}
	endpointName := endpointName{Endpoint: endpoint}

	query, err := st.Prepare(`
SELECT &portRange.*
FROM port_range
JOIN protocol ON port_range.protocol_id = protocol.id
JOIN unit_endpoint ON port_range.unit_endpoint_uuid = unit_endpoint.uuid
WHERE unit_endpoint.unit_uuid = $unitUUID.unit_uuid
AND unit_endpoint.endpoint = $endpointName.endpoint
`, portRange{}, unitUUID, endpointName)
	if err != nil {
		return nil, errors.Errorf("preparing get endpoint opened ports statement: %w", err)
	}

	var portRanges []portRange
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, unitUUID, endpointName).GetAll(&portRanges)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return nil, errors.Errorf("getting opened ports for endpoint %q of unit %q: %w", endpoint, unit, err)
	}

	decodedPortRanges := make([]network.PortRange, len(portRanges))
	for i, pr := range portRanges {
		decodedPortRanges[i] = pr.decode()
	}
	network.SortPortRanges(decodedPortRanges)
	return decodedPortRanges, nil
}

// SetUnitPorts sets open ports for the endpoints of a given unit.
func (st *State) SetUnitPorts(
	ctx context.Context, unitName string, openPorts network.GroupedPortRanges,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Errorf("getting DB: %w", err)
	}

	endpoints := make([]string, len(openPorts))
	i := 0
	for endpoint := range openPorts {
		endpoints[i] = endpoint
		i++
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unitUUID, err := st.getUnitUUID(ctx, tx, unitName)
		if err != nil {
			return errors.Errorf("getting uuid of unit %q: %w", unitName, err)
		}

		endpoints, err := st.ensureEndpoints(ctx, tx, unitUUID, endpoints)
		if err != nil {
			return errors.Errorf("ensuring endpoints exist for unit %q: %w", unitName, err)
		}

		err = st.openPorts(ctx, tx, openPorts, nil, endpoints)
		if err != nil {
			return errors.Errorf("opening ports for unit %q: %w", unitName, err)
		}

		return nil
	})
}

// UpdateUnitPorts opens and closes ports for the endpoints of a given unit.
// The service layer must ensure that opened and closed ports for the same
// endpoints must not conflict.
func (st *State) UpdateUnitPorts(
	ctx domain.AtomicContext, unit coreunit.UUID, openPorts, closePorts network.GroupedPortRanges,
) error {
	endpointsUnderActionSet := set.NewStrings()
	for endpoint := range openPorts {
		endpointsUnderActionSet.Add(endpoint)
	}
	for endpoint := range closePorts {
		endpointsUnderActionSet.Add(endpoint)
	}
	endpointsUnderAction := endpoints(endpointsUnderActionSet.Values())

	unitUUID := unitUUID{UUID: unit.String()}

	return domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		endpoints, err := st.ensureEndpoints(ctx, tx, unitUUID, endpointsUnderAction)
		if err != nil {
			return errors.Errorf("ensuring endpoints exist for unit %q: %w", unit, err)
		}

		currentUnitOpenedPorts, err := st.getUnitOpenedPorts(ctx, tx, unitUUID)
		if err != nil {
			return errors.Errorf("getting opened ports for unit %q: %w", unit, err)
		}

		err = st.openPorts(ctx, tx, openPorts, currentUnitOpenedPorts, endpoints)
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

// ensureEndpoints ensures that the given endpoints are present in the database.
// Return all endpoints under action with their corresponding UUIDs.
//
// TODO(jack-w-shaw): Once it has been implemented, we should verify new endpoints
// are valid by checking the charm_relation table.
func (st *State) ensureEndpoints(
	ctx context.Context, tx *sqlair.TX, unitUUID unitUUID, endpointsUnderAction endpoints,
) ([]endpoint, error) {
	getUnitEndpoints, err := st.Prepare(`
SELECT &endpoint.*
FROM unit_endpoint
WHERE unit_uuid = $unitUUID.unit_uuid
AND endpoint IN ($endpoints[:])
`, endpoint{}, unitUUID, endpointsUnderAction)
	if err != nil {
		return nil, errors.Errorf("preparing get unit endpoints statement: %w", err)
	}

	insertUnitEndpoint, err := st.Prepare("INSERT INTO unit_endpoint (*) VALUES ($unitEndpoint.*)", unitEndpoint{})
	if err != nil {
		return nil, errors.Errorf("preparing insert unit endpoint statement: %w", err)
	}

	var endpoints []endpoint
	err = tx.Query(ctx, getUnitEndpoints, unitUUID, endpointsUnderAction).GetAll(&endpoints)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, jujuerrors.Trace(err)
	}

	foundEndpoints := set.NewStrings()
	for _, ep := range endpoints {
		foundEndpoints.Add(ep.Endpoint)
	}

	// Insert any new endpoints that are required.
	requiredEndpoints := set.NewStrings(endpointsUnderAction...).Difference(foundEndpoints)
	newUnitEndpoints := make([]unitEndpoint, requiredEndpoints.Size())
	for i, requiredEndpoint := range requiredEndpoints.Values() {
		uuid, err := uuid.NewUUID()
		if err != nil {
			return nil, errors.Errorf("generating UUID for unit endpoint: %w", err)
		}
		newUnitEndpoints[i] = unitEndpoint{
			UUID:     uuid.String(),
			UnitUUID: unitUUID.UUID,
			Endpoint: requiredEndpoint,
		}
		endpoints = append(endpoints, endpoint{
			Endpoint: requiredEndpoint,
			UUID:     uuid.String(),
		})
	}

	if len(newUnitEndpoints) > 0 {
		err = tx.Query(ctx, insertUnitEndpoint, newUnitEndpoints).Run()
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
	}

	return endpoints, nil
}

// getUnitOpenedPorts returns the opened ports for the given unit.
//
// NOTE: This differs from GetUnitOpenedPorts in that it returns port ranges with
// their UUIDs, which are not needed by GetUnitOpenedPorts.
func (st *State) getUnitOpenedPorts(ctx context.Context, tx *sqlair.TX, unitUUID unitUUID) ([]endpointPortRangeUUID, error) {
	getOpenedPorts, err := st.Prepare(`
SELECT 
	port_range.uuid AS &endpointPortRangeUUID.uuid,
	protocol.protocol AS &endpointPortRangeUUID.protocol,
	port_range.from_port AS &endpointPortRangeUUID.from_port,
	port_range.to_port AS &endpointPortRangeUUID.to_port,
	unit_endpoint.endpoint AS &endpointPortRangeUUID.endpoint
FROM port_range
JOIN protocol ON port_range.protocol_id = protocol.id
JOIN unit_endpoint ON port_range.unit_endpoint_uuid = unit_endpoint.uuid
WHERE unit_endpoint.unit_uuid = $unitUUID.unit_uuid
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
		return nil, jujuerrors.Trace(err)
	}
	return openedPorts, nil
}

// openPorts inserts the given port ranges into the database, unless they're already open.
func (st *State) openPorts(
	ctx context.Context, tx *sqlair.TX,
	openPorts network.GroupedPortRanges, currentOpenedPorts []endpointPortRangeUUID, endpoints []endpoint,
) error {
	insertPortRange, err := st.Prepare("INSERT INTO port_range (*) VALUES ($unitPortRange.*)", unitPortRange{})
	if err != nil {
		return errors.Errorf("preparing insert port range statement: %w", err)
	}

	protocolMap, err := st.getProtocolMap(ctx, tx)
	if err != nil {
		return errors.Errorf("getting protocol map: %w", err)
	}

	// index the current opened ports by endpoint and port range
	currentOpenedPortRangeExistenceIndex := make(map[string]map[network.PortRange]bool)
	for _, openedPortRange := range currentOpenedPorts {
		if _, ok := currentOpenedPortRangeExistenceIndex[openedPortRange.Endpoint]; !ok {
			currentOpenedPortRangeExistenceIndex[openedPortRange.Endpoint] = make(map[network.PortRange]bool)
		}
		currentOpenedPortRangeExistenceIndex[openedPortRange.Endpoint][openedPortRange.decode()] = true
	}

	// Construct the new port ranges to open
	var openPortRanges []unitPortRange

	for _, ep := range endpoints {
		ports, ok := openPorts[ep.Endpoint]
		if !ok {
			continue
		}

		for _, portRange := range ports {
			// skip port range if it's already open on this endpoint
			if _, ok := currentOpenedPortRangeExistenceIndex[ep.Endpoint][portRange]; ok {
				continue
			}

			uuid, err := uuid.NewUUID()
			if err != nil {
				return errors.Errorf("generating UUID for port range: %w", err)
			}
			openPortRanges = append(openPortRanges, unitPortRange{
				UUID:             uuid.String(),
				ProtocolID:       protocolMap[portRange.Protocol],
				FromPort:         portRange.FromPort,
				ToPort:           portRange.ToPort,
				UnitEndpointUUID: ep.UUID,
			})
		}
	}

	if len(openPortRanges) > 0 {
		err = tx.Query(ctx, insertPortRange, openPortRanges).Run()
		if err != nil {
			return jujuerrors.Trace(err)
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
		return nil, jujuerrors.Trace(err)
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
			return jujuerrors.Trace(err)
		}
	}
	return nil
}

// GetEndpoints returns all the endpoints for the given unit
//
// TODO(jack-w-shaw): At the moment, we calculate this by checking the unit_endpoints
// table. However, this will not always return a complete list, as it only includes
// endpoints that have had ports opened on them at some point.
//
// Once it has been implemented, we should check the charm_relation table to get a
// complete list of endpoints instead.
func (st *State) GetEndpoints(ctx domain.AtomicContext, unit coreunit.UUID) ([]string, error) {
	unitUUID := unitUUID{UUID: unit.String()}

	getEndpoints, err := st.Prepare(`
SELECT &endpointName.*
FROM unit_endpoint
WHERE unit_endpoint.unit_uuid = $unitUUID.unit_uuid
`, endpointName{}, unitUUID)
	if err != nil {
		return nil, errors.Errorf("preparing get endpoints statement: %w", err)
	}

	var endpointNames []endpointName
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, getEndpoints, unitUUID).GetAll(&endpointNames)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return nil, errors.Errorf("getting endpoints for unit %q: %w", unit, err)
	}

	endpoints := make([]string, len(endpointNames))
	for i, ep := range endpointNames {
		endpoints[i] = ep.Endpoint
	}
	return endpoints, nil
}

func (st *State) getUnitUUID(ctx context.Context, tx *sqlair.TX, unitName string) (unitUUID, error) {
	unit := name{Name: unitName}
	var uuid unitUUID
	getUnitStmt, err := st.Prepare(`
SELECT uuid AS &unitUUID.unit_uuid 
FROM   unit 
WHERE  name = $name.name
`, unit, uuid)
	if err != nil {
		return unitUUID{}, errors.Errorf("preparing get unit uuid statement: %w", err)
	}

	err = tx.Query(ctx, getUnitStmt, unit).Get(&uuid)
	if errors.Is(err, sqlair.ErrNoRows) {
		return unitUUID{}, errors.Errorf("%w: %s", applicationerrors.UnitNotFound, unitName)
	}
	if err != nil {
		return unitUUID{}, errors.Errorf("querying unit %q: %w", unitName, err)
	}
	return uuid, nil
}
