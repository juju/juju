// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/port"
	porterrors "github.com/juju/juju/domain/port/errors"
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
		return nil, errors.Capture(err)
	}

	unitUUID := unitUUID{UUID: unit}

	query, err := st.Prepare(`
SELECT &endpointPortRange.*
FROM v_port_range
WHERE unit_uuid = $unitUUID.unit_uuid
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
		return errors.Capture(err)
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

// GetAllOpenedPorts returns the opened ports in the model, grouped by unit name.
//
// NOTE: We do not group by endpoint here. It is not needed. Instead, we just
// group by unit name
func (s *State) GetAllOpenedPorts(ctx context.Context) (port.UnitGroupedPortRanges, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query, err := s.Prepare(`
SELECT DISTINCT &unitNamePortRange.*
FROM v_port_range
JOIN unit ON unit_uuid = unit.uuid
`, unitNamePortRange{})
	if err != nil {
		return nil, errors.Errorf("preparing get all opened ports statement: %w", err)
	}

	results := []unitNamePortRange{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Errorf("getting all opened ports: %w", err)
	}

	groupedPortRanges := port.UnitGroupedPortRanges{}
	for _, portRange := range results {
		unitName := portRange.UnitName
		groupedPortRanges[unitName] = append(groupedPortRanges[unitName], portRange.decode())
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
func (st *State) GetMachineOpenedPorts(ctx context.Context, machine string) (map[coreunit.Name]network.GroupedPortRanges, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	machineUUID := machineUUID{UUID: machine}

	query, err := st.Prepare(`
SELECT &unitEndpointPortRange.*
FROM v_port_range
JOIN unit ON unit_uuid = unit.uuid
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
		return errors.Capture(err)
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
		return nil, errors.Capture(err)
	}

	applicationUUID := applicationUUID{UUID: application}

	query, err := st.Prepare(`
SELECT &unitEndpointPortRange.*
FROM v_port_range
JOIN unit ON unit_uuid = unit.uuid
WHERE application_uuid = $applicationUUID.application_uuid
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
		return errors.Capture(err)
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
	unitUUID := unitUUID{UUID: unit}

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
	err = domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, getOpenedPorts, unitUUID).GetAll(&portRanges)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Errorf("getting opened ports for colocated units with %q: %w", unit, err)
	}

	ret := transform.Slice(portRanges, portRange.decode)
	network.SortPortRanges(ret)
	return ret, nil
}

// GetEndpointOpenedPorts returns the opened ports for a given endpoint of a
// given unit.
func (st *State) GetEndpointOpenedPorts(ctx domain.AtomicContext, unit coreunit.UUID, endpoint string) ([]network.PortRange, error) {
	unitUUID := unitUUID{UUID: unit}
	endpointName := endpointName{Endpoint: endpoint}

	query, err := st.Prepare(`
SELECT &portRange.*
FROM v_port_range
WHERE unit_uuid = $unitUUID.unit_uuid
AND endpoint = $endpointName.endpoint
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
		return errors.Capture(err)
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
	endpointsUnderActionSet.Remove(port.WildcardEndpoint)
	endpointsUnderAction := endpoints(endpointsUnderActionSet.Values())

	unitUUID := unitUUID{UUID: unit}

	return domain.Run(ctx, func(ctx context.Context, tx *sqlair.TX) error {
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

func (st *State) lookupRelationUUIDs(
	ctx context.Context, tx *sqlair.TX, unitUUID unitUUID, endpointsUnderAction endpoints,
) ([]endpoint, error) {
	getEndpoints, err := st.Prepare(`
SELECT &endpoint.*
FROM v_endpoint
WHERE unit_uuid = $unitUUID.unit_uuid
AND endpoint IN ($endpoints[:])
`, endpoint{}, unitUUID, endpointsUnderAction)
	if err != nil {
		return nil, errors.Errorf("preparing get endpoints statement: %w", err)
	}

	endpoints := []endpoint{}
	err = tx.Query(ctx, getEndpoints, unitUUID, endpointsUnderAction).GetAll(&endpoints)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	if len(endpoints) != len(endpointsUnderAction) {
		endpointsSet := set.NewStrings([]string(endpointsUnderAction)...)
		for _, ep := range endpoints {
			endpointsSet.Remove(ep.Endpoint)
		}
		return nil, errors.Errorf(
			"%w; %v does exist on unit %v",
			porterrors.InvalidEndpoint, endpointsSet.Values(), unitUUID.UUID,
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
			if ep != port.WildcardEndpoint {
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

// GetEndpoints returns all the valid relation endpoints for a given unit. This
// does not include the special wildcard endpoint.
func (st *State) GetEndpoints(ctx domain.AtomicContext, unit coreunit.UUID) ([]string, error) {
	unitUUID := unitUUID{UUID: unit}

	getEndpoints, err := st.Prepare(`
SELECT &endpointName.*
FROM v_endpoint
WHERE unit_uuid = $unitUUID.unit_uuid
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
		return errors.Capture(err)
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

// GetUnitUUID returns the UUID of the unit with the given name, returning an
// error satisfying [porterrors.UnitNotFound] if the unit does not exist.
func (st *State) GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var unitUUID coreunit.UUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		unitUUID, err = st.getUnitUUID(ctx, tx, unitName.String())
		return errors.Capture(err)
	})
	return unitUUID, errors.Capture(err)
}

func (st *State) getUnitUUID(ctx context.Context, tx *sqlair.TX, name string) (coreunit.UUID, error) {
	u := unitName{Name: name}

	selectUnitUUIDStmt, err := st.Prepare(`
SELECT &unitName.uuid
FROM unit
WHERE name=$unitName.name`, u)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = tx.Query(ctx, selectUnitUUIDStmt, u).Get(&u)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", fmt.Errorf("%s %w", name, porterrors.UnitNotFound)
	}
	if err != nil {
		return "", errors.Errorf("looking up unit UUID for %q: %w", name, err)
	}
	return u.UUID, errors.Capture(err)
}
