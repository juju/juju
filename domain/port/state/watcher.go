// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	jujuerrors "github.com/juju/errors"

	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/internal/errors"
)

// WatchOpenedPortsTable returns the name of the table that should be watched
func (st *State) WatchOpenedPortsTable() string {
	return "port_range"
}

// InitialWatchMachineOpenedPortsStatement returns the query to load the initial event
// for the WatchOpenedPorts watcher
func (st *State) InitialWatchMachineOpenedPortsStatement() string {
	return "SELECT name FROM machine"
}

// InitialWatchApplicationOpenedPortsStatement returns the query to load the initial
// event for the WatchApplicationOpenedPorts watcher
func (st *State) InitialWatchApplicationOpenedPortsStatement() string {
	return "SELECT name FROM application"
}

// GetMachineNamesForUnitEndpoints returns a slice of machine names that host the provided endpoints.
func (st *State) GetMachineNamesForUnitEndpoints(ctx context.Context, eps []string) ([]coremachine.Name, error) {
	db, err := st.DB()
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}

	endpointUUIDs := endpoints(eps)

	query, err := st.Prepare(`
SELECT DISTINCT machine.name AS &machineName.name
FROM machine
JOIN unit ON machine.net_node_uuid = unit.net_node_uuid
JOIN unit_endpoint ON unit.uuid = unit_endpoint.unit_uuid
WHERE unit_endpoint.uuid IN ($endpoints[:])
`, machineName{}, endpointUUIDs)
	if err != nil {
		return nil, errors.Errorf("failed to prepare machine for endpoint query: %w", err)
	}

	machineNames := []machineName{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, endpointUUIDs).GetAll(&machineNames)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return nil, errors.Errorf("failed to get machines for endpoints: %w", err)
	}

	return transform.Slice(machineNames, func(m machineName) coremachine.Name { return m.Name }), nil
}

// GetApplicationNamesForUnitEndpoints returns a slice of application names that host
// the provided endpoints.
func (st *State) GetApplicationNamesForUnitEndpoints(ctx context.Context, eps []string) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}

	endpointUUIDs := endpoints(eps)

	query, err := st.Prepare(`
SELECT DISTINCT application.name AS &applicationName.name
FROM application
JOIN unit ON application.uuid = unit.application_uuid
JOIN unit_endpoint ON unit.uuid = unit_endpoint.unit_uuid
WHERE unit_endpoint.uuid IN ($endpoints[:])
`, applicationName{}, endpointUUIDs)
	if err != nil {
		return nil, errors.Errorf("failed to prepare application for endpoint query: %w", err)
	}

	applicationNames := []applicationName{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, endpointUUIDs).GetAll(&applicationNames)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return nil, errors.Errorf("failed to get applications for endpoints: %w", err)
	}

	return transform.Slice(applicationNames, func(a applicationName) string { return a.Name }), nil
}
