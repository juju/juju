// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/database"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/resolve"
	resolveerrors "github.com/juju/juju/domain/resolve/errors"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

// State defines the access mechanism for interacting with the resolve state in
// the context of the model database.
type State struct {
	*domain.StateBase
}

// NewState constructs a new state for interacting with the underlying resolve
// state of a model.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetUnitUUID returns the UUID of the unit with the given name, returning
// an error satisfying [resolveerrors.UnitNotFound] if the unit does not
// exist.
func (st *State) GetUnitUUID(ctx context.Context, name coreunit.Name) (coreunit.UUID, error) {
	unitName := unitName{Name: name}

	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &unitUUID.*
FROM unit
WHERE name=$unitName.name
`, unitUUID{}, unitName)
	if err != nil {
		return "", errors.Capture(err)
	}

	var unitUUID unitUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, unitName).Get(&unitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return resolveerrors.UnitNotFound
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return unitUUID.UUID, nil
}

// UnitResolveMode returns the resolve mode for the given unit. If no resolved
// marker is found for the unit, an error satisfying [resolveerrors.UnitNotResolved]
// is returned.
func (st *State) UnitResolveMode(ctx context.Context, uuid coreunit.UUID) (resolve.ResolveMode, error) {
	unitUUID := unitUUID{UUID: uuid}

	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &unitResolveMode.*
FROM unit_resolved
WHERE unit_uuid = $unitUUID.uuid
`, unitResolveMode{}, unitUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	var mode unitResolveMode
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, unitUUID).Get(&mode)
		if errors.Is(err, sqlair.ErrNoRows) {
			return resolveerrors.UnitNotResolved
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return decodeResolveMode(mode.ModeID)
}

// ResolveUnit marks the unit as resolved. If no agent status is found for the
// specified unit uuid, an error satisfying [resolveerrors.UnitAgentStatusNotFound]
// is returned. If the unit is not in error status, an error satisfying
// [resolveerrors.UnitNotInErrorStatus] is returned.
func (st *State) ResolveUnit(ctx context.Context, uuid coreunit.UUID, mode resolve.ResolveMode) error {
	unitUUID := unitUUID{UUID: uuid}

	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	errorStatusID, err := status.EncodeAgentStatus(status.UnitAgentStatusError)
	if err != nil {
		return errors.Errorf("cannot encode error status: %w", err)
	}

	resolveModeID, err := encodeResolveMode(mode)
	if err != nil {
		return errors.Errorf("cannot encode resolve mode: %w", err)
	}

	getUnitStatusID, err := st.Prepare(`
SELECT &statusID.*
FROM unit_agent_status
WHERE unit_uuid = $unitUUID.uuid
`, statusID{}, unitUUID)
	if err != nil {
		return errors.Capture(err)
	}

	resolveStmt, err := st.Prepare(`
INSERT INTO unit_resolved (*)
VALUES ($unitResolve.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    mode_id = excluded.mode_id
`, unitResolve{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check the unit is in error state. Only units in error state can be
		// resolved.
		var statusID statusID
		err := tx.Query(ctx, getUnitStatusID, unitUUID).Get(&statusID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("checking unit %q status: %w", uuid, resolveerrors.UnitAgentStatusNotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		if statusID.StatusID != errorStatusID {
			return errors.Errorf("checking unit %q status: %w", uuid, resolveerrors.UnitNotInErrorState)
		}

		unitResolve := unitResolve{
			UnitUUID: uuid,
			ModeID:   resolveModeID,
		}
		if err := tx.Query(ctx, resolveStmt, unitResolve).Run(); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("resolving unit: %w", err)
	}
	return nil
}

// ResolveAllUnits marks all units as resolved.
func (st *State) ResolveAllUnits(ctx context.Context, mode resolve.ResolveMode) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	errorStatusID, err := status.EncodeAgentStatus(status.UnitAgentStatusError)
	if err != nil {
		return errors.Errorf("cannot encode error status: %w", err)
	}

	resolveModeID, err := encodeResolveMode(mode)
	if err != nil {
		return errors.Errorf("cannot encode resolve mode: %w", err)
	}

	getUnitsInErrorStatusStmt, err := st.Prepare(`
SELECT unit_uuid AS &unitUUID.uuid
FROM unit_agent_status
WHERE status_id = $statusID.status_id
`, unitUUID{}, statusID{})
	if err != nil {
		return errors.Capture(err)
	}

	resolveStmt, err := st.Prepare(`
INSERT INTO unit_resolved (*)
VALUES ($unitResolve.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    mode_id = excluded.mode_id
`, unitResolve{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Retrieve all units in error state. Only units in error state can be
		// resolved. Ignore the rest.
		var errorUnits []unitUUID
		err := tx.Query(ctx, getUnitsInErrorStatusStmt, statusID{StatusID: errorStatusID}).GetAll(&errorUnits)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}

		if len(errorUnits) == 0 {
			return nil
		}

		unitResolves := transform.Slice(errorUnits, func(r unitUUID) unitResolve {
			return unitResolve{
				UnitUUID: r.UUID,
				ModeID:   resolveModeID,
			}
		})
		if err := tx.Query(ctx, resolveStmt, unitResolves).Run(); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("resolving all units: %w", err)
	}
	return nil
}

// ClearResolved removes any resolved marker from the unit. If the unit is not
// marked as resolved, an error that satisfies [resolveerrors.UnitNotResolved]
// will be returned.
func (st *State) ClearResolved(ctx context.Context, uuid coreunit.UUID) error {
	unitUUID := unitUUID{UUID: uuid}

	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	stmt, err := st.Prepare(`
DELETE FROM unit_resolved
WHERE unit_uuid = $unitUUID.uuid
`, unitUUID)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, unitUUID).Get(&outcome); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("clearing resolved marker: %w", err)
	}
	affected, err := outcome.Result().RowsAffected()
	if err != nil {
		return errors.Errorf("determining results of clearing resolved marker: %w", err)
	}
	if affected == 0 {
		return errors.Errorf("unit %q: %w", unitUUID, resolveerrors.UnitNotResolved)
	}
	return nil
}
