// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	porterrors "github.com/juju/juju/domain/port/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// ImportOpenUnitPorts opens ports for the endpoints of a given unit during
// migration.  There can be no conflicts as no other ports for this give
// unit exist.
func (st *State) ImportOpenUnitPorts(
	ctx context.Context, unit coreunit.UUID, openPorts network.GroupedPortRanges,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	unitUUID := unitUUID{UUID: unit}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		endpointsUnderActionSet := set.NewStrings()
		for endpoint := range openPorts {
			endpointsUnderActionSet.Add(endpoint)
		}
		endpointsUnderActionSet.Remove(network.WildcardEndpoint)
		endpointsUnderAction := endpoints(endpointsUnderActionSet.Values())

		endpoints, err := st.lookupCharmRelationUUIDs(ctx, tx, unitUUID, endpointsUnderAction)
		if err != nil {
			return errors.Errorf("looking up charm relation endpoint uuids for unit %q: %w", unit, err)
		}

		err = st.openPorts(ctx, tx, openPorts, unitUUID, endpoints)
		if err != nil {
			return errors.Errorf("opening ports for unit %q: %w", unit, err)
		}

		return nil
	})
}

// lookupCharmRelationUUIDs returns the UUIDs of the given charm relations,
// aka endpoints, on a given unit. We return a slice of `endpoint` structs,
// allowing us to identify which endpoint uuid is which, as order is not
// guaranteed.
func (st *State) lookupCharmRelationUUIDs(
	ctx context.Context, tx *sqlair.TX, unitUUID unitUUID, endpointNames endpoints,
) ([]endpoint, error) {
	getEndpoints, err := st.Prepare(`
SELECT &endpoint.*
FROM   v_endpoint
WHERE  unit_uuid = $unitUUID.unit_uuid
AND    endpoint IN ($endpoints[:])
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

// openPorts inserts the given port ranges into the database, unless they're already open.
func (st *State) openPorts(
	ctx context.Context, tx *sqlair.TX,
	openPorts network.GroupedPortRanges,
	unitUUID unitUUID,
	endpoints []endpoint,
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

	for ep, ports := range openPorts {
		for _, portRange := range ports {
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
	getProtocols, err := st.Prepare("SELECT &protocol.* FROM protocol WHERE id >= 0", protocol{})
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
