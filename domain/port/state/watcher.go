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
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/internal/errors"
)

// WatchOpenedPortsTable returns the name of the table that should be watched
func (st *State) WatchOpenedPortsTable() string {
	return "port_range"
}

// InitialWatchOpenedPortsStatement returns the query to load the initial event
// for the WatchOpenedPorts watcher
func (st *State) InitialWatchOpenedPortsStatement() string {
	return "SELECT name FROM machine"
}

// GetMachinesForEndpoints returns a slice of machine UUIDs that host the provided endpoints.
func (st *State) GetMachinesForEndpoints(ctx context.Context, eps []string) ([]coremachine.Name, error) {
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

// FilterEndpointsForApplication returns the subset of provided endpoint uuids
// that are associated with the provided application.
func (st *State) FilterEndpointsForApplication(ctx context.Context, app coreapplication.ID, eps []string) (set.Strings, error) {
	db, err := st.DB()
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}

	applicationUUID := applicationUUID{UUID: app.String()}
	endpointUUIDs := endpoints(eps)

	query, err := st.Prepare(`
SELECT unit_endpoint.uuid AS &endpointUUID.uuid
FROM unit
JOIN unit_endpoint ON unit.uuid = unit_endpoint.unit_uuid
WHERE unit_endpoint.uuid IN ($endpoints[:])
AND unit.application_uuid = $applicationUUID.application_uuid
`, endpointUUID{}, applicationUUID, endpointUUIDs)
	if err != nil {
		return nil, errors.Errorf("failed to prepare application for endpoint query: %w", err)
	}

	filteredEps := []endpointUUID{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, applicationUUID, endpointUUIDs).GetAll(&filteredEps)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return nil, errors.Errorf("failed to get applications for endpoints: %w", err)
	}

	return set.NewStrings(transform.Slice(filteredEps, func(e endpointUUID) string { return e.UUID })...), nil
}
