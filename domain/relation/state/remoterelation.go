// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/status"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// SetRemoteRelationSuspendedState sets the suspended state of the specified
// remote relation in the local model. The relation must be a cross-model
func (st *State) SetRemoteRelationSuspendedState(
	ctx context.Context,
	relationUUID string,
	suspended bool,
	reason string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkRelationExistsByUUID(ctx, tx, relationUUID)
		if err != nil {
			return errors.Errorf("checking relation exists: %w", err)
		} else if !exists {
			return relationerrors.RelationNotFound
		}

		// Check it's a remote relation.
		remote, err := st.isRemoteRelation(ctx, tx, relationUUID)
		if err != nil {
			return errors.Errorf("checking relation is remote: %w", err)
		} else if !remote {
			return errors.Errorf("relation must be a remote relation to be suspended")
		}

		// Set the suspended state.
		if err := st.updateRemoteRelationSuspendedState(ctx, tx, relationUUID, suspended, reason); err != nil {
			return errors.Errorf("updating remote relation suspended state: %w", err)
		}

		return nil
	}); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// SetRelationErrorStatus sets the relation status to Error. This method only
// allows updating the status of cross-model relations.
//
// The following error types can be expected to be returned:
//   - [relationerrors.RelationNotFound] is returned if the relation UUID is not
//     found.
func (st *State) SetRelationErrorStatus(
	ctx context.Context,
	relationUUID string,
	message string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkRelationExistsByUUID(ctx, tx, relationUUID)
		if err != nil {
			return errors.Errorf("checking relation exists: %w", err)
		} else if !exists {
			return relationerrors.RelationNotFound
		}

		// Check it's a remote relation.
		remote, err := st.isRemoteRelation(ctx, tx, relationUUID)
		if err != nil {
			return errors.Errorf("checking relation is remote: %w", err)
		} else if !remote {
			return errors.Errorf("relation must be a remote relation to set error status")
		}

		// Set the error status.
		if err := st.updateRelationStatus(ctx, tx, relationUUID, message); err != nil {
			return errors.Errorf("updating relation status: %w", err)
		}

		return nil
	}); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (st *State) isRemoteRelation(ctx context.Context, tx *sqlair.TX, relationUUID string) (bool, error) {
	type count struct {
		Count int `db:"count"`
	}

	selectStmt, err := st.Prepare(`
SELECT COUNT(cs.name) AS &count.count
FROM   charm_source AS cs
JOIN   charm AS c ON cs.id = c.source_id
JOIN   application AS a ON c.uuid = a.charm_uuid
JOIN   application_endpoint AS ae ON a.uuid = ae.application_uuid
JOIN   relation_endpoint AS re ON ae.uuid = re.endpoint_uuid
WHERE  re.relation_uuid = $entityUUID.uuid
AND    cs.name = 'cmr'
`, count{}, entityUUID{})
	if err != nil {
		return false, errors.Capture(err)
	}

	var counter count
	if err := tx.Query(ctx, selectStmt, entityUUID{UUID: relationUUID}).Get(&counter); errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return counter.Count > 0, nil
}

func (st *State) updateRemoteRelationSuspendedState(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID string,
	suspended bool,
	reason string,
) error {
	type suspension struct {
		Suspended bool   `db:"suspended"`
		Reason    string `db:"suspended_reason"`
	}

	updateStmt, err := st.Prepare(`
UPDATE relation
SET    suspended = $suspension.suspended,
	   suspended_reason = $suspension.suspended_reason
WHERE  uuid = $entityUUID.uuid
`, suspension{}, entityUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, updateStmt, suspension{
		Suspended: suspended,
		Reason:    reason,
	}, entityUUID{UUID: relationUUID}).Run(); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// SetRelationRemoteApplicationAndUnitSettings will set the application and
// unit settings for a remote relation. If the unit has not yet entered
// scope, it will force the unit to enter scope. All settings will be
// replaced with the provided settings.
// This will ensure that the application, relation and units exist and that
// they are alive.
//
// Additionally, it will prevent a unit from entering scope if:
// - the relation is a peer relation
// - the unit's application is a subordinate
func (st *State) SetRelationRemoteApplicationAndUnitSettings(
	ctx context.Context,
	applicationUUID, relationUUID string,
	applicationSettings map[string]string,
	unitSettings map[string]map[string]string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	appUUID := entityUUID{UUID: applicationUUID}

	unitNames := make([]string, 0, len(unitSettings))
	for name := range unitSettings {
		unitNames = append(unitNames, name)
	}
	type names []string

	getUnitsStmt, err := st.Prepare(`
SELECT &unitUUIDNameLife.*
FROM   unit u
LEFT   JOIN life AS l ON l.id = u.life_id AND l.value != 'dead'
WHERE  u.name IN ($names[:])
AND    u.application_uuid = $entityUUID.uuid
`, unitUUIDNameLife{}, names{}, appUUID)
	if err != nil {
		return errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// We need to ensure that relation exists and it has entered scope
		// before attempting to set the application settings. If however we're
		// just setting the application settings, and no units have been
		// provided, then we can skip the whole unit statements below.
		if len(unitNames) > 0 {
			// Get all the unit UUIDs for the unit names.
			var units []unitUUIDNameLife
			if err := tx.Query(ctx, getUnitsStmt, names(unitNames), appUUID).GetAll(&units); errors.Is(err, sqlair.ErrNoRows) {
				return applicationerrors.UnitNotFound
			} else if err != nil {
				return errors.Capture(err)
			}

			// Ensure that all the units are correctly found up front.
			if len(units) != len(unitNames) {
				missing := findMissingNames(units, unitNames)
				return errors.Errorf("expected %d units, got %d, missing: %v", len(unitNames), len(units), missing).Add(applicationerrors.UnitNotFound)
			}

			// Check relation is alive.
			relationLife, err := st.getRelationLife(ctx, tx, relationUUID)
			if errors.Is(err, coreerrors.NotFound) {
				return relationerrors.RelationNotFound
			} else if err != nil {
				return errors.Errorf("getting relation life: %w", err)
			} else if relationLife != life.Alive {
				return relationerrors.CannotEnterScopeNotAlive
			}

			// Get the IDs of the applications in the relation.
			appIDs, err := st.getApplicationsInRelation(ctx, tx, relationUUID)
			if err != nil {
				return errors.Errorf("getting applications in relation: %w", err)
			}

			// Ensure the unit can enter scope in this relation.
			if err := st.checkUnitCanEnterScopeForRemoteRelation(ctx, tx, applicationUUID, appIDs); err != nil {
				return errors.Capture(err)
			}

			// Set all the unit settings that are available.
			for _, unit := range units {
				// Insert the row recording that the unit has entered scope.
				relationUnitUUID, err := st.insertRelationUnit(ctx, tx, relationUUID, unit.UUID)
				if err != nil {
					return errors.Capture(err)
				}

				// We guarantee that the unit settings exist here, as we've
				// checked that all the unit names exist above.
				settings := unitSettings[unit.Name]

				// Blindly insert the settings for the unit, replacing any
				// existing settings.
				if err := st.insertRelationUnitSettings(ctx, tx, relationUnitUUID, settings); err != nil {
					return errors.Errorf("replacing relation unit settings: %w", err)
				}
			}
		}

		// Set the application settings for the relation.
		if err := st.setRelationApplicationSettings(ctx, tx, relationUUID, applicationUUID, applicationSettings); err != nil {
			return errors.Errorf("setting relation unit settings: %w", err)
		}

		return nil
	}); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// checkUnitCanEnterScopeForRemoteRelation checks that the unit can enter scope
// in the given relation.
func (st *State) checkUnitCanEnterScopeForRemoteRelation(ctx context.Context, tx *sqlair.TX, unitsAppID string, appIDs []string) error {
	// Check that the application of the unit is in the relation. Remote
	// relations can not be peer relations, or for a subordinate unit.
	switch len(appIDs) {
	case 1: // Peer relation.
		return relationerrors.CannotEnterScopePeerRelation
	case 2: // Regular relation.
		// If the unit application is a subordinate, it can not enter scope.
		if subordinate, err := st.isSubordinate(ctx, tx, unitsAppID); err != nil {
			return errors.Errorf("checking if application is subordinate: %w", err)
		} else if subordinate {
			return relationerrors.CannotEnterScopeForSubordinate
		}
		return nil
	default:
		return errors.Errorf("unexpected number of applications in relation: %d", len(appIDs))
	}
}

func (st *State) insertRelationUnitSettings(
	ctx context.Context,
	tx *sqlair.TX,
	relationUnitUUID string,
	settings map[string]string,
) error {
	// Get the relation endpoint UUID.
	exists, err := st.checkRelationUnitExistsByUUID(ctx, tx, relationUnitUUID)
	if err != nil {
		return errors.Errorf("checking relation unit exists: %w", err)
	} else if !exists {
		return relationerrors.RelationUnitNotFound
	}

	// Update the unit settings specified in the settings argument.
	err = st.replaceUnitSettings(ctx, tx, relationUnitUUID, settings)
	if err != nil {
		return errors.Errorf("updating relation unit settings: %w", err)
	}

	// Fetch all the new settings in the relation for this unit.
	newSettings, err := st.getRelationUnitSettings(ctx, tx, relationUnitUUID)
	if err != nil {
		return errors.Errorf("getting new relation unit settings: %w", err)
	}

	// Hash the new settings.
	hash, err := hashSettings(newSettings)
	if err != nil {
		return errors.Errorf("generating hash of relation unit settings: %w", err)
	}

	// Update the hash in the database.
	err = st.updateUnitSettingsHash(ctx, tx, relationUnitUUID, hash)
	if err != nil {
		return errors.Errorf("updating relation unit settings hash: %w", err)
	}

	return nil
}

// replaceUnitSettings replaces all the settings for a relation unit according
// to the provided settings map.
func (st *State) replaceUnitSettings(
	ctx context.Context, tx *sqlair.TX, relUnitUUID string, settings map[string]string,
) error {
	id := entityUUID{UUID: relUnitUUID}
	deleteStmt, err := st.Prepare(`
DELETE FROM relation_unit_setting
WHERE       relation_unit_uuid = $entityUUID.uuid
`, id)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, deleteStmt, id).Run()
	if err != nil {
		return errors.Capture(err)
	}

	// Determine the keys to set and unset.
	var set []relationUnitSetting
	for k, v := range settings {
		if v == "" {
			continue
		}

		set = append(set, relationUnitSetting{
			UUID:  relUnitUUID,
			Key:   k,
			Value: v,
		})
	}

	// Insert the keys to set.
	if len(set) > 0 {
		updateStmt, err := st.Prepare(`
INSERT INTO relation_unit_setting (*) 
VALUES ($relationUnitSetting.*) 
`, relationUnitSetting{})
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, updateStmt, set).Run()
		if err != nil {
			return errors.Capture(err)
		}
	}

	return nil
}

func findMissingNames(found []unitUUIDNameLife, expected []string) []string {
	all := set.NewStrings(expected...)
	for _, unit := range found {
		all.Remove(unit.Name)
	}
	return all.SortedValues()
}

func (st *State) updateRelationStatus(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID string,
	message string,
) error {
	statusID, err := status.EncodeRelationStatus(status.RelationStatusTypeError)
	if err != nil {
		return errors.Capture(err)
	}

	now := time.Now().UTC()

	statusInfo := relationStatus{
		RelationUUID: relationUUID,
		StatusID:     statusID,
		Message:      message,
		Since:        &now,
	}
	stmt, err := st.Prepare(`
INSERT INTO relation_status (*) VALUES ($relationStatus.*)
ON CONFLICT(relation_uuid) DO UPDATE SET
    relation_status_type_id = excluded.relation_status_type_id,
    message = excluded.message,
    updated_at = excluded.updated_at
WHERE relation_uuid = $relationStatus.relation_uuid
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, statusInfo).Run()
	if internaldatabase.IsErrConstraintForeignKey(err) {
		return relationerrors.RelationNotFound
	}
	return errors.Capture(err)
}
