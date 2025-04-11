// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/internal/errors"
)

func (st *State) InitialWatchRelatedUnits(name coreunit.Name, uuid corerelation.UUID) (string, string, eventsource.NamespaceQuery,
	eventsource.Mapper) {

	relationUnitNamespace := "relation_unit"
	relationEndpointNamespace := "relation_endpoint"

	return relationUnitNamespace, relationEndpointNamespace,
		func(ctx context.Context, _ database.TxnRunner) ([]string, error) {
			db, err := st.DB()
			if err != nil {
				return nil, errors.Capture(err)
			}
			var units []getUnit
			err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
				// Initial Query: return all related unit UUID
				units, err = st.getRelatedUnits(ctx, tx, name, uuid)
				if err != nil {
					return errors.Errorf("fetching related unit for %q, through relation %s: %w", name, uuid, err)
				}
				return nil
			})
			return transform.Slice(units, func(u getUnit) string {
					return fmt.Sprintf("%s:%s", relationUnitNamespace, u.UUID.String())
				}),
				errors.Capture(err)
		}, func(ctx context.Context, _ database.TxnRunner, events []changestream.ChangeEvent) ([]changestream.
			ChangeEvent, error) {
			db, err := st.DB()
			if err != nil {
				return nil, errors.Capture(err)
			}
			// Determine if the change comes from relation_unit or relation_endpoint
			var out []changestream.ChangeEvent
			err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
				var relatedUnits set.Strings
				var relatedEndpoint *string
				for _, event := range events {
					switch event.Namespace() {
					case relationUnitNamespace: // Changes from relation_unit
						// Fetch related unit if not already fetched.
						if relatedUnits == nil {
							fetchedRelatedUnits, err := st.getRelatedUnits(ctx, tx, name, uuid)
							if err != nil {
								return errors.Errorf("fetching related unit for %q, through relation %s: %w", name, uuid, err)
							}
							relatedUnits = set.NewStrings(transform.Slice(fetchedRelatedUnits,
								func(u getUnit) string { return u.UUID.String() })...)
						}
						// Discard event that are not from related units.
						if !relatedUnits.Contains(event.Changed()) {
							continue
						}
					case relationEndpointNamespace: // Changes from relation_endpoint
						if relatedEndpoint == nil {
							endpointUUID, err := st.getRelatedRelationEndpointUUIDForUnit(ctx, tx, name, uuid)
							if err != nil {
								return errors.Errorf("filtering event %q from namespace %q: %w", event.Changed(), event.Namespace(), err)
							}
							relatedEndpoint = &endpointUUID
						}
						// Discard event that are not from the expected endpoint
						if event.Changed() != *relatedEndpoint {
							continue
						}
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

// getRelatedEndpointUUIDForUnit fetches the related EndpointUUID linked to a
// given unit and relation UUID. If there is no such endpoint, return an empty string.
func (st *State) getRelatedRelationEndpointUUIDForUnit(ctx context.Context, tx *sqlair.TX, name coreunit.Name,
	relationUUID corerelation.UUID) (string, error) {

	type relationEndpoint struct {
		UUID string `db:"uuid"`
	}

	stmt, err := st.Prepare(`
SELECT re.uuid AS &relationEndpoint.uuid
FROM relation_endpoint AS re
JOIN application_endpoint AS ae 
  ON  re.endpoint_uuid = ae.uuid
  AND re.relation_uuid = $getRelationUnit.relation_uuid
-- We do a left join, because the other application may not have any units
LEFT JOIN unit AS u ON ae.application_uuid = u.application_uuid
-- Due to the left join, u.name may be null, so we use IS NOT instead of !=
WHERE u.name IS NOT $getRelationUnit.name 
`, relationEndpoint{}, getRelationUnit{})
	if err != nil {
		return "", errors.Capture(err)
	}
	var endpoint relationEndpoint
	err = tx.Query(ctx, stmt, getRelationUnit{
		RelationUUID: relationUUID,
		Name:         name,
	}).Get(&endpoint)
	// if there is no row, we may be in a peer relation. Returning an empty string
	// in this case will be ok.
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Capture(err)
	}

	return endpoint.UUID, nil
}

// getRelatedUnits fetches the list of unit UUIDs related to a given unit name
// through a specific relation UUID, including units from the same application.
// Returns a slice of related unit UUIDs or an error in case of failure.
// It returns all potential units, event those not in the relation scope.
func (st *State) getRelatedUnits(ctx context.Context, tx *sqlair.TX, name coreunit.Name, uuid corerelation.UUID) ([]getUnit,
	error) {
	type relation struct {
		UUID corerelation.UUID `db:"uuid"`
	}

	stmt, err := st.Prepare(`
SELECT u.* AS &getUnit.*
FROM   unit AS u
JOIN   application_endpoint AS ae ON u.application_uuid = ae.application_uuid
JOIN   relation_endpoint AS re ON ae.uuid = re.endpoint_uuid AND re.relation_uuid = $relation.uuid
WHERE  u.name != $getUnit.name
`, getUnit{}, relation{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var units []getUnit
	err = tx.Query(ctx, stmt, getUnit{Name: name}, relation{UUID: uuid}).GetAll(&units)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	return units, nil
}
