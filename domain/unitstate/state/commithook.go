// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	coreunit "github.com/juju/juju/core/unit"
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

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.updateRelationSettings(ctx, tx, arg.UnitUUID, arg.RelationSettings); err != nil {
			return errors.Errorf("update relation settings: %v", err)
		}

		if err := st.updateUnitPorts(ctx, tx, arg.UnitUUID.String(), arg.OpenPorts, arg.ClosePorts); err != nil {
			return errors.Errorf("update ports: %v", err)
		}

		if err := st.updateCharmState(ctx, tx, entityUUID{UUID: arg.UnitUUID.String()}, arg.CharmState); err != nil {
			return errors.Errorf("update charm state: %v", err)
		}

		if err := st.createSecrets(ctx, tx, arg.SecretCreates); err != nil {
			return errors.Errorf("create secrets: %v", err)
		}

		if err := st.updateSecrets(ctx, tx, arg.SecretUpdates); err != nil {
			return errors.Errorf("update secrets: %v", err)
		}

		if err := st.grantSecretsAccess(ctx, tx, arg.SecretGrants); err != nil {
			return errors.Errorf("grant secrets access: %v", err)
		}

		if err := st.revokeSecretsAccess(ctx, tx, arg.SecretRevokes); err != nil {
			return errors.Errorf("revoke secrets access: %v", err)
		}

		if err := st.deleteSecrets(ctx, tx, arg.SecretDeletes); err != nil {
			return errors.Errorf("delete secrets: %v", err)
		}

		if err := st.trackSecrets(ctx, tx, arg.TrackLatestSecrets); err != nil {
			return errors.Errorf("track latest secrets: %v", err)
		}

		// TODO: (hml) 10-Dec-2025
		// Implement storage
		return nil
	})
}

func (st *State) updateRelationSettings(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	relationSettings []internal.RelationSettings,
) error {
	for _, settings := range relationSettings {
		if err := st.setRelationApplicationAndUnitSettings(ctx, tx, unitUUID, settings); err != nil {
			return errors.Errorf("setting relation settings for relation %q: %v", settings.RelationUUID, err)
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
