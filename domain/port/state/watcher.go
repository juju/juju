// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
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

// GetMachineNamesForUnits returns a slice of machine names that host the provided units.
func (st *State) GetMachineNamesForUnits(ctx context.Context, units []unit.UUID) ([]coremachine.Name, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	unitUUIDs := unitUUIDs(units)

	query, err := st.Prepare(`
SELECT DISTINCT machine.name AS &machineName.name
FROM machine
JOIN unit ON machine.net_node_uuid = unit.net_node_uuid
WHERE unit.uuid IN ($unitUUIDs[:])
`, machineName{}, unitUUIDs)
	if err != nil {
		return nil, errors.Errorf("failed to prepare machine for unit query: %w", err)
	}

	machineNames := []machineName{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, unitUUIDs).GetAll(&machineNames)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Errorf("failed to get machines for units: %w", err)
	}

	return transform.Slice(machineNames, func(m machineName) coremachine.Name { return m.Name }), nil
}

func (st *State) FilterUnitUUIDsForApplication(ctx context.Context, units []unit.UUID, app coreapplication.ID) (set.Strings, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	applicationUUID := applicationUUID{UUID: app}
	unitUUIDs := unitUUIDs(units)

	query, err := st.Prepare(`
SELECT uuid AS &unitUUID.unit_uuid
FROM unit
WHERE unit.uuid IN ($unitUUIDs[:])
AND unit.application_uuid = $applicationUUID.application_uuid
`, unitUUID{}, applicationUUID, unitUUIDs)
	if err != nil {
		return nil, errors.Errorf("failed to prepare application for unit query: %w", err)
	}

	filteredUnits := []unitUUID{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, applicationUUID, unitUUIDs).GetAll(&filteredUnits)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Errorf("failed to get applications for units: %w", err)
	}

	return set.NewStrings(transform.Slice(filteredUnits, func(u unitUUID) string { return u.UUID.String() })...), nil
}
