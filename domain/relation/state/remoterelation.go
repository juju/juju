// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/errors"
)

// RemoteUnitsEnterScope indicates that the provided units have joined the
// relation. When the unit has already entered its relation scope,
// RemoteUnitsEnterScope will report success but make no changes to state. The
// unit's settings are created or overwritten in the relation according to
// the supplied map. This does not handle subordinate unit creation, or
// related checks.
func (st *State) RemoteUnitsEnterScope(
	ctx context.Context,
	applicationUUID, relationUUID string,
	applicationSettings map[string]string,
	unitSettings map[string]map[string]string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	unitNames := make([]string, 0, len(unitSettings))
	for name := range unitSettings {
		unitNames = append(unitNames, name)
	}
	type names []string

	getUnitsStmt, err := st.Prepare(`
SELECT &unitUUIDName.*
FROM   unit
WHERE  name IN ($names[:])
`, unitUUIDName{}, names{})
	if err != nil {
		return errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get all the unit UUIDs for the unit names.
		var units []unitUUIDName
		if err := tx.Query(ctx, getUnitsStmt, names(unitNames)).GetAll(&units); errors.Is(err, sqlair.ErrNoRows) {
			return relationerrors.UnitNotFound
		} else if err != nil {
			return errors.Capture(err)
		}

		// Ensure that all the units are correctly found up front.
		if len(units) != len(unitNames) {
			missing := findMissingNames(units, unitNames)
			return errors.Errorf("expected %d units, got %d, missing: %v", len(unitNames), len(units), missing)
		}

		// Set all the unit settings that are available.
		for _, unit := range units {
			// Ensure the unit can enter scope in this relation.
			if err := st.checkUnitCanEnterScopeForRemoteRelation(ctx, tx, relationUUID, unit.UUID); err != nil {
				return errors.Capture(err)
			}

			// Upsert the row recording that the unit has entered scope.
			relationUnitUUID, err := st.insertRelationUnit(ctx, tx, relationUUID, unit.UUID)
			if err != nil {
				return errors.Capture(err)
			}

			// We guarantee that the unit settings exist here, as we've checked
			// that all the unit names exist above.
			settings := unitSettings[unit.Name]

			// Set the relation unit settings.
			err = st.setRelationUnitSettings(ctx, tx, relationUnitUUID, settings)
			if err != nil {
				return errors.Errorf("setting relation unit settings: %w", err)
			}
		}

		if err := st.setRelationApplicationSettings(ctx, tx, relationUUID, applicationUUID, applicationSettings); err != nil {
			return errors.Errorf("setting relation unit settings: %w", err)
		}

		return nil
	}); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// checkUnitCanEnterScopeForRemoteRelation checks that the unit can enter scope in the given
// relation.
func (st *State) checkUnitCanEnterScopeForRemoteRelation(ctx context.Context, tx *sqlair.TX, relationUUID, unitUUID string) error {
	// Check relation is alive.
	relationLife, err := st.getLife(ctx, tx, "relation", relationUUID)
	if errors.Is(err, coreerrors.NotFound) {
		return relationerrors.RelationNotFound
	} else if err != nil {
		return errors.Errorf("getting relation life: %w", err)
	}
	if relationLife != life.Alive {
		return relationerrors.CannotEnterScopeNotAlive
	}

	// Check unit is alive.
	unitLife, err := st.getLife(ctx, tx, "unit", unitUUID)
	if errors.Is(err, coreerrors.NotFound) {
		return relationerrors.UnitNotFound
	} else if err != nil {
		return errors.Errorf("getting unit life: %w", err)
	}
	if unitLife != life.Alive {
		return relationerrors.CannotEnterScopeNotAlive
	}

	// Get the IDs of the applications in the relation.
	appIDs, err := st.getApplicationsInRelation(ctx, tx, relationUUID)
	if err != nil {
		return errors.Errorf("getting applications in relation: %w", err)
	}

	// Get the ID of the application for the unit trying to enter scope.
	unitsAppID, err := st.getApplicationOfUnit(ctx, tx, unitUUID)
	if err != nil {
		return errors.Errorf("getting application of unit: %w", err)
	}

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

func findMissingNames(found []unitUUIDName, expected []string) []string {
	all := set.NewStrings(expected...)
	for _, unit := range found {
		all.Remove(unit.Name)
	}
	return all.SortedValues()
}
