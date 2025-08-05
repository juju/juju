// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/port"
	porterrors "github.com/juju/juju/domain/port/errors"
	"github.com/juju/juju/internal/errors"
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
	db, err := st.DB(ctx)
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
	db, err := s.DB(ctx)
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
	db, err := st.DB(ctx)
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
	db, err := st.DB(ctx)
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

// GetUnitUUID returns the UUID of the unit with the given name, returning an
// error satisfying [porterrors.UnitNotFound] if the unit does not exist.
func (st *State) GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error) {
	db, err := st.DB(ctx)
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
		return "", errors.Errorf("%s %w", name, porterrors.UnitNotFound)
	}
	if err != nil {
		return "", errors.Errorf("looking up unit UUID for %q: %w", name, err)
	}
	return u.UUID, errors.Capture(err)
}
