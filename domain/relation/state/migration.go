// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment/charm"
	domainrelation "github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/errors"
)

// ImportPeerRelation establishes a peer relation on the endpoint passed as
// argument. Used for migration import.
func (st *State) ImportPeerRelation(
	ctx context.Context,
	epIdentifier corerelation.EndpointIdentifier,
	id uint64,
	scope charm.RelationScope,
) (corerelation.UUID, error) {
	var relUUID corerelation.UUID
	db, err := st.DB(ctx)
	if err != nil {
		return relUUID, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get endpoint uuid for the application.
		endpointUUID, err := st.getApplicationEndpointUUID(ctx, tx, epIdentifier.ApplicationName, epIdentifier.EndpointName)
		if err != nil {
			return err
		}

		// Insert a new relation with a new relation UUID.
		relUUID, err = st.insertNewRelation(ctx, tx, id, scope)
		if err != nil {
			return errors.Errorf("inserting new relation: %w", err)
		}

		// Insert relation_endpoint
		if err := st.insertNewRelationEndpoint(ctx, tx, relUUID, endpointUUID); err != nil {
			return errors.Errorf("inserting relation endpoint %q: %w", epIdentifier.String(), err)
		}

		return nil
	})
	return relUUID, errors.Capture(err)
}

// ImportRelation establishes a relation between two endpoints identified
// by ep1 and ep2 and returns the relation UUID. Used for migration
// import.
func (st *State) ImportRelation(
	ctx context.Context,
	epIdentifier1, epIdentifier2 corerelation.EndpointIdentifier,
	id uint64,
	scope charm.RelationScope,
) (corerelation.UUID, error) {
	var relUUID corerelation.UUID
	db, err := st.DB(ctx)
	if err != nil {
		return relUUID, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get endpoint uuids for both endpoints of the relation.
		endpointUUID1, err := st.getApplicationEndpointUUID(ctx, tx, epIdentifier1.ApplicationName, epIdentifier1.EndpointName)
		if err != nil {
			return err
		}
		endpointUUID2, err := st.getApplicationEndpointUUID(ctx, tx, epIdentifier2.ApplicationName, epIdentifier2.EndpointName)
		if err != nil {
			return err
		}

		// Insert a new relation with a new relation UUID.
		relUUID, err = st.insertNewRelation(ctx, tx, id, scope)
		if err != nil {
			return errors.Errorf("inserting new relation: %w", err)
		}

		// Insert both relation_endpoint from application_endpoint_uuid and relation
		// uuid.
		if err := st.insertNewRelationEndpoint(ctx, tx, relUUID, endpointUUID1); err != nil {
			return errors.Errorf("inserting relation endpoint %q: %w", epIdentifier1.String(), err)
		}
		if err := st.insertNewRelationEndpoint(ctx, tx, relUUID, endpointUUID2); err != nil {
			return errors.Errorf("inserting relation endpoint %q: %w", epIdentifier2.String(), err)
		}

		return nil
	})
	return relUUID, errors.Capture(err)
}

// GetApplicationUUIDByName returns the application UUID of the given application.
//
// The following error types can be expected to be returned:
//   - [applicationerrors.ApplicationNotFound] is returned if application UUID
//     doesn't refer an existing application.
func (st *State) GetApplicationUUIDByName(ctx context.Context, appName string) (application.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var id application.UUID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		app := nameAndUUID{Name: appName}
		queryApplicationStmt, err := st.Prepare(`
SELECT uuid AS &nameAndUUID.uuid
FROM application
WHERE name = $nameAndUUID.name
`, app)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, queryApplicationStmt, app).Get(&app)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting UUID for application %q not found", appName).
				Add(applicationerrors.ApplicationNotFound)
		} else if err != nil {
			return errors.Errorf("looking up UUID for application %q: %w", appName, err)
		}
		id = application.UUID(app.UUID)
		return nil
	}); err != nil {
		return "", errors.Capture(err)
	}
	return id, nil
}

// SetRelationApplicationSettings records settings for a specific application
// relation combination.
//
// The following error types can be expected to be returned:
//   - [relationerrors.ApplicationNotFoundForRelation] is returned if the
//     application is not part of the relation.
//   - [relationerrors.RelationNotFound] is returned if the relation UUID
//     is not found.
func (st *State) SetRelationApplicationSettings(
	ctx context.Context,
	relationUUID corerelation.UUID,
	applicationID application.UUID,
	settings map[string]string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setRelationApplicationSettings(ctx, tx, relationUUID.String(), applicationID.String(), settings)
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// ExportRelations returns all relation information to be exported for the
// model.
func (st *State) ExportRelations(ctx context.Context) ([]domainrelation.ExportRelation, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	type getRelation struct {
		UUID corerelation.UUID `db:"uuid"`
		ID   int               `db:"relation_id"`
	}
	stmt, err := st.Prepare(`
SELECT (r.uuid, r.relation_id) AS (&getRelation.*)
FROM   relation r
`, getRelation{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var exportRelations []domainrelation.ExportRelation
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var rels []getRelation
		err = tx.Query(ctx, stmt).GetAll(&rels)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}

		for _, rel := range rels {
			exportRelation := domainrelation.ExportRelation{
				ID: rel.ID,
			}

			eps, err := st.exportRelationEndpoints(ctx, tx, rel.UUID.String())
			if err != nil {
				return errors.Errorf("getting relation endpoints: %w", err)
			}
			for _, ep := range eps {
				exportEndpoint := domainrelation.ExportEndpoint{
					ApplicationName: ep.ApplicationName,
					Name:            ep.EndpointName,
					Role:            ep.Role,
					Interface:       ep.Interface,
					Optional:        ep.Optional,
					Limit:           ep.Capacity,
					Scope:           ep.Scope,
				}

				appSettings, err := st.getApplicationSettings(ctx, tx, ep.RelationEndpointUUID)
				if err != nil {
					return errors.Errorf("getting application settings: %w", err)
				}
				exportEndpoint.ApplicationSettings = make(map[string]any, len(appSettings))
				for _, s := range appSettings {
					exportEndpoint.ApplicationSettings[s.Key] = s.Value
				}

				relUnits, err := st.getRelationUnits(ctx, tx, ep.RelationEndpointUUID)
				if err != nil {
					return errors.Errorf("getting relation units: %w", err)
				}

				allUnitSettings := make(map[string]map[string]any)
				for _, relUnit := range relUnits {
					unitSettings, err := st.getRelationUnitSettings(ctx, tx, relUnit.RelationUnitUUID.String())
					if err != nil {
						return errors.Errorf("getting relation unit settings: %w", err)
					}
					exportUnitSettings := make(map[string]any, len(unitSettings))
					for _, s := range unitSettings {
						exportUnitSettings[s.Key] = s.Value
					}
					allUnitSettings[relUnit.UnitName.String()] = exportUnitSettings
				}
				exportEndpoint.AllUnitSettings = allUnitSettings

				exportRelation.Endpoints = append(exportRelation.Endpoints, exportEndpoint)
			}
			exportRelations = append(exportRelations, exportRelation)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return exportRelations, nil
}
