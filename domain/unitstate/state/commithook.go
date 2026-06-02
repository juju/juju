// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/domain/unitstate"
	unitstateerrors "github.com/juju/juju/domain/unitstate/errors"
	"github.com/juju/juju/domain/unitstate/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
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
			return unitstateerrors.UnitLifePreconditionFailed
		}

		if err := st.checkRelationsExist(ctx, tx, arg.RelationSettings); err != nil {
			return errors.Errorf("relations: %w", err)
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

		// TODO(secrets): clean up unit secret reservations and tokens here,
		// inside the transaction, once the state-layer implementation is
		// provided (currently a no-op in domain/secret/state).

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

func (st *State) grantSecretsAccess(ctx context.Context, tx *sqlair.TX, grants []internal.GrantSecretArg) error {
	if len(grants) == 0 {
		return nil
	}

	// Collect unique secret IDs and batch-check their existence.
	ids := make(secretIDs, 0, len(grants))
	for _, g := range grants {
		ids = append(ids, g.SecretID)
	}
	existing, err := st.filterExistingSecrets(ctx, tx, ids)
	if err != nil {
		return errors.Capture(err)
	}

	// Check that an existing permission row has compatible scope and subject
	// type — changing these after initial grant is forbidden.
	checkInvariantStmt, err := st.Prepare(`
SELECT sp.secret_id AS &secretID.secret_id
FROM   secret_permission sp
WHERE  sp.secret_id = $secretPermissionGrant.secret_id
AND    sp.subject_uuid = $secretPermissionGrant.subject_uuid
AND    (sp.subject_type_id <> $secretPermissionGrant.subject_type_id
        OR sp.scope_uuid <> $secretPermissionGrant.scope_uuid
        OR sp.scope_type_id <> $secretPermissionGrant.scope_type_id)
`, secretPermissionGrant{}, secretID{})
	if err != nil {
		return errors.Capture(err)
	}

	// Upsert: insert or update role on existing row.
	upsertStmt, err := st.Prepare(`
INSERT INTO secret_permission (*)
VALUES ($secretPermissionGrant.*)
ON CONFLICT(secret_id, subject_uuid) DO UPDATE SET
    role_id=excluded.role_id,
    subject_type_id=excluded.subject_type_id,
    scope_type_id=excluded.scope_type_id,
    scope_uuid=excluded.scope_uuid
`, secretPermissionGrant{})
	if err != nil {
		return errors.Capture(err)
	}

	for _, g := range grants {
		if _, ok := existing[g.SecretID]; !ok {
			continue
		}

		// Verify subject entity still exists (concurrent removal race).
		subjectExists, err := st.subjectExists(ctx, tx, g.SubjectUUID, g.SubjectTypeID)
		if err != nil {
			return errors.Errorf("checking grant subject %q exists: %w", g.SubjectUUID, err)
		}
		if !subjectExists {
			st.logger.Debugf(ctx, "grant subject %q no longer exists, skipping", g.SubjectUUID)
			continue
		}

		// Verify scope entity still exists (concurrent removal race).
		scopeExists, err := st.scopeExists(ctx, tx, g.ScopeUUID, g.ScopeTypeID)
		if err != nil {
			return errors.Errorf("checking grant scope %q exists: %w", g.ScopeUUID, err)
		}
		if !scopeExists {
			st.logger.Debugf(ctx, "grant scope %q no longer exists, skipping", g.ScopeUUID)
			continue
		}

		perm := secretPermissionGrant{
			SecretID:      g.SecretID,
			SubjectUUID:   g.SubjectUUID,
			SubjectTypeID: g.SubjectTypeID,
			ScopeUUID:     g.ScopeUUID,
			ScopeTypeID:   g.ScopeTypeID,
			RoleID:        g.RoleID,
		}

		// Reject attempts to change the scope or subject type of an existing
		// permission row — this is an invariant in the DB schema.
		var conflictID secretID
		err = tx.Query(ctx, checkInvariantStmt, perm).Get(&conflictID)
		if err == nil {
			return errors.Errorf(
				"cannot change scope or subject type of existing grant for %q on %q",
				g.SubjectUUID, g.SecretID,
			).Add(secreterrors.InvalidSecretPermissionChange)
		} else if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("checking permission invariant for %q on %q: %w", g.SubjectUUID, g.SecretID, err)
		}

		if err := tx.Query(ctx, upsertStmt, perm).Run(); err != nil {
			return errors.Errorf("upserting secret grant for %q on %q: %w", g.SubjectUUID, g.SecretID, err)
		}
	}
	return nil
}

// subjectExists checks whether the subject entity referenced by uuid still
// exists in the database, dispatching to the appropriate table based on type.
func (st *State) subjectExists(ctx context.Context, tx *sqlair.TX, uuid string, t secret.GrantSubjectType) (bool, error) {
	switch t {
	case secret.SubjectUnit:
		return st.unitExists(ctx, tx, uuid)
	case secret.SubjectApplication:
		return st.applicationExists(ctx, tx, uuid)
	case secret.SubjectModel:
		return st.modelExists(ctx, tx, uuid)
	default:
		return false, errors.Errorf("unknown subject type %d", t)
	}
}

// scopeExists checks whether the scope entity referenced by uuid still
// exists in the database, dispatching to the appropriate table based on type.
func (st *State) scopeExists(ctx context.Context, tx *sqlair.TX, uuid string, t secret.GrantScopeType) (bool, error) {
	switch t {
	case secret.ScopeUnit:
		return st.unitExists(ctx, tx, uuid)
	case secret.ScopeApplication:
		return st.applicationExists(ctx, tx, uuid)
	case secret.ScopeModel:
		return st.modelExists(ctx, tx, uuid)
	case secret.ScopeRelation:
		return st.relationExists(ctx, tx, uuid)
	default:
		return false, errors.Errorf("unknown scope type %d", t)
	}
}

// unitExists returns whether a unit with the given UUID exists.
func (st *State) unitExists(ctx context.Context, tx *sqlair.TX, id string) (bool, error) {
	stmt, err := st.Prepare(`SELECT &entityUUID.uuid FROM unit WHERE uuid = $entityUUID.uuid`, entityUUID{})
	if err != nil {
		return false, errors.Capture(err)
	}
	var result entityUUID
	err = tx.Query(ctx, stmt, entityUUID{UUID: id}).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
}

// applicationExists returns whether an application with the given UUID exists.
func (st *State) applicationExists(ctx context.Context, tx *sqlair.TX, id string) (bool, error) {
	stmt, err := st.Prepare(`SELECT &entityUUID.uuid FROM application WHERE uuid = $entityUUID.uuid`, entityUUID{})
	if err != nil {
		return false, errors.Capture(err)
	}
	var result entityUUID
	err = tx.Query(ctx, stmt, entityUUID{UUID: id}).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
}

// modelExists returns whether a model with the given UUID exists.
func (st *State) modelExists(ctx context.Context, tx *sqlair.TX, id string) (bool, error) {
	stmt, err := st.Prepare(`SELECT &entityUUID.uuid FROM model WHERE uuid = $entityUUID.uuid`, entityUUID{})
	if err != nil {
		return false, errors.Capture(err)
	}
	var result entityUUID
	err = tx.Query(ctx, stmt, entityUUID{UUID: id}).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
}

// relationExists returns whether a relation with the given UUID exists.
func (st *State) relationExists(ctx context.Context, tx *sqlair.TX, id string) (bool, error) {
	stmt, err := st.Prepare(`SELECT &entityUUID.uuid FROM relation WHERE uuid = $entityUUID.uuid`, entityUUID{})
	if err != nil {
		return false, errors.Capture(err)
	}
	var result entityUUID
	err = tx.Query(ctx, stmt, entityUUID{UUID: id}).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
}

// filterExistingSecrets returns the subset of ids that are present in
// secret_metadata. Any ID absent from the returned set was concurrently
// deleted; this method logs each such ID at debug level so callers do not
// need to.
func (st *State) filterExistingSecrets(ctx context.Context, tx *sqlair.TX, ids secretIDs) (map[string]struct{}, error) {
	stmt, err := st.Prepare(`
SELECT &secretID.secret_id
FROM   secret_metadata
WHERE  secret_id IN ($secretIDs[:])
`, secretID{}, secretIDs{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []secretID
	if err := tx.Query(ctx, stmt, ids).GetAll(&rows); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("batch-checking secret existence: %w", err)
	}

	existing := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		existing[r.ID] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := existing[id]; !ok {
			st.logger.Debugf(ctx, "secret %q no longer exists, skipping", id)
		}
	}
	return existing, nil
}

func (st *State) revokeSecretsAccess(ctx context.Context, tx *sqlair.TX, revokes []internal.RevokeSecretArg) error {
	if len(revokes) == 0 {
		return nil
	}

	// Collect unique secret IDs and batch-check their existence.
	ids := make(secretIDs, 0, len(revokes))
	for _, r := range revokes {
		ids = append(ids, r.SecretID)
	}
	existing, err := st.filterExistingSecrets(ctx, tx, ids)
	if err != nil {
		return errors.Capture(err)
	}

	// Delete permission row.
	deleteStmt, err := st.Prepare(`
DELETE FROM secret_permission
WHERE  secret_id = $secretPermissionRevoke.secret_id
AND    subject_type_id = $secretPermissionRevoke.subject_type_id
AND    subject_uuid = $secretPermissionRevoke.subject_uuid
`, secretPermissionRevoke{})
	if err != nil {
		return errors.Capture(err)
	}

	for _, rev := range revokes {
		if _, ok := existing[rev.SecretID]; !ok {
			continue
		}

		perm := secretPermissionRevoke{
			SecretID:      rev.SecretID,
			SubjectUUID:   rev.SubjectUUID,
			SubjectTypeID: rev.SubjectTypeID,
		}
		if err := tx.Query(ctx, deleteStmt, perm).Run(); err != nil {
			return errors.Errorf("deleting secret grant for %q on %q: %w", rev.SubjectUUID, rev.SecretID, err)
		}
	}
	return nil
}

func (st *State) deleteSecrets(ctx context.Context, tx *sqlair.TX, deletes []internal.DeleteSecretArg) error {
	if len(deletes) == 0 {
		return nil
	}

	stmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($secretRemovalJob.*)", secretRemovalJob{})
	if err != nil {
		return errors.Capture(err)
	}

	now := st.clock.Now().UTC()
	jobs := make([]secretRemovalJob, 0, len(deletes))
	for _, del := range deletes {
		jobUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}

		rec := secretRemovalJob{
			UUID:          jobUUID.String(),
			RemovalTypeID: charmSecretRemovalJobTypeID,
			EntityUUID:    del.URI,
			ScheduledFor:  now,
		}

		if del.ArgJSON != nil {
			rec.Arg = sql.NullString{
				String: *del.ArgJSON,
				Valid:  true,
			}
		}

		jobs = append(jobs, rec)
	}

	if err := tx.Query(ctx, stmt, jobs).Run(); err != nil {
		return errors.Errorf("inserting secret removal jobs: %w", err)
	}
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
