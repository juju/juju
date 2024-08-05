// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
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

// GetOpenedPorts returns the opened ports for a given unit uuid,
// grouped by endpoint.
func (st *State) GetOpenedPorts(ctx context.Context, unit string) (network.GroupedPortRanges, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	unitUUID := unitUUID{UUID: unit}

	query, err := st.Prepare(`
SELECT &endpointPortRange.*
FROM port_range
JOIN protocol ON port_range.protocol_id = protocol.id
JOIN unit_endpoint ON port_range.unit_endpoint_uuid = unit_endpoint.uuid
WHERE unit_endpoint.unit_uuid = $unitUUID.unit_uuid
`, endpointPortRange{}, unitUUID)
	if err != nil {
		return nil, errors.Annotate(err, "preparing get opened ports statement")
	}

	results := []endpointPortRange{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, unitUUID).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Annotatef(err, "getting opened ports for unit %q", unit)
	}

	groupedPortRanges := network.GroupedPortRanges{}
	for _, endpointPortRange := range results {
		endpointName := endpointPortRange.Endpoint
		if _, ok := groupedPortRanges[endpointName]; !ok {
			groupedPortRanges[endpointPortRange.Endpoint] = []network.PortRange{}
		}
		groupedPortRanges[endpointName] = groupedPortRanges[endpointName].Add(endpointPortRange.decode())
	}
	return groupedPortRanges, nil
}

// UpdateUnitPorts opens and closes ports for the endpoints of a given unit.
// The service layer must ensure that opened and closed ports for the same
// endpoints must not conflict.
func (st *State) UpdateUnitPorts(
	ctx context.Context, unit string, openPorts, closePorts network.GroupedPortRanges,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	endpointsUnderActionSet := set.NewStrings()
	for endpoint := range openPorts {
		endpointsUnderActionSet.Add(endpoint)
	}
	for endpoint := range closePorts {
		endpointsUnderActionSet.Add(endpoint)
	}
	endpointsUnderAction := endpoints(endpointsUnderActionSet.Values())

	unitUUID := unitUUID{UUID: unit}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {

		endpoints, err := st.ensureEndpoints(ctx, tx, unitUUID, endpointsUnderAction)
		if err != nil {
			return errors.Annotatef(err, "ensuring endpoints exist for unit %q", unit)
		}

		protocolMap, err := st.getProtocolMap(ctx, tx)
		if err != nil {
			return errors.Annotate(err, "getting protocol map")
		}

		endpointToPorts, err := st.getOpenedPortsForEndpoints(ctx, tx, unitUUID, endpointsUnderAction)
		if err != nil {
			return errors.Annotatef(err, "getting opened ports for unit %q", unit)
		}

		var (
			// changedEndpoints is a list of endpoint UUIDs that have had their
			// opened ports changed.
			changedEndpoints endpointUUIDs

			// unitPortRanges is a list of port ranges for each endpoint that
			// have had their opened ports changed.
			unitPortRanges []unitPortRange
		)
		for _, endpoint := range endpoints {
			endpointName := endpoint.Endpoint
			currentOpenedPorts := endpointToPorts[endpointName]

			reconciledOpenPorts := currentOpenedPorts.Update(openPorts[endpointName], closePorts[endpointName])

			if !currentOpenedPorts.EqualTo(reconciledOpenPorts) {
				changedEndpoints = append(changedEndpoints, endpoint.UUID)

				endpointPortRanges := transform.Slice(reconciledOpenPorts, func(p network.PortRange) unitPortRange {
					return unitPortRange{
						ProtocolID:       protocolMap[p.Protocol],
						FromPort:         p.FromPort,
						ToPort:           p.ToPort,
						UnitEndpointUUID: endpoint.UUID,
					}
				})
				unitPortRanges = append(unitPortRanges, endpointPortRanges...)
			}
		}

		err = st.updateEndpointOpenedPorts(ctx, tx, changedEndpoints, unitPortRanges)
		if err != nil {
			return errors.Annotatef(err, "updating opened ports for unit %q", unit)
		}

		return nil
	})
}

// ensureEndpoints ensures that the given endpoints are present in the database.
// Return all endpoints under action with their corresponding UUIDs.
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
		return nil, errors.Annotate(err, "preparing get unit endpoints statement")
	}

	insertUnitEndpoint, err := st.Prepare("INSERT INTO unit_endpoint (*) VALUES ($unitEndpoint.*)", unitEndpoint{})
	if err != nil {
		return nil, errors.Annotate(err, "preparing insert unit endpoint statement")
	}

	var endpoints []endpoint
	err = tx.Query(ctx, getUnitEndpoints, unitUUID, endpointsUnderAction).GetAll(&endpoints)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Trace(err)
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
			return nil, errors.Annotatef(err, "generating UUID for unit endpoint")
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
			return nil, errors.Trace(err)
		}
	}

	return endpoints, nil
}

// getProtocolMap returns a map of protocol names to their IDs in DQLite.
func (st *State) getProtocolMap(ctx context.Context, tx *sqlair.TX) (map[string]int, error) {
	getProtocols, err := st.Prepare("SELECT &protocol.* FROM protocol", protocol{})
	if err != nil {
		return nil, errors.Annotate(err, "preparing get protocol ID statement")
	}

	protocols := []protocol{}
	err = tx.Query(ctx, getProtocols).GetAll(&protocols)
	if err != nil {
		return nil, errors.Trace(err)
	}

	protocolMap := map[string]int{}
	for _, protocol := range protocols {
		protocolMap[protocol.Name] = protocol.ID
	}

	return protocolMap, nil
}

// getOpenedPortsForEndpoints returns the opened ports for the given endpoints
// of a given unit.
func (st *State) getOpenedPortsForEndpoints(
	ctx context.Context, tx *sqlair.TX, unitUUID unitUUID, endpoints endpoints,
) (network.GroupedPortRanges, error) {
	getOpenedPortsOnEndpoints, err := st.Prepare(`
SELECT &endpointPortRange.*
FROM port_range
JOIN protocol ON port_range.protocol_id = protocol.id
JOIN unit_endpoint ON port_range.unit_endpoint_uuid = unit_endpoint.uuid
WHERE unit_endpoint.unit_uuid = $unitUUID.unit_uuid
AND unit_endpoint.endpoint IN ($endpoints[:])
`, endpointPortRange{}, unitUUID, endpoints)
	if err != nil {
		return nil, errors.Annotate(err, "preparing get opened ports on endpoints statement")
	}

	var currentOpenedEndpointPorts []endpointPortRange
	err = tx.Query(ctx, getOpenedPortsOnEndpoints, unitUUID, endpoints).GetAll(&currentOpenedEndpointPorts)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Trace(err)
	}

	endpointToPorts := make(network.GroupedPortRanges)
	for _, r := range currentOpenedEndpointPorts {
		endpointToPorts[r.Endpoint] = endpointToPorts[r.Endpoint].Add(r.decode())
	}

	return endpointToPorts, nil
}

// updateEndpointOpenedPorts updates the opened ports for the given endpoints.
func (st *State) updateEndpointOpenedPorts(
	ctx context.Context, tx *sqlair.TX, endpointUUIDs endpointUUIDs, unitPortRanges []unitPortRange,
) error {
	insertPortRange, err := st.Prepare("INSERT INTO port_range (*) VALUES ($unitPortRange.*)", unitPortRange{})
	if err != nil {
		return errors.Annotate(err, "preparing insert port range statement")
	}

	clearPortRange, err := st.Prepare(`
DELETE FROM port_range
WHERE unit_endpoint_uuid IN ($endpointUUIDs[:])
`, endpointUUIDs)
	if err != nil {
		return errors.Annotate(err, "preparing clear port range statement")
	}

	// Clear out all existing open ports for endpoints undergoing changes.
	if len(endpointUUIDs) > 0 {
		err := tx.Query(ctx, clearPortRange, endpointUUIDs).Run()
		if err != nil {
			return errors.Trace(err)
		}
	}

	// Insert the freshly calculated port ranges
	if len(unitPortRanges) > 0 {
		err := tx.Query(ctx, insertPortRange, unitPortRanges).Run()
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
