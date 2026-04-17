// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/unitstate"
	unitstateerrors "github.com/juju/juju/domain/unitstate/errors"
	"github.com/juju/juju/domain/unitstate/internal"
	"github.com/juju/juju/internal/errors"
)

// CommitHookChanges persists a set of changes after a hook successfully
// completes and executes them in a single transaction.
func (st *State) CommitHookChanges(ctx context.Context, arg internal.CommitHookChangesArg) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	unitUUID := arg.UnitUUID

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unitLife, err := st.getUnitLife(ctx, tx, arg.UnitUUID)
		if err != nil {
			return errors.Errorf("checking unit alive: %w", err)
		}
		if unitLife != int(arg.UnitLife) {
			return unitstateerrors.UnitLifePredicateFailed
		}

		if err := st.checkRelationsExist(ctx, tx, arg.RelationSettings); err != nil {
			return errors.Errorf("relations: %w", err)
		}

		if err := st.updateNetworkInfo(ctx, tx, arg.UpdateNetworkInfo); err != nil {
			return errors.Errorf("update network info: %w", err)
		}

		if err := st.updateRelationSettings(ctx, tx, unitUUID, arg.RelationSettings); err != nil {
			return errors.Errorf("update relation settings:%w", err)
		}

		if err := st.updateUnitPorts(ctx, tx, unitUUID, arg.OpenPorts, arg.ClosePorts); err != nil {
			return errors.Errorf("update ports:%w", err)
		}

		if err := st.updateCharmState(ctx, tx, entityUUID{UUID: unitUUID}, arg.CharmState); err != nil {
			return errors.Errorf("update charm state:%w", err)
		}

		if err := st.createSecrets(ctx, tx, arg.SecretCreates); err != nil {
			return errors.Errorf("create secrets:%w", err)
		}

		if err := st.updateSecrets(ctx, tx, arg.SecretUpdates); err != nil {
			return errors.Errorf("update secrets:%w", err)
		}

		if err := st.grantSecretsAccess(ctx, tx, arg.SecretGrants); err != nil {
			return errors.Errorf("grant secrets access:%w", err)
		}

		if err := st.revokeSecretsAccess(ctx, tx, arg.SecretRevokes); err != nil {
			return errors.Errorf("revoke secrets access:%w", err)
		}

		if err := st.deleteSecrets(ctx, tx, arg.SecretDeletes); err != nil {
			return errors.Errorf("delete secrets:%w", err)
		}

		if err := st.trackSecrets(ctx, tx, arg.TrackLatestSecrets); err != nil {
			return errors.Errorf("track latest secrets:%w", err)
		}

		if err := st.addStorage(ctx, tx, arg); err != nil {
			return errors.Errorf("add storage:%w", err)
		}

		return nil
	})
}

func (st *State) updateNetworkInfo(ctx context.Context, tx *sqlair.TX, info bool) error {
	return nil
}

func (st *State) updateRelationSettings(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID string,
	relationSettings []internal.RelationSettings,
) error {
	for _, settings := range relationSettings {
		if err := st.setRelationApplicationAndUnitSettings(ctx, tx, unitUUID, settings); err != nil {
			return errors.Errorf("setting relation settings for relation %q: %w", settings.RelationUUID, err)
		}
	}
	return nil
}

func (st *State) updateCharmState(ctx context.Context, tx *sqlair.TX, unit entityUUID, charmState *map[string]string) error {
	if charmState == nil {
		return nil
	}
	return st.setUnitStateCharm(ctx, tx, unit, *charmState)
}

func (st *State) createSecrets(ctx context.Context, tx *sqlair.TX, creates []unitstate.CreateSecretArg) error {
	return nil
}

func (st *State) updateSecrets(ctx context.Context, tx *sqlair.TX, updates []unitstate.UpdateSecretArg) error {
	return nil
}

func (st *State) grantSecretsAccess(ctx context.Context, tx *sqlair.TX, grants []unitstate.GrantRevokeSecretArg) error {
	return nil
}

func (st *State) revokeSecretsAccess(ctx context.Context, tx *sqlair.TX, revokes []unitstate.GrantRevokeSecretArg) error {
	return nil
}

func (st *State) deleteSecrets(ctx context.Context, tx *sqlair.TX, deletes []unitstate.DeleteSecretArg) error {
	return nil
}

func (st *State) trackSecrets(ctx context.Context, tx *sqlair.TX, secrets []string) error {
	return nil
}

// GetCommitHookUnitInfo returns the unit UUID and machine UUID if assigned,
// returning an error satisfying
//
// The following errors can be expected:
// - [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) GetCommitHookUnitInfo(ctx context.Context, name string) (internal.CommitHookUnitInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return internal.CommitHookUnitInfo{}, errors.Capture(err)
	}

	unitNameArg := unitName{Name: name}

	stmt, err := st.Prepare(`
SELECT u.uuid AS &commitHookUnitInfo.unit_uuid,
       u.life_id AS &commitHookUnitInfo.unit_life_id,
       m.uuid AS &commitHookUnitInfo.machine_uuid
FROM   unit u
LEFT JOIN machine m ON m.net_node_uuid = u.net_node_uuid
WHERE  u.name = $unitName.name
`, unitNameArg, commitHookUnitInfo{})
	if err != nil {
		return internal.CommitHookUnitInfo{}, errors.Capture(err)
	}

	var result commitHookUnitInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, unitNameArg).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.UnitNotFound
		}
		return errors.Capture(err)
	})
	if err != nil {
		return internal.CommitHookUnitInfo{}, errors.Capture(err)
	}

	retVal := internal.CommitHookUnitInfo{
		UnitUUID: result.UnitUUID,
		UnitLife: life.Life(result.UnitLife),
	}
	if result.MachineUUID.Valid {
		retVal.MachineUUID = &result.MachineUUID.String
	}

	return retVal, nil
}

func (st *State) getUnitLife(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID string,
) (int, error) {
	arg := entityUUID{UUID: unitUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.*
FROM   unit
WHERE  uuid = $entityUUID.uuid
`, arg, entityLife{})
	if err != nil {
		return 0, errors.Capture(err)
	}

	var result entityLife
	err = tx.Query(ctx, stmt, arg).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return 0, applicationerrors.UnitNotFound
	} else if err != nil {
		return 0, errors.Capture(err)
	}

	return result.Life, nil
}

// checkRelationsExist checks if all relations in settings exist in the model.
func (st *State) checkRelationsExist(ctx context.Context, tx *sqlair.TX, settings []internal.RelationSettings) error {
	if len(settings) == 0 {
		return nil
	}
	relationUUIDs := transform.Slice(settings, func(s internal.RelationSettings) string {
		return s.RelationUUID.String()
	})

	stmt, err := st.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM   relation
WHERE  uuid IN ($uuids[:])
`, countResult{}, uuids{})
	if err != nil {
		return errors.Capture(err)
	}

	var result countResult
	err = tx.Query(ctx, stmt, uuids(relationUUIDs)).Get(&result)
	if err != nil {
		return errors.Capture(err)
	}
	if result.Count != len(relationUUIDs) {
		return errors.Errorf(
			"expected %d relations but found %d",
			len(relationUUIDs), result.Count,
		).Add(relationerrors.RelationNotFound)
	}
	return nil
}
