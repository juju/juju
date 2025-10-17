// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/errors"
)

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
	exists, err := st.checkExistsByUUID(ctx, tx, "relation_unit", relationUnitUUID)
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
