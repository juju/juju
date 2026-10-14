// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationinternal "github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
)

// SetCAASApplicationUnitScale applies a service-validated CAAS unit mutation
// plan. It verifies both the old Alive membership and each new allocated name
// in the same transaction, so a stale plan cannot commit a gap.
func (st *State) SetCAASApplicationUnitScale(
	ctx context.Context, appUUID string, expected []string, leaving []string,
	newUnits []applicationinternal.ScaleCAASUnitArg,
) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Scale down changes desired unit membership atomically with additions. The
	// provisioner observes this Dying unit and calls removal.RemoveUnit, whose
	// cascade is intentionally safe for an already-Dying unit: it schedules
	// removal and advances dependent storage without moving the unit life back.
	var inserted []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Re-read the Alive set inside the transaction before applying the plan.
		_, alive, err := st.getCAASApplicationScaleState(ctx, tx, appUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if len(alive) != len(expected) {
			return applicationerrors.ScalingStateInconsistent
		}

		// Membership can change and return to the same count. Comparing names
		// prevents a stale plan from updating units that are no longer Alive.
		aliveNames := make(map[string]struct{}, len(alive))
		for _, unit := range alive {
			aliveNames[unit.Name] = struct{}{}
		}
		for _, name := range expected {
			if _, ok := aliveNames[name]; !ok {
				return applicationerrors.ScalingStateInconsistent
			}
			delete(aliveNames, name)
		}

		// Departures and new reservations are committed together.
		if err := st.setCAASUnitsDying(ctx, tx, appUUID, leaving); err != nil {
			return errors.Capture(err)
		}

		args := transform.Slice(newUnits, func(unit applicationinternal.ScaleCAASUnitArg) applicationinternal.AddCAASUnitArg {
			return unit.AddCAASUnitArg
		})
		txnInserted, err := st.insertPendingCAASUnits(ctx, tx, appUUID, args)
		if err != nil {
			return errors.Capture(err)
		}
		for i, name := range txnInserted {
			// The sequence can advance after the service builds the plan.
			// Reject a gap using the name allocated in this transaction.
			if name != newUnits[i].ExpectedName {
				return errors.Errorf("allocated unit name changed: %w", applicationerrors.ScalingStateInconsistent)
			}
		}

		inserted = txnInserted
		return nil
	})
	return inserted, errors.Capture(err)
}

// ChangeCAASApplicationUnitScale atomically applies a relative change to the
// Alive unit set. Scale-down removes the lowest ordinals, while scale-up
// allocates fresh identities from the application sequence.
func (st *State) ChangeCAASApplicationUnitScale(
	ctx context.Context, appUUID string, scaleChange int,
	newUnits []applicationinternal.AddCAASUnitArg,
) (int, []string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, nil, errors.Capture(err)
	}

	newScale := -1
	var inserted []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		app, alive, err := st.getCAASApplicationScaleState(ctx, tx, appUUID)
		if err != nil {
			return errors.Capture(err)
		}
		newScale, err = validateCAASScaleChange(len(alive), scaleChange, len(newUnits))
		if err != nil {
			return errors.Capture(err)
		}
		if newScale == 0 {
			if err := st.checkCAASControllerScaleZero(ctx, tx, appUUID); err != nil {
				return errors.Capture(err)
			}
		}

		switch {
		case scaleChange < 0:
			inserted = nil
			return st.setLowestCAASUnitsDying(ctx, tx, appUUID, alive, -scaleChange)
		case scaleChange > 0:
			inserted, err = st.insertScaledCAASUnits(ctx, tx, app, alive, newUnits)
			return errors.Capture(err)
		default:
			inserted = nil
			return nil
		}
	})
	if err != nil {
		return -1, nil, errors.Capture(err)
	}
	return newScale, inserted, nil
}

func (st *State) getCAASApplicationScaleState(
	ctx context.Context, tx *sqlair.TX, appUUID string,
) (applicationDetails, []unitNameAndOrdinal, error) {
	app := applicationDetails{UUID: appUUID}
	getApplication, err := st.Prepare(`
SELECT a.name AS &applicationDetails.name,
       a.life_id AS &applicationDetails.life_id,
       c.source_id = 2 AS &applicationDetails.is_application_synthetic
FROM application AS a
JOIN charm AS c ON c.uuid = a.charm_uuid
WHERE a.uuid = $applicationDetails.uuid
`, app)
	if err != nil {
		return applicationDetails{}, nil, errors.Capture(err)
	}
	if err := tx.Query(ctx, getApplication, app).Get(&app); errors.Is(err, sqlair.ErrNoRows) {
		return applicationDetails{}, nil, errors.Errorf("application %q not found", appUUID).
			Add(applicationerrors.ApplicationNotFound)
	} else if err != nil {
		return applicationDetails{}, nil, errors.Capture(err)
	}
	if app.LifeID != life.Alive {
		return applicationDetails{}, nil, errors.Errorf("application %q is not alive: %w", app.Name, applicationerrors.ApplicationNotAlive)
	}
	if app.IsApplicationSynthetic {
		return applicationDetails{}, nil, errors.Errorf("cannot scale synthetic application %q", app.Name)
	}

	getAlive, err := st.Prepare(`
SELECT u.name AS &unitName.name
FROM unit AS u
WHERE u.application_uuid = $applicationDetails.uuid
AND u.life_id = 0
`, app, unitName{})
	if err != nil {
		return applicationDetails{}, nil, errors.Capture(err)
	}
	var names []unitName
	if err := tx.Query(ctx, getAlive, app).GetAll(&names); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return applicationDetails{}, nil, errors.Capture(err)
	}
	alive := make([]unitNameAndOrdinal, len(names))
	for i, unit := range names {
		ordinal, err := unitOrdinal(unit.Name)
		if err != nil {
			return applicationDetails{}, nil, errors.Capture(err)
		}
		alive[i] = unitNameAndOrdinal{Name: unit.Name, Ordinal: ordinal}
	}
	sort.Slice(alive, func(i, j int) bool {
		return alive[i].Ordinal < alive[j].Ordinal
	})
	return app, alive, nil
}

// validateCAASScaleChange belongs in state because the current Alive count and
// the resulting mutation must be validated in the same transaction. Checking
// the count in the service would race another scale request.
func validateCAASScaleChange(current, change, newUnitCount int) (int, error) {
	switch {
	case change < -current:
		return -1, errors.Errorf("%w: cannot remove more units than currently exist", applicationerrors.ScaleChangeInvalid)
	case change > 0 && current > math.MaxInt-change:
		return -1, applicationerrors.ScaleChangeInvalid
	case change > 0 && newUnitCount != change:
		return -1, errors.Errorf("expected %d new units, got %d", change, newUnitCount)
	case change <= 0 && newUnitCount != 0:
		return -1, errors.Errorf("unexpected new units for scale change %d", change)
	default:
		return current + change, nil
	}
}

func (st *State) checkCAASControllerScaleZero(
	ctx context.Context, tx *sqlair.TX, appUUID string,
) error {
	controller := controllerApplication{ApplicationID: coreapplication.UUID(appUUID)}
	stmt, err := st.Prepare(`
SELECT TRUE AS &controllerApplication.is_controller
FROM application_controller
WHERE application_uuid = $controllerApplication.application_uuid
`, controller)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, stmt, controller).Get(&controller); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Capture(err)
	} else if err == nil && controller.IsController {
		return errors.Errorf("cannot scale controller application to 0 units")
	}
	return nil
}

func (st *State) setLowestCAASUnitsDying(
	ctx context.Context, tx *sqlair.TX, appUUID string,
	units []unitNameAndOrdinal, count int,
) error {
	names := make([]string, count)
	for i, unit := range units[:count] {
		names[i] = unit.Name
	}
	return st.setCAASUnitsDying(ctx, tx, appUUID, names)
}

func (st *State) setCAASUnitsDying(
	ctx context.Context, tx *sqlair.TX, appUUID string, names []string,
) error {
	app := applicationDetails{UUID: appUUID}
	stmt, err := st.Prepare(`
UPDATE unit
SET life_id = 1
WHERE application_uuid = $applicationDetails.uuid
AND name = $unitName.name
AND life_id = 0
`, app, unitName{})
	if err != nil {
		return errors.Capture(err)
	}
	for _, name := range names {
		if err := tx.Query(ctx, stmt, app, unitName{Name: name}).Run(); err != nil {
			return errors.Errorf("marking unit as dying: %w", err)
		}
	}
	return nil
}

func (st *State) insertScaledCAASUnits(
	ctx context.Context, tx *sqlair.TX, app applicationDetails,
	alive []unitNameAndOrdinal, newUnits []applicationinternal.AddCAASUnitArg,
) ([]string, error) {
	for i := 1; i < len(alive); i++ {
		if alive[i].Ordinal != alive[i-1].Ordinal+1 {
			return nil, applicationerrors.ScaleChangeInvalid
		}
	}
	inserted, err := st.insertPendingCAASUnits(ctx, tx, app.UUID, newUnits)
	if err != nil {
		return nil, errors.Capture(err)
	}
	for i, name := range inserted {
		if len(alive) > 0 {
			expectedName := fmt.Sprintf("%s/%d", app.Name, alive[len(alive)-1].Ordinal+i+1)
			if name != expectedName {
				return nil, errors.Errorf("allocated unit name changed: %w", applicationerrors.ScalingStateInconsistent)
			}
		}
	}
	return inserted, nil
}

func (st *State) insertPendingCAASUnits(
	ctx context.Context, tx *sqlair.TX, appUUID string,
	newUnits []applicationinternal.AddCAASUnitArg,
) ([]string, error) {
	if len(newUnits) == 0 {
		return nil, nil
	}
	charmUUID, err := st.getCharmIDByApplicationUUID(ctx, tx, appUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	inserted := make([]string, 0, len(newUnits))
	for _, unit := range newUnits {
		name, err := st.insertCAASUnit(ctx, tx, appUUID, charmUUID, unit)
		if err != nil {
			return nil, errors.Errorf("inserting pending unit: %w", err)
		}
		inserted = append(inserted, name)
	}
	return inserted, nil
}

func unitOrdinal(name string) (int, error) {
	separator := strings.LastIndexByte(name, '/')
	if separator == -1 || separator == len(name)-1 {
		return 0, errors.Errorf("invalid unit name %q", name)
	}
	ordinal, err := strconv.Atoi(name[separator+1:])
	if err != nil || ordinal < 0 {
		return 0, errors.Errorf("invalid unit name %q", name)
	}
	return ordinal, nil
}
