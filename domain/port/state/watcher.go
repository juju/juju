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
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/errors"
)

// NamespaceForWatchOpenedPort returns the name of the table that should be watched
func (*State) NamespaceForWatchOpenedPort() string {
	return "port_range"
}

// InitialWatchMachineOpenedPortsStatement returns the name of the table
// that should be watched and the query to load the
// initial event for the WatchMachineOpenedPorts watcher
func (*State) InitialWatchMachineOpenedPortsStatement() (string, string) {
	// It looks strange that we don't return the same namespace than the table
	// returned in the initial statement, but it is actually ok.
	// We want an event stream with machine names, but call site will compute
	// machine names from port_range event. It is why this looks weird.
	return "port_range", "SELECT name FROM machine"
}

// GetMachineNamesForUnits returns a slice of machine names that host the
// provided units.
func (st *State) GetMachineNamesForUnits(ctx context.Context, units []unit.UUID) ([]coremachine.Name, error) {
	db, err := st.DB()
	if err != nil {
		return nil, jujuerrors.Trace(err)
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
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return nil, errors.Errorf("failed to get machines for units: %w", err)
	}

	return transform.Slice(machineNames, func(m machineName) coremachine.Name { return m.Name }), nil
}

func (st *State) FilterUnitUUIDsForApplication(ctx context.Context, units []unit.UUID, app coreapplication.ID) (set.Strings, error) {
	db, err := st.DB()
	if err != nil {
		return nil, jujuerrors.Trace(err)
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
		return jujuerrors.Trace(err)
	})
	if err != nil {
		return nil, errors.Errorf("failed to get applications for units: %w", err)
	}

	return set.NewStrings(transform.Slice(filteredUnits, func(u unitUUID) string { return u.UUID.String() })...), nil
}
