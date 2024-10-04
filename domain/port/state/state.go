// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	jujuerrors "github.com/juju/errors"

	"github.com/juju/juju/core/application"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/port"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// State represents the persistence layer for opened ports.
type State struct {
	*domain.StateBase
}

// NewState returns a new state reference.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetUnitOpenedPorts returns the opened ports for a given unit uuid,
// grouped by endpoint.
func (st *State) GetUnitOpenedPorts(ctx context.Context, unit unit.UUID) (network.GroupedPortRanges, error) {
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

// GetUnitOpenedPortsWithUUIDs returns the opened ports for the given unit with the
// UUID of the port range. The opened ports are grouped by endpoint.
func (st *State) GetUnitOpenedPortsWithUUIDs(ctx domain.AtomicContext, unit unit.UUID) (map[string][]port.PortRangeWithUUID, error) {
	unitUUID := unitUUID{UUID: unit.String()}
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

	openedPorts := []endpointPortRangeUUID{}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, getOpenedPorts, unitUUID).GetAll(&openedPorts)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}

	ret := map[string][]port.PortRangeWithUUID{}
	for _, openedPort := range openedPorts {
		if _, ok := ret[openedPort.Endpoint]; !ok {
			ret[openedPort.Endpoint] = []port.PortRangeWithUUID{}
		}
		ret[openedPort.Endpoint] = append(ret[openedPort.Endpoint], port.PortRangeWithUUID{
			UUID:      port.UUID(openedPort.UUID),
			PortRange: openedPort.decode(),
		})
	}
	return ret, nil
}

// GetMachineOpenedPorts returns the opened ports for all the units on the given
// machine. Opened ports are grouped first by unit and then by endpoint.
//
// NOTE: In the ddl machines and units both share 1-to-1 relations with net_nodes.
// So to join units to machines we go via their net_nodes.
func (st *State) GetMachineOpenedPorts(ctx context.Context, machine string) (map[unit.UUID]network.GroupedPortRanges, error) {
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
func (st *State) GetApplicationOpenedPorts(ctx context.Context, application application.ID) (port.UnitEndpointPortRanges, error) {
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
// Port ranges are NOT grouped
func (st *State) GetColocatedOpenedPorts(ctx domain.AtomicContext, unit unit.UUID) ([]network.PortRange, error) {
	unitUUID := unitUUID{UUID: unit.String()}

	getOpenedPorts, err := st.Prepare(`
SELECT DISTINCT &portRange.*
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

// AddOpenedPorts adds the given port ranges to the database. Port ranges must
// be grouped by endpoint UUID.
func (st *State) AddOpenedPorts(ctx domain.AtomicContext, portRangesByEndpointUUID map[port.UUID][]network.PortRange) error {
	insertPortRange, err := st.Prepare("INSERT INTO port_range (*) VALUES ($unitPortRange.*)", unitPortRange{})
	if err != nil {
		return errors.Errorf("preparing insert port range statement: %w", err)
	}

	protocolMap, err := st.getProtocolMap(ctx)
	if err != nil {
		return errors.Errorf("getting protocol map: %w", err)
	}

	// Construct the new port ranges to open
	var openPortRanges []unitPortRange

	for endpointUUID, portRanges := range portRangesByEndpointUUID {
		for _, portRange := range portRanges {
			portRangeUUID, err := port.NewUUID()
			if err != nil {
				return errors.Errorf("generating UUID for port range: %w", err)
			}
			openPortRanges = append(openPortRanges, unitPortRange{
				UUID:             portRangeUUID.String(),
				ProtocolID:       protocolMap[portRange.Protocol],
				FromPort:         portRange.FromPort,
				ToPort:           portRange.ToPort,
				UnitEndpointUUID: endpointUUID.String(),
			})
		}
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, insertPortRange, openPortRanges).Run()
	})
	if err != nil {
		return jujuerrors.Trace(err)
	}

	return nil
}

// getProtocolMap returns a map of protocol names to their IDs in DQLite.
func (st *State) getProtocolMap(ctx domain.AtomicContext) (map[string]int, error) {
	getProtocols, err := st.Prepare("SELECT &protocol.* FROM protocol", protocol{})
	if err != nil {
		return nil, errors.Errorf("preparing get protocol ID statement: %w", err)
	}

	protocols := []protocol{}
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, getProtocols).GetAll(&protocols)
	})
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}

	protocolMap := map[string]int{}
	for _, protocol := range protocols {
		protocolMap[protocol.Name] = protocol.ID
	}

	return protocolMap, nil
}

// RemoveOpenedPorts removes the given port ranges from the database by uuid.
func (st *State) RemoveOpenedPorts(ctx domain.AtomicContext, uuids []port.UUID) error {
	portRangeUUIDs := portRangeUUIDs(transform.Slice(uuids, func(u port.UUID) string { return u.String() }))

	closePortRanges, err := st.Prepare(`
DELETE FROM port_range
WHERE uuid IN ($portRangeUUIDs[:])
`, portRangeUUIDs)
	if err != nil {
		return errors.Errorf("preparing remove opened ports statement: %w", err)
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, closePortRanges, portRangeUUIDs).Run()
	})
	return jujuerrors.Trace(err)
}

// GetEndpoints returns all the endpoints for the given unit
//
// TODO(jack-w-shaw): At the moment, we calculate this by checking the unit_endpoints
// table. However, this will not always return a complete list, as it only includes
// endpoints that have had ports opened on them at some point.
//
// Once it has been implemented, we should check the charm_relation table to get a
// complete list of endpoints instead.
func (st *State) GetEndpoints(ctx domain.AtomicContext, unit unit.UUID) ([]port.Endpoint, error) {
	unitUUID := unitUUID{UUID: unit.String()}

	getEndpoints, err := st.Prepare(`
SELECT &endpoint.*
FROM unit_endpoint
WHERE unit_endpoint.unit_uuid = $unitUUID.unit_uuid
`, endpoint{}, unitUUID)
	if err != nil {
		return nil, errors.Errorf("preparing get endpoints statement: %w", err)
	}

	var endpoints []endpoint
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, getEndpoints, unitUUID).GetAll(&endpoints)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return nil, errors.Errorf("getting endpoints for unit %q: %w", unit, err)
	}

	return transform.Slice(endpoints, func(e endpoint) port.Endpoint { return e.decode() }), nil
}

// AddEndpoints adds the endpoints to a given unit. Return the added endpoints
// with their corresponding UUIDs.
func (st *State) AddEndpoints(ctx domain.AtomicContext, unit unit.UUID, endpoints []string) ([]port.Endpoint, error) {
	if len(endpoints) == 0 {
		return nil, nil
	}

	unitUUID := unitUUID{UUID: unit.String()}

	insertUnitEndpoint, err := st.Prepare("INSERT INTO unit_endpoint (*) VALUES ($unitEndpoint.*)", unitEndpoint{})
	if err != nil {
		return nil, errors.Errorf("preparing insert unit endpoint statement: %w", err)
	}

	newEndpoints := make([]unitEndpoint, len(endpoints))
	for i, endpoint := range endpoints {
		uuid, err := port.NewUUID()
		if err != nil {
			return nil, errors.Errorf("generating UUID for unit endpoint: %w", err)
		}
		newEndpoints[i] = unitEndpoint{
			UUID:     uuid.String(),
			UnitUUID: unitUUID.UUID,
			Endpoint: endpoint,
		}
	}

	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, insertUnitEndpoint, newEndpoints).Run()
		if database.IsErrConstraintUnique(err) {
			return ErrUnitEndpointConflict
		}
		return err
	})
	if err != nil {
		return nil, errors.Errorf("adding endpoints to unit %q: %w", unit, err)
	}

	return transform.Slice(newEndpoints, func(e unitEndpoint) port.Endpoint { return e.decode() }), nil
}
