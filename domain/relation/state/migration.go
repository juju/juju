// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainrelation "github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

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
			return errors.Errorf("setting new relation: %s %s: %w", epIdentifier1, epIdentifier2, err)
		}

		// Insert relation status.
		if err := st.insertNewRelationStatus(ctx, tx, relUUID); err != nil {
			return errors.Errorf("setting new relation status: %s %s: %w", epIdentifier1, epIdentifier2, err)
		}

		// Insert both relation_endpoint from application_endpoint_uuid and relation
		// uuid.
		if err := st.insertNewRelationEndpoint(ctx, tx, relUUID, endpointUUID1); err != nil {
			return errors.Errorf("setting new relation endpoint for %q: %w", epIdentifier1.String(), err)
		}
		if err := st.insertNewRelationEndpoint(ctx, tx, relUUID, endpointUUID2); err != nil {
			return errors.Errorf("setting new relation endpoint for %q: %w", epIdentifier2.String(), err)
		}

		return nil
	})
	return relUUID, errors.Capture(err)
}

// GetApplicationIDByName returns the application ID of the given application.
//
// The following error types can be expected to be returned:
//   - [applicationerrors.ApplicationNotFound] is returned if application ID
//     doesn't refer an existing application.
func (st *State) GetApplicationIDByName(ctx context.Context, appName string) (application.ID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var id application.ID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		app := applicationIDAndName{Name: appName}
		queryApplicationStmt, err := st.Prepare(`
SELECT uuid AS &applicationIDAndName.uuid
FROM application
WHERE name = $applicationIDAndName.name
`, app)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, queryApplicationStmt, app).Get(&app)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("%w: %s", applicationerrors.ApplicationNotFound, appName)
		} else if err != nil {
			return errors.Errorf("looking up UUID for application %q: %w", appName, err)
		}
		id = app.ID
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
	applicationID application.ID,
	settings map[string]string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setRelationApplicationSettings(ctx, tx, relationUUID, applicationID, settings)
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// DeleteImportedRelations deletes all imported relations in a model during
// an import rollback.
func (st *State) DeleteImportedRelations(
	ctx context.Context,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	tables := []string{
		"relation_unit_setting",
		"relation_unit_settings_hash",
		"relation_unit",
		"relation_application_setting",
		"relation_application_settings_hash",
		"relation_endpoint",
		"relation_status",
		"relation",
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, table := range tables {
			stmt, err := st.Prepare(fmt.Sprintf(`DELETE FROM %s`, table))
			if err != nil {
				return errors.Capture(err)
			}

			if err = tx.Query(ctx, stmt).Run(); err != nil {
				return errors.Errorf("deleting table %q: %w", table, err)
			}
		}

		return nil
	})
	return errors.Capture(err)
}

// ExportRelations returns all relation information to be exported for the
// model.
func (st *State) ExportRelations(ctx context.Context) ([]domainrelation.ExportRelation, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var exportRelations []domainrelation.ExportRelation
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		type getRelation struct {
			UUID corerelation.UUID `db:"uuid"`
			ID   int               `db:"relation_id"`
		}
		stmt, err := st.Prepare(`
SELECT (r.uuid, r.relation_id) AS (&getRelation.*)
FROM   relation r
`, getRelation{})
		if err != nil {
			return errors.Capture(err)
		}

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

			eps, err := st.exportRelationEndpoints(ctx, tx, rel.UUID)
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
