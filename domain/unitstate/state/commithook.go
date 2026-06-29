// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"
	"slices"

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
			return errors.Errorf("update relation settings: %w", err)
		}

		if err := st.updateUnitPorts(ctx, tx, unitUUID, arg.OpenPorts, arg.ClosePorts); err != nil {
			return errors.Errorf("update ports: %w", err)
		}

		if err := st.updateCharmState(ctx, tx, entityUUID{UUID: unitUUID}, arg.CharmState); err != nil {
			return errors.Errorf("update charm state: %w", err)
		}

		if err := st.createSecrets(ctx, tx, arg.SecretCreates); err != nil {
			return errors.Errorf("create secrets: %w", err)
		}

		if err := st.updateSecrets(ctx, tx, arg.SecretUpdates); err != nil {
			return errors.Errorf("update secrets: %w", err)
		}

		if err := st.grantSecretsAccess(ctx, tx, arg.SecretGrants); err != nil {
			return errors.Errorf("grant secrets access: %w", err)
		}

		if err := st.revokeSecretsAccess(ctx, tx, arg.SecretRevokes); err != nil {
			return errors.Errorf("revoke secrets access: %w", err)
		}

		// TODO(secrets): clean up unit secret reservations and tokens here,
		// inside the transaction, once the state-layer implementation is
		// provided (currently a no-op in domain/secret/state).

		if err := st.deleteSecrets(ctx, tx, arg.SecretDeletes); err != nil {
			return errors.Errorf("delete secrets: %w", err)
		}

		if err := st.trackSecrets(ctx, tx, arg.UnitUUID, arg.TrackLatestSecrets); err != nil {
			return errors.Errorf("track latest secrets: %w", err)
		}

		if err := st.addStorage(ctx, tx, arg); err != nil {
			return errors.Errorf("add storage: %w", err)
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

func (st *State) updateSecrets(ctx context.Context, tx *sqlair.TX, updates []internal.UpdateSecretArg) error {
	if len(updates) == 0 {
		return nil
	}

	ids := make(secretIDs, 0, len(updates))
	for _, u := range updates {
		ids = append(ids, u.SecretID)
	}
	existing, err := st.filterExistingSecrets(ctx, tx, ids)
	if err != nil {
		return errors.Capture(err)
	}

	for _, update := range updates {
		if _, ok := existing[update.SecretID]; !ok {
			st.logger.Debugf(ctx, "secret %q no longer exists, skipping update", update.SecretID)
			continue
		}

		if err := st.updateCharmSecret(ctx, tx, update); err != nil {
			return errors.Errorf("updating secret %q: %w", update.SecretID, err)
		}
	}

	idsToUpdate := make(secretIDs, 0, len(updates))
	for id := range existing {
		idsToUpdate = append(idsToUpdate, id)
	}
	return st.markSecretRevisionsObsolete(ctx, tx, idsToUpdate)
}

func (st *State) updateCharmSecret(ctx context.Context, tx *sqlair.TX, update internal.UpdateSecretArg) error {
	existingQuery := `
SELECT sm.secret_id AS &secretInfo.secret_id,
       sm.version AS &secretInfo.version,
       sm.description AS &secretInfo.description,
       sm.rotate_policy_id AS &secretInfo.rotate_policy_id,
       sm.auto_prune AS &secretInfo.auto_prune,
       sm.latest_revision_checksum AS &secretInfo.latest_revision_checksum,
       sm.create_time AS &secretInfo.create_time,
       sm.update_time AS &secretInfo.update_time,
       MAX(sr.revision) AS &secretInfo.latest_revision
FROM   secret_metadata sm
       LEFT JOIN secret_revision sr ON sr.secret_id = sm.secret_id
WHERE  sm.secret_id = $secretID.secret_id
GROUP BY sm.secret_id`
	existingStmt, err := st.Prepare(existingQuery, secretID{}, secretInfo{})
	if err != nil {
		return errors.Capture(err)
	}

	var existing []secretInfo
	err = tx.Query(ctx, existingStmt, secretID{ID: update.SecretID}).GetAll(&existing)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errors.Capture(err)
	}
	info := existing[0]

	rotatePolicyID := info.RotatePolicyID
	if update.RotatePolicy != nil {
		rotatePolicyID = int(*update.RotatePolicy)
	}

	upsertMdStmt, err := st.Prepare(`
INSERT INTO secret_metadata (secret_id, version, description, rotate_policy_id, auto_prune, latest_revision_checksum, create_time, update_time)
VALUES ($secretInfo.secret_id, $secretInfo.version, $secretInfo.description, $secretInfo.rotate_policy_id, $secretInfo.auto_prune, $secretInfo.latest_revision_checksum, $secretInfo.create_time, $secretInfo.update_time)
ON CONFLICT(secret_id) DO UPDATE SET
    description=excluded.description,
    rotate_policy_id=excluded.rotate_policy_id,
    latest_revision_checksum=excluded.latest_revision_checksum,
    update_time=excluded.update_time
`, secretInfo{})
	if err != nil {
		return errors.Capture(err)
	}

	updatedInfo := secretInfo{
		ID:                     update.SecretID,
		Version:                info.Version,
		Description:            info.Description,
		RotatePolicyID:         rotatePolicyID,
		AutoPrune:              info.AutoPrune,
		LatestRevisionChecksum: update.Checksum,
		CreateTime:             info.CreateTime,
		UpdateTime:             st.clock.Now().UTC(),
	}
	if update.Description != nil {
		updatedInfo.Description = *update.Description
	}

	if err := tx.Query(ctx, upsertMdStmt, updatedInfo).Run(); err != nil {
		return errors.Capture(err)
	}

	if update.Label != nil {
		label := *update.Label
		switch update.OwnerKind {
		case secret.ApplicationCharmSecretOwner:
			ownerStmt, err := st.Prepare(`
INSERT INTO secret_application_owner (secret_id, application_uuid, label)
SELECT $secretID.secret_id, owner_uuid, $secretApplicationOwner.label
FROM   v_secret_owner
WHERE  secret_id = $secretID.secret_id
ON CONFLICT(secret_id, application_uuid) DO UPDATE SET label=excluded.label
`, secretID{}, secretApplicationOwner{})
			if err != nil {
				return errors.Capture(err)
			}
			if err := tx.Query(ctx, ownerStmt, secretID{ID: update.SecretID}, secretApplicationOwner{Label: label}).Run(); err != nil {
				return errors.Errorf("updating application secret label: %w", err)
			}
		case secret.UnitCharmSecretOwner:
			ownerStmt, err := st.Prepare(`
INSERT INTO secret_unit_owner (secret_id, unit_uuid, label)
SELECT $secretID.secret_id, owner_uuid, $secretUnitOwner.label
FROM   v_secret_owner
WHERE  secret_id = $secretID.secret_id
ON CONFLICT(secret_id, unit_uuid) DO UPDATE SET label=excluded.label
`, secretID{}, secretUnitOwner{})
			if err != nil {
				return errors.Capture(err)
			}
			if err := tx.Query(ctx, ownerStmt, secretID{ID: update.SecretID}, secretUnitOwner{Label: label}).Run(); err != nil {
				return errors.Errorf("updating unit secret label: %w", err)
			}
		}
	}

	if update.RevisionUUID != "" {
		rev := secretRevision{
			UUID:       update.RevisionUUID,
			SecretID:   update.SecretID,
			Revision:   info.LatestRevision + 1,
			CreateTime: st.clock.Now().UTC(),
			UpdateTime: st.clock.Now().UTC(),
		}
		upsertRevStmt, err := st.Prepare(`
INSERT INTO secret_revision (uuid, secret_id, revision, create_time, update_time)
VALUES ($secretRevision.*)
ON CONFLICT(uuid) DO UPDATE SET update_time=excluded.update_time
`, secretRevision{})
		if err != nil {
			return errors.Capture(err)
		}
		if err := tx.Query(ctx, upsertRevStmt, rev).Run(); err != nil {
			return errors.Errorf("inserting revision: %w", err)
		}

		if update.ExpireTime != nil {
			expStmt, err := st.Prepare(`
INSERT INTO secret_revision_expire (revision_uuid, expire_time)
VALUES ($secretRevisionExpire.*)
ON CONFLICT(revision_uuid) DO UPDATE SET expire_time=excluded.expire_time
`, secretRevisionExpire{})
			if err != nil {
				return errors.Capture(err)
			}
			exp := secretRevisionExpire{RevisionUUID: rev.UUID, ExpireTime: *update.ExpireTime}
			if err := tx.Query(ctx, expStmt, exp).Run(); err != nil {
				return errors.Errorf("setting revision expiry: %w", err)
			}
		}

		if len(update.Data) > 0 {
			contentStmt, err := st.Prepare(`
INSERT INTO secret_content (revision_uuid, name, content)
VALUES ($secretContent.*)
ON CONFLICT(revision_uuid, name) DO UPDATE SET content=excluded.content
`, secretContent{})
			if err != nil {
				return errors.Capture(err)
			}
			for name, val := range update.Data {
				content := secretContent{RevisionUUID: rev.UUID, Name: name, Content: val}
				if err := tx.Query(ctx, contentStmt, content).Run(); err != nil {
					return errors.Errorf("inserting secret content: %w", err)
				}
			}
		}

		if update.ValueRefBackendID != "" {
			refStmt, err := st.Prepare(`
INSERT INTO secret_value_ref (revision_uuid, backend_uuid, revision_id)
VALUES ($secretValueRef.*)
ON CONFLICT(revision_uuid) DO UPDATE SET backend_uuid=excluded.backend_uuid, revision_id=excluded.revision_id
`, secretValueRef{})
			if err != nil {
				return errors.Capture(err)
			}
			ref := secretValueRef{
				RevisionUUID: rev.UUID,
				BackendUUID:  update.ValueRefBackendID,
				RevisionID:   update.ValueRefRevisionID,
			}
			if err := tx.Query(ctx, refStmt, ref).Run(); err != nil {
				return errors.Errorf("setting revision value ref: %w", err)
			}
		}
	}

	return nil
}

func (st *State) grantSecretsAccess(ctx context.Context, tx *sqlair.TX, grants []internal.GrantSecretArg) error {
	if len(grants) == 0 {
		return nil
	}

	// Collect unique secret IDs and batch-check their existence.
	seen := make(map[string]struct{}, len(grants))
	for _, g := range grants {
		if _, ok := seen[g.SecretID]; ok {
			continue
		}
		seen[g.SecretID] = struct{}{}
	}
	ids := slices.Collect(maps.Keys(seen))
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
	seen := make(map[string]struct{}, len(revokes))
	for _, r := range revokes {
		if _, ok := seen[r.SecretID]; ok {
			continue
		}
		seen[r.SecretID] = struct{}{}
	}
	ids := slices.Collect(maps.Keys(seen))
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
			return errors.Errorf("revoking secret access for %q on %q: %w", rev.SubjectUUID, rev.SecretID, err)
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

// maxSecretsPerObsoleteQuery is the maximum number of secret IDs that can be
// passed to markSecretRevisionsObsolete in a single call. SQLite/DQLite limits
// bind variables to 32766 per statement (SQLITE_MAX_VARIABLE_NUMBER); the
// obsolete-revision query expands the secretIDs slice independently in 4 IN
// clauses, producing 4N bind variables for N secrets. Sqlair expands each
// $secretIDs[:] occurrence as an independent set of bind variables (it does
// not use SQLite's named ?N shared references), so the effective multiplier
// is 4. Safe maximum: 32766/4 = 8191.
const maxSecretsPerObsoleteQuery = 8191

// getModelUUID returns the UUID of the model stored in the model table,
// within the supplied transaction.
func (st *State) getModelUUID(ctx context.Context, tx *sqlair.TX) (string, error) {
	stmt, err := st.Prepare("SELECT &entityUUID.uuid FROM model", entityUUID{})
	if err != nil {
		return "", errors.Capture(err)
	}
	var result entityUUID
	if err := tx.Query(ctx, stmt).Get(&result); err != nil {
		return "", errors.Errorf("querying model UUID: %w", err)
	}
	return result.UUID, nil
}

// trackSecrets updates secret_unit_consumer rows so that the specified unit
// tracks the latest revision for each supplied secret. Secrets that no longer
// exist are silently skipped (idempotent). After updating each consumer row,
// revisions that are no longer in use are marked obsolete so the removal
// worker can clean them up.
//
// The ids slice contains bare secret ID strings (xid format), as stored
// in CommitHookChangesArg.TrackLatestSecrets. Only local secrets are tracked
// here; cross-model secrets are never added to
// CommitHookChangesArg.TrackLatestSecrets by the uniter context.
func (st *State) trackSecrets(ctx context.Context, tx *sqlair.TX, unitUUID string, ids secretIDs) error {
	if len(ids) == 0 {
		return nil
	}

	existing, err := st.filterExistingSecrets(ctx, tx, ids)
	if err != nil {
		return errors.Capture(err)
	}
	if len(existing) == 0 {
		return nil
	}

	// Local secrets store the model UUID as their source_model_uuid, matching
	// the invariant set by SaveSecretConsumer. Fetch it once for the upsert.
	modelUUID, err := st.getModelUUID(ctx, tx)
	if err != nil {
		return errors.Errorf("getting model UUID for secret tracking: %w", err)
	}

	// Build the sorted slice of IDs that are confirmed to exist.
	existingIDs := secretIDs(slices.Sorted(maps.Keys(existing)))

	// Batch-fetch the latest (MAX) revision for every existing secret in one
	// GROUP BY query. Secrets with no revisions are absent from the result.
	latestRevStmt, err := st.Prepare(`
SELECT secret_id AS &secretIDAndRevision.secret_id,
       MAX(revision) AS &secretIDAndRevision.revision
FROM   secret_revision
WHERE  secret_id IN ($secretIDs[:])
GROUP BY secret_id
`, secretIDAndRevision{}, secretIDs{})
	if err != nil {
		return errors.Capture(err)
	}

	var latestRevs []secretIDAndRevision
	if err := tx.Query(ctx, latestRevStmt, existingIDs).GetAll(&latestRevs); err != nil &&
		!errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("batch-querying latest revisions: %w", err)
	}

	if len(latestRevs) == 0 {
		return nil
	}

	// Build the consumer upsert slice, skipping any secret with revision 0
	// (should not happen after filtering above, but guard defensively).
	consumers := make([]secretUnitConsumerLatest, 0, len(latestRevs))
	toMark := make(secretIDs, 0, len(latestRevs))
	for _, r := range latestRevs {
		if r.Revision == 0 {
			st.logger.Debugf(ctx, "secret %q has no revisions, skipping track", r.SecretID)
			continue
		}
		consumers = append(consumers, secretUnitConsumerLatest{
			SecretID:        r.SecretID,
			SourceModelUUID: modelUUID,
			UnitUUID:        unitUUID,
			CurrentRevision: r.Revision,
		})
		toMark = append(toMark, r.SecretID)
	}

	if len(consumers) == 0 {
		return nil
	}

	// Upsert all consumer rows in one bulk statement. On conflict, only
	// current_revision is updated so the existing label is preserved.
	upsertStmt, err := st.Prepare(`
INSERT INTO secret_unit_consumer (secret_id, source_model_uuid, unit_uuid, label, current_revision)
VALUES ($secretUnitConsumerLatest.secret_id, $secretUnitConsumerLatest.source_model_uuid,
        $secretUnitConsumerLatest.unit_uuid, '',
        $secretUnitConsumerLatest.current_revision)
ON CONFLICT(secret_id, unit_uuid) DO UPDATE SET
    current_revision = excluded.current_revision
`, secretUnitConsumerLatest{})
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, upsertStmt, consumers).Run(); err != nil {
		return errors.Errorf("bulk-upserting secret consumers: %w", err)
	}

	// Mark obsolete revisions for all tracked secrets, chunked to stay within
	// SQLite's 32766 bind-variable limit (the query uses the slice in 4 places).
	for chunk := range slices.Chunk(toMark, maxSecretsPerObsoleteQuery) {
		if err := st.markSecretRevisionsObsolete(ctx, tx, chunk); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

// markSecretRevisionsObsolete marks secret revisions that are no longer in use
// as obsolete with pending_delete=true across all supplied secret IDs in two
// queries: one to identify obsolete revision UUIDs, one bulk INSERT to mark
// them.
//
// A revision is considered "in use" if at least one local or remote consumer
// currently tracks it, or if it is the latest revision for the secret.
//
// The caller must ensure len(ids) <= maxSecretsPerObsoleteQuery to stay within
// SQLite's 32766 bind-variable limit (the query uses ids in 4 IN clauses).
//
// This mirrors (*State).markObsoleteRevisions in domain/secret/state but
// operates inside the unitstate commit-hook transaction to avoid a separate
// round-trip.
func (st *State) markSecretRevisionsObsolete(ctx context.Context, tx *sqlair.TX, ids secretIDs) error {
	findObsoleteStmt, err := st.Prepare(`
SELECT sr.uuid AS &secretRevisionUUID.uuid
FROM   secret_revision sr
       LEFT JOIN (
           SELECT DISTINCT current_revision AS revision, secret_id
           FROM   secret_unit_consumer
           WHERE  secret_id IN ($secretIDs[:])
           UNION
           SELECT DISTINCT current_revision AS revision, secret_id
           FROM   secret_remote_unit_consumer
           WHERE  secret_id IN ($secretIDs[:])
           UNION
           SELECT MAX(revision) AS revision, secret_id
           FROM   secret_revision
           WHERE  secret_id IN ($secretIDs[:])
           GROUP BY secret_id
       ) in_use ON sr.revision = in_use.revision
              AND sr.secret_id = in_use.secret_id
WHERE  sr.secret_id IN ($secretIDs[:])
AND    (in_use.revision IS NULL OR in_use.revision = 0)
`, secretRevisionUUID{}, secretIDs{})
	if err != nil {
		return errors.Capture(err)
	}

	var obsoleteUUIDs []secretRevisionUUID
	err = tx.Query(ctx, findObsoleteStmt, ids).GetAll(&obsoleteUUIDs)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("querying obsolete revisions: %w", err)
	}
	if len(obsoleteUUIDs) == 0 {
		return nil
	}

	markStmt, err := st.Prepare(`
INSERT INTO secret_revision_obsolete (revision_uuid, obsolete, pending_delete)
VALUES ($secretRevisionObsolete.revision_uuid,
        $secretRevisionObsolete.obsolete,
        $secretRevisionObsolete.pending_delete)
ON CONFLICT(revision_uuid) DO UPDATE SET
    obsolete       = excluded.obsolete,
    pending_delete = excluded.pending_delete
`, secretRevisionObsolete{})
	if err != nil {
		return errors.Capture(err)
	}

	recs := make([]secretRevisionObsolete, len(obsoleteUUIDs))
	for i, rev := range obsoleteUUIDs {
		recs[i] = secretRevisionObsolete{
			UUID:          rev.UUID,
			Obsolete:      true,
			PendingDelete: true,
		}
	}
	if err := tx.Query(ctx, markStmt, recs).Run(); err != nil {
		return errors.Errorf("bulk-marking revisions obsolete: %w", err)
	}
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
