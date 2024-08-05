// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"reflect"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	internaldatabase "github.com/juju/juju/internal/database"
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
		return domain.CoerceError(err)
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	groupedPortRanges := network.GroupedPortRanges{}
	for _, endpointPortRange := range results {
		if _, ok := groupedPortRanges[endpointPortRange.Endpoint]; !ok {
			groupedPortRanges[endpointPortRange.Endpoint] = []network.PortRange{}
		}
		groupedPortRanges[endpointPortRange.Endpoint] = append(groupedPortRanges[endpointPortRange.Endpoint], decodeEndpointPortRange(endpointPortRange))
	}
	return groupedPortRanges, nil
}

// UpdateUnitPorts opens and closes ports for a given unit. Ports are grouped by endpoint.
//
// NOTE: This method opens ports first, and then closes ports. However, it is recommended
// that your service layer reconciles overlapping port ranges before calling this method.
func (st *State) UpdateUnitPorts(ctx context.Context, unitUUID string, openPorts, closePorts network.GroupedPortRanges) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	endpointsUnderAction := set.NewStrings()
	for endpoint := range openPorts {
		endpointsUnderAction.Add(endpoint)
	}
	for endpoint := range closePorts {
		endpointsUnderAction.Add(endpoint)
	}

	getProtocols, err := st.Prepare("SELECT &protocol.* FROM protocol", protocol{})
	if err != nil {
		return errors.Annotate(err, "preparing get protocol ID statement")
	}

	insertPortRange, err := st.Prepare("INSERT INTO port_range (*) VALUES ($unitPortRange.*)", unitPortRange{})
	if err != nil {
		return errors.Annotate(err, "preparing insert port range statement")
	}

	clearPortRange, err := st.Prepare(`
DELETE FROM port_range
WHERE unit_endpoint_uuid IN ($unitEndpointUUIDs[:])
`, unitEndpointUUIDs{})
	if err != nil {
		return errors.Annotate(err, "preparing clear port range statement")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {

		unitEndpointMap := map[string]unitEndpointUUID{}
		for _, endpoint := range endpointsUnderAction.Values() {
			var err error
			unitEndpointMap[endpoint], err = st.getUnitEndpointUUID(ctx, tx, unitUUID, endpoint)
			if errors.Is(err, UnitEndpointNotFound) {
				unitEndpointMap[endpoint], err = st.insertUnitEndpoint(ctx, tx, unitUUID, endpoint)
			}
			if err != nil {
				return errors.Trace(err)
			}
		}

		protocols := []protocol{}
		protocolMap := map[string]int{}
		err = tx.Query(ctx, getProtocols).GetAll(&protocols)
		if err != nil {
			return domain.CoerceError(err)
		}
		for _, protocol := range protocols {
			protocolMap[protocol.Name] = protocol.ID
		}

		unitPortRanges := []unitPortRange{}
		changedEndpoints := unitEndpointUUIDs{}

		for endpoint, unitEndpointUUID := range unitEndpointMap {
			currentOpenedPorts, err := st.getOpenedPorts(ctx, tx, unitEndpointUUID)
			if err != nil {
				return errors.Trace(err)
			}

			currentNetworkPorts := transform.Slice(currentOpenedPorts, decodePortRange)
			network.SortPortRanges(currentNetworkPorts)

			reconciledOpenPorts := reconcilePorts(currentNetworkPorts, openPorts[endpoint], closePorts[endpoint])
			network.SortPortRanges(reconciledOpenPorts)

			// Continue if there are no changes are to be made. We previously sorted so
			// port range permutations aren't picked up as changes
			if reflect.DeepEqual(currentNetworkPorts, reconciledOpenPorts) {
				continue
			}

			changedEndpoints = append(changedEndpoints, unitEndpointUUID.UUID)

			for _, portRange := range reconciledOpenPorts {
				unitPortRanges = append(unitPortRanges, unitPortRange{
					ProtocolID:       protocolMap[portRange.Protocol],
					FromPort:         portRange.FromPort,
					ToPort:           portRange.ToPort,
					UnitEndpointUUID: unitEndpointUUID.UUID,
				})
			}
		}

		// Clear out all existing open ports for this unit's endpoint
		err = tx.Query(ctx, clearPortRange, changedEndpoints).Run()
		if err != nil {
			return domain.CoerceError(err)
		}
		// Insert the freshly calculated port ranges
		err = tx.Query(ctx, insertPortRange, unitPortRanges).Run()
		if err != nil {
			return domain.CoerceError(err)
		}
		return nil
	})
}

// getOpenedPorts returns the opened ports for a given unit's endpoint.
func (st *State) getOpenedPorts(ctx context.Context, tx *sqlair.TX, unitEndpointUUID unitEndpointUUID) ([]portRange, error) {
	var result []portRange

	selectOpenedPorts, err := st.Prepare(`
SELECT &portRange.*
FROM port_range
JOIN protocol ON port_range.protocol_id = protocol.id
WHERE unit_endpoint_uuid = $unitEndpointUUID.uuid
`, portRange{}, unitEndpointUUID)

	if err != nil {
		return nil, errors.Annotate(err, "preparing select opened ports statement")
	}

	err = tx.Query(ctx, selectOpenedPorts, unitEndpointUUID).GetAll(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return []portRange{}, nil
	}
	return result, domain.CoerceError(err)
}

// getUnitEndpointUUID returns the UUID of the specified unit's endpoint.
// If the unit_endpoint does not exist, UnitEndpointNotFound is returned.
func (st *State) getUnitEndpointUUID(ctx context.Context, tx *sqlair.TX, unitUUID, endpoint string) (unitEndpointUUID, error) {
	var result unitEndpointUUID
	unitUUIDEndpoint := unitUUIDEndpoint{
		UUID:     unitUUID,
		Endpoint: endpoint,
	}

	selectUnitEndpoint, err := st.Prepare(`
SELECT &unitEndpointUUID.*
FROM unit_endpoint
WHERE unit_uuid = $unitUUIDEndpoint.unit_uuid 
AND endpoint = $unitUUIDEndpoint.endpoint
`, result, unitUUIDEndpoint)

	if err != nil {
		return result, errors.Annotate(err, "preparing select unit endpoint statement")
	}

	err = tx.Query(ctx, selectUnitEndpoint, unitUUIDEndpoint).Get(&result)
	if internaldatabase.IsErrNotFound(err) {
		return result, errors.Annotatef(UnitEndpointNotFound, "unit %q endpoint %q", unitUUID, endpoint)
	}
	if err != nil {
		return result, domain.CoerceError(err)
	}
	return result, nil
}

// insertUnitEndpoint inserts a new unit_endpoint row into DQLite, corresponding
// to the specified unit's endpoint.
func (st *State) insertUnitEndpoint(ctx context.Context, tx *sqlair.TX, unitUUID, endpoint string) (unitEndpointUUID, error) {
	// TODO(jack-w-shaw): Verify that the endpoint is valid before inserting it.
	// Once the charm domain has been completed, the table "charm_relation" will be
	// populated with the charms endpoints in the "name" column.
	//
	// Use the unitUUID to select the application's non-requirer relations & check
	// endpoint is included.

	uuid, err := uuid.NewUUID()
	if err != nil {
		return unitEndpointUUID{}, errors.Annotatef(err, "generating UUID for unit endpoint")
	}

	unitEndpoint := unitEndpoint{
		UUID:     uuid.String(),
		UnitUUID: unitUUID,
		Endpoint: endpoint,
	}
	insertUnitEndpoint, err := st.Prepare("INSERT INTO unit_endpoint (*) VALUES ($unitEndpoint.*)", unitEndpoint)
	if err != nil {
		return unitEndpointUUID{}, errors.Annotate(err, "preparing insert unit endpoint statement")
	}
	err = tx.Query(ctx, insertUnitEndpoint, unitEndpoint).Run()

	return unitEndpointUUID{UUID: uuid.String()}, domain.CoerceError(err)
}

func decodePortRange(p portRange) network.PortRange {
	return network.PortRange{
		Protocol: p.Protocol,
		FromPort: p.FromPort,
		ToPort:   p.ToPort,
	}
}

func decodeEndpointPortRange(p endpointPortRange) network.PortRange {
	return network.PortRange{
		Protocol: p.Protocol,
		FromPort: p.FromPort,
		ToPort:   p.ToPort,
	}
}
