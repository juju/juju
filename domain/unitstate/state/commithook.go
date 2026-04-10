// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/unitstate"
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
	unitUUID := arg.UnitUUID.String()

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// TODO (hml) 2-Apr-2026
		// For every UUID added to arg, add check that it exists in the model.
		// Remove todo when this method is complete.
		if err := st.ensureCommitHookChangesUUIDs(ctx, tx, arg); err != nil {
			return errors.Errorf("unit data has changed since beginning:%w", err)
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

		// TODO: (hml) 10-Dec-2025
		// Implement storage
		return nil
	})
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

// GetUnitUUIDByName returns the UUID for the named unit, returning an
// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't
// exist.
func (st *State) GetUnitUUIDByName(ctx context.Context, name coreunit.Name) (coreunit.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var result entityUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result, err = st.getUnitUUIDForName(ctx, tx, string(name))
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return coreunit.UUID(result.UUID), nil
}

// ensureCommitHookChangesUUIDs verifies that all UUIDs in arg still exist.
// Do not check life, as hooks still run when various entities are dying,
// e.g. relations.
func (st *State) ensureCommitHookChangesUUIDs(ctx context.Context, tx *sqlair.TX, arg internal.CommitHookChangesArg) error {
	if err := st.checkUnitExists(ctx, tx, arg.UnitUUID.String()); err != nil {
		return err
	}

	if err := st.checkRelationsExist(ctx, tx, arg.RelationSettings); err != nil {
		return errors.Errorf("relations: %w", err)
	}
	return nil
}

// checkUnitExists checks if a unit with the given UUID exists in the model.
func (st *State) checkUnitExists(
	ctx context.Context, tx *sqlair.TX, uuid string,
) error {

	entityUUIDInput := entityUUID{UUID: uuid}
	stmt, err := st.Prepare(
		"SELECT &entityUUID.* FROM unit WHERE  uuid = $entityUUID.uuid",
		entityUUIDInput,
	)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, entityUUIDInput).Get(&entityUUIDInput)
	if errors.Is(err, sqlair.ErrNoRows) {
		return applicationerrors.UnitNotFound
	} else if err != nil {
		return errors.Capture(err)
	}

	return nil
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
		return errors.Errorf("expected %d, found %d", len(relationUUIDs),
			result.Count).Add(relationerrors.RelationNotFound)
	}
	return nil
}
