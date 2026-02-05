// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/errors"
)

// NamespaceForWatchOpenedPort returns the name of the table that should be watched
func (*State) NamespaceForWatchOpenedPort() string {
	return "port_range"
}

// InitialWatchOpenedPortsStatement returns the name of the table
// that should be watched and the query to load the
// initial event for the WatchOpenedPorts watcher
func (*State) InitialWatchOpenedPortsStatement() (string, string) {
	// We only care about units that have an associate machine.
	return "port_range", `
SELECT u.uuid FROM unit AS u
JOIN machine AS m ON u.net_node_uuid = m.net_node_uuid
`
}

// FilterUnitUUIDsForApplication returns the subset of provided endpoint
// uuids that are associated with the provided application.
func (st *State) FilterUnitUUIDsForApplication(ctx context.Context, units []unit.UUID, app coreapplication.UUID) (set.Strings, error) {
	db, err := st.DB(ctx)
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
