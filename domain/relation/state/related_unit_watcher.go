// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/errors"
)

// InitialWatchRelatedUnits returns namespaces, initial query and mappers to
// watch related units.
func (st *State) InitialWatchRelatedUnits(
	ctx context.Context, unitUUID string, relationUUID string,
) ([]string, eventsource.NamespaceQuery, eventsource.Mapper, error) {
	const (
		relationUnitNamespace               = "relation_unit"
		relationApplicationSettingNamespace = "relation_application_settings_hash"
		relationUnitSettingNamespace        = "relation_unit_settings_hash"
	)

	// Get a map of application UUIDs by relation endpoint UUID.
	// This is used to determine which application UUID to emit for application
	// settings changes.
	// We can use the number of endpoints as a proxy for whether this is a peer
	// relation; peer relations have only one.
	// For peer relations, we watch the app settings for the input unit's side
	// of the relation. Otherwise, it's the other side.
	// These values never change over the lifetime of the relation,
	// so we can let these be closed over by the functions below.
	appByEndpoint, err := st.getRelationAppEndpoints(ctx, relationUUID)
	if err != nil {
		return nil, nil, nil, errors.Errorf("getting endpoints for relation %q: %w", relationUUID, err)
	}

	return []string{relationApplicationSettingNamespace, relationUnitSettingNamespace, relationUnitNamespace},
		// Initial query.
		func(ctx context.Context, _ database.TxnRunner) ([]string, error) {
			units, err := st.getUnitsInRelation(ctx, relationUUID)
			if err != nil {
				return nil, errors.Errorf("fetching units for relation %q: %w", relationUUID, err)
			}

			// Exclude the input unit from the list of related units.
			otherUnits := make([]string, 0, len(units)-1)
			for _, u := range units {
				if u.UnitUUID != unitUUID {
					otherUnits = append(otherUnits, relation.EncodeUnitUUID(u.UnitUUID))
				}
			}
			return otherUnits, nil
		},
		// Mapper.
		func(ctx context.Context, events []changestream.ChangeEvent) ([]string, error) {
			unitsInRelation, err := st.getUnitsInRelation(ctx, relationUUID)
			if err != nil {
				return nil, errors.Errorf("fetching units for relation %q: %w", relationUUID, err)
			}

			// Populate data structures for convenient lookups.
			// Exclude the input unit from the list of related units.
			// Determine the endpoint to watch for application settings changes.
			var localEndpointUUID string
			endpointUUIDs := set.NewStrings()
			unitByRelationUnit := make(map[string]string)
			relatedUnits := set.NewStrings()
			for _, u := range unitsInRelation {
				endpointUUIDs.Add(u.RelationEndpointUUID)

				if u.UnitUUID == unitUUID {
					localEndpointUUID = u.RelationEndpointUUID
					continue
				}

				if u.RelationUnitUUID != "" {
					unitByRelationUnit[u.RelationUnitUUID] = u.UnitUUID
				}

				relatedUnits.Add(u.UnitUUID)
			}
			// If this is a peer relation, we will have only one endpointUUID,
			// which will be the one.
			// Else, we will have 2 endpoints, and we will want the remote one.
			var endpointForAppSettingsChange string
			if endpointUUIDs.Size() > 1 {
				endpointUUIDs.Remove(localEndpointUUID)
			}
			if endpointUUIDs.Size() == 1 {
				endpointForAppSettingsChange = endpointUUIDs.Values()[0]
			} else {
				return nil, errors.Errorf(
					"programming error: we should have 1 endpoint to watch at this point, but have %d for relation %q",
					endpointUUIDs.Size(), relationUUID)
			}

			var out []string
			for _, event := range events {
				switch event.Namespace() {
				case relationUnitNamespace:
					// Discard events that are not from related units.
					if !relatedUnits.Contains(event.Changed()) {
						continue
					}
					event = newUnitUUIDEvent(event, event.Changed())
				case relationUnitSettingNamespace:
					// Discard events that are not from related units.
					unitUUID, ok := unitByRelationUnit[event.Changed()]
					if !ok {
						continue
					}
					event = newUnitUUIDEvent(event, unitUUID)
				case relationApplicationSettingNamespace:
					// Discard events that are not from the expected endpoint.
					if event.Changed() != endpointForAppSettingsChange {
						continue
					}
					appUUID, ok := appByEndpoint[endpointForAppSettingsChange]
					if !ok {
						// This should be impossible.
						return nil, errors.Errorf("no application UUID found for endpoint %q in relation %q",
							endpointForAppSettingsChange, relationUUID)
					}
					event = newApplicationUUIDEvent(event, appUUID)
				default:
					st.logger.Warningf(ctx, "watching related unit: unexpected namespace %q", event.Namespace())
					continue
				}
				out = append(out, event.Changed())
			}

			return out, errors.Capture(err)
		},
		// Error
		nil
}

// maskedEvent is a struct that wraps a change
// value and a custom UUID encoding function.
type maskedEvent struct {
	changestream.ChangeEvent

	change     string
	encodeUUID func(string) string
}

// Changed returns the change value with encoding applied.
func (e maskedEvent) Changed() string {
	return e.encodeUUID(e.change)
}

// newApplicationUUIDEvent creates a maskedEvent for application UUIDs,
// encoding the UUID with a custom function.
func newApplicationUUIDEvent(event changestream.ChangeEvent, change string) maskedEvent {
	return maskedEvent{
		ChangeEvent: event,
		change:      change,
		encodeUUID:  relation.EncodeApplicationUUID,
	}
}

// newApplicationUUIDEvent creates a maskedEvent for unit UUIDs,
// encoding the UUID with a custom function.
func newUnitUUIDEvent(event changestream.ChangeEvent, change string) maskedEvent {
	return maskedEvent{
		ChangeEvent: event,
		change:      change,
		encodeUUID:  relation.EncodeUnitUUID,
	}
}

// getUnitsInRelation fetches all units for applications participating in
// the input relation, whether or not they have entered scope.
// Units that are in scope will have a populated RelationUnitUUID.
// The RelationEndpointUUID can be used to tell what side of the relation
// the returned units are on.
func (st *State) getUnitsInRelation(ctx context.Context, relUUID string) ([]relationUnit, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	relUnit := relationUnit{RelationUUID: relUUID}

	q := `
SELECT u.uuid as &relationUnit.unit_uuid,
	   re.uuid as &relationUnit.relation_endpoint_uuid,
       IFNULL(ru.uuid, '') as &relationUnit.uuid
FROM   relation_endpoint re
       JOIN application_endpoint ae ON re.endpoint_uuid = ae.uuid
       JOIN unit u ON ae.application_uuid = u.application_uuid
       LEFT JOIN relation_unit ru ON u.uuid = ru.unit_uuid
                                  AND re.uuid = ru.relation_endpoint_uuid
WHERE  re.relation_uuid = $relationUnit.relation_uuid
-- This aids the domain-level integration tests.
ORDER BY u.uuid`

	stmt, err := st.Prepare(q, relUnit)
	if err != nil {
		return nil, errors.Errorf("preparing units in relation query: %w", err)
	}

	var units []relationUnit
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, relUnit).GetAll(&units)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("running units in relation query: %w", err)
		}
		return nil
	})

	st.logger.Tracef(ctx, "units in relation: %#v", units)

	return units, errors.Capture(err)
}

func (st *State) getRelationAppEndpoints(ctx context.Context, relationUUID string) (map[string]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	rUUID := entityUUID{UUID: relationUUID}

	type appEndpoint struct {
		RelEndpointUUID string `db:"uuid"`
		ApplicationUUID string `db:"application_uuid"`
	}

	q := `
SELECT (re.uuid, ae.application_uuid) AS (&appEndpoint.*)
FROM   relation_endpoint re JOIN application_endpoint ae ON re.endpoint_uuid = ae.uuid
WHERE  re.relation_uuid = $entityUUID.uuid`

	stmt, err := st.Prepare(q, appEndpoint{}, rUUID)
	if err != nil {
		return nil, errors.Errorf("preparing endpoints query: %w", err)
	}

	var endpoints []appEndpoint
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, rUUID).GetAll(&endpoints)
		if err != nil {
			return errors.Errorf("running endpoints query: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.SliceToMap(endpoints, func(e appEndpoint) (string, string) {
		return e.RelEndpointUUID, e.ApplicationUUID
	}), nil
}
