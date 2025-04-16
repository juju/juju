// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/errors"
)

// InitialWatchRelatedUnits returns namepsaces, initial query and mappers to watch
// related units.
func (st *State) InitialWatchRelatedUnits(unitName coreunit.Name, relationUUID corerelation.UUID) ([]string,
	eventsource.NamespaceQuery, eventsource.Mapper) {

	relationUnitNamespace := "relation_unit"
	relationApplicationSettingNamespace := "relation_application_settings_hash"
	relationUnitSettingNamespace := "relation_unit_settings_hash"

	return []string{relationApplicationSettingNamespace, relationUnitSettingNamespace, relationUnitNamespace},
		// Initial query.
		func(ctx context.Context,
			_ database.TxnRunner) ([]string, error) {
			db, err := st.DB()
			if err != nil {
				return nil, errors.Capture(err)
			}
			var units []getRelatedUnit
			err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
				// Initial Query: return all related unit UUID
				units, err = st.getRelatedUnits(ctx, tx, unitName, relationUUID)
				if err != nil {
					return errors.Errorf("fetching units related to %q, through relation %s: %w", unitName, relationUUID, err)
				}
				return nil
			})
			return transform.Slice(units, func(u getRelatedUnit) string {
					return relation.EncodeUnitUUID(u.UUID)
				}),
				errors.Capture(err)
		},
		// Mapper.
		func(ctx context.Context, _ database.TxnRunner, events []changestream.ChangeEvent) ([]changestream.
			ChangeEvent, error) {
			db, err := st.DB()
			if err != nil {
				return nil, errors.Capture(err)
			}

			var out []changestream.ChangeEvent
			err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
				var relatedUnits set.Strings
				var unitByRelationUnit map[string]string

				// Helper to fetch related units once, to avoid multiple query if
				// there is several events coming from the one of the unit
				// namespaces.
				fetchRelatedUnits := func() error {
					if unitByRelationUnit == nil {
						fetchedRelatedUnits, err := st.getRelatedUnits(ctx, tx, unitName, relationUUID)
						if err != nil {
							return errors.Capture(err)
						}
						unitByRelationUnit = make(map[string]string, len(fetchedRelatedUnits))
						relatedUnits = set.NewStrings()
						for _, u := range fetchedRelatedUnits {
							relatedUnits.Add(u.UUID.String())
							unitByRelationUnit[u.RelationUnitUUID.String()] = u.UUID.String()
						}
					}
					return nil
				}
				// This is a pointer to allows lazy loading, we fetch it only
				// if needed (event from application setting namespace), and only
				// once.
				var relatedEndpoint *relationEndpoint
				for _, event := range events {
					switch event.Namespace() {
					case relationUnitNamespace: // Changes from relation_unit
						// Fetch related unit if not already fetched.
						if err := fetchRelatedUnits(); err != nil {
							return errors.Errorf("fetching related unit for %q, through relation %s: %w", unitName, relationUUID, err)
						}

						// Discard event that are not from related units.
						if !relatedUnits.Contains(event.Changed()) {
							continue
						}
						event = newUnitUUIDEvent(event, coreunit.UUID(event.Changed()))
					case relationUnitSettingNamespace:
						// Fetch related unit if not already fetched.
						if err := fetchRelatedUnits(); err != nil {
							return errors.Errorf("fetching related unit for %q, through relation %s: %w", unitName, relationUUID, err)
						}

						// Discard event that are not from related units.
						unitUUID, ok := unitByRelationUnit[event.Changed()]
						if !ok {
							continue
						}
						event = newUnitUUIDEvent(event, coreunit.UUID(unitUUID))
					case relationApplicationSettingNamespace:
						if relatedEndpoint == nil {
							endpoint, err := st.getRelatedRelationEndpointForUnit(ctx, tx, unitName, relationUUID)
							if err != nil {
								return errors.Errorf("filtering event %q from namespace %q: %w", event.Changed(), event.Namespace(), err)
							}
							relatedEndpoint = &endpoint
						}
						// Discard event that are not from the expected endpoint
						if event.Changed() != relatedEndpoint.UUID.String() {
							continue
						}
						event = newApplicationUUIDEvent(event, relatedEndpoint.ApplicationUUID)
					default:
						st.logger.Warningf(ctx, "watching related unit: unexpected namespace %q", event.Namespace())
						continue
					}
					out = append(out, event)
				}
				return nil
			})
			return out, errors.Capture(err)
		}
}

// maskedEvent is a struct that wraps a changestream.ChangeEvent and applies
// a custom UUID encoding function.
type maskedEvent struct {
	changestream.ChangeEvent
	encodeUUID func(string) string
}

// Changed implements wraps changestream.ChangeEvent. It wraps the masked change
// event with a specific encoding function, to keep the uuid type.
func (e maskedEvent) Changed() string {
	return e.encodeUUID(e.ChangeEvent.Changed())
}

// newApplicationUUIDEvent creates a maskedEvent for application UUIDs,
// encoding the UUID with a custom function.
func newApplicationUUIDEvent(event changestream.ChangeEvent, uuid coreapplication.ID) maskedEvent {
	return maskedEvent{
		ChangeEvent: event,
		encodeUUID: func(s string) string {
			return relation.EncodeApplicationUUID(uuid)
		},
	}
}

// newApplicationUUIDEvent creates a maskedEvent for unit UUIDs,
// encoding the UUID with a custom function.
func newUnitUUIDEvent(event changestream.ChangeEvent, uuid coreunit.UUID) maskedEvent {
	return maskedEvent{
		ChangeEvent: event,
		encodeUUID: func(s string) string {
			return relation.EncodeUnitUUID(uuid)
		},
	}
}

// getRelatedRelationEndpointForUnit fetches the related EndpointUUID linked to a
// given unit and relation UUID. If there is no such endpoint, return an empty string.
func (st *State) getRelatedRelationEndpointForUnit(
	ctx context.Context, tx *sqlair.TX,
	name coreunit.Name,
	relationUUID corerelation.UUID,
) (relationEndpoint, error) {
	stmt, err := st.Prepare(`
SELECT 
    re.uuid AS &relationEndpoint.uuid,
    ae.application_uuid AS &relationEndpoint.application_uuid
FROM relation_endpoint AS re
JOIN application_endpoint AS ae 
  ON  re.endpoint_uuid = ae.uuid
  AND re.relation_uuid = $getRelationUnit.relation_uuid
JOIN application AS a ON ae.application_uuid = a.uuid
WHERE ae.application_uuid NOT IN (SELECT application_uuid
                                 FROM unit
                                 WHERE name =  $getRelationUnit.name)
`, relationEndpoint{}, getRelationUnit{})
	if err != nil {
		return relationEndpoint{}, errors.Capture(err)
	}
	var endpoint relationEndpoint
	err = tx.Query(ctx, stmt, getRelationUnit{
		RelationUUID: relationUUID,
		Name:         name,
	}).Get(&endpoint)
	// if there is no row, we may be in a peer relation. Returning an empty string
	// in this case will be ok, it will be discarded in the caller anyway
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return relationEndpoint{}, errors.Capture(err)
	}

	return endpoint, nil
}

// getRelatedUnits fetches the list of unit UUIDs related to a given unit name
// through a specific relation UUID, including units from the same application.
// Returns a slice of related unit UUIDs or an error in case of failure.
// It returns all potential units, event those not in the relation scope.
// However, if the relation is in scope, it also return the relation_unit uuid
func (st *State) getRelatedUnits(
	ctx context.Context, tx *sqlair.TX,
	name coreunit.Name,
	uuid corerelation.UUID,
) ([]getRelatedUnit, error) {
	type relation struct {
		UUID corerelation.UUID `db:"uuid"`
	}

	unitName := getRelatedUnit{Name: name}
	relationUUID := relation{UUID: uuid}

	stmt, err := st.Prepare(`
SELECT 
	u.name AS &getRelatedUnit.name,
	u.uuid AS &getRelatedUnit.uuid,
	ru.uuid AS &getRelatedUnit.relation_unit_uuid
FROM   unit AS u
JOIN   application_endpoint AS ae ON u.application_uuid = ae.application_uuid
JOIN   relation_endpoint AS re ON ae.uuid = re.endpoint_uuid
LEFT JOIN relation_unit AS ru ON u.uuid = ru.unit_uuid 
    						  AND re.uuid = ru.relation_endpoint_uuid
WHERE  u.name != $getRelatedUnit.name
AND    re.relation_uuid = $relation.uuid
-- initial query test are flaky when there is several returns. Ordering allows to avoid it.
ORDER BY u.uuid
`, unitName, relationUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var units []getRelatedUnit
	err = tx.Query(ctx, stmt, unitName, relationUUID).GetAll(&units)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	return units, nil
}
