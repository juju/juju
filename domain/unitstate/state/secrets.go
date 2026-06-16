// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/canonical/sqlair"

	coresecrets "github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
	"github.com/juju/juju/internal/errors"
)

// updateSecretMetadataFromParams applies non-nil fields from p onto md.
func updateSecretMetadataFromParams(p domainsecret.UpsertSecretParams, md *secretMetadata) {
	if p.Description != nil {
		md.Description = *p.Description
	}
	if p.AutoPrune != nil {
		md.AutoPrune = *p.AutoPrune
	}
	if p.RotatePolicy != nil {
		md.RotatePolicyID = int(*p.RotatePolicy)
	}
	if p.Checksum != "" {
		md.LatestRevisionChecksum = p.Checksum
	}
	md.CreateTime = p.CreateTime.UTC()
	md.UpdateTime = p.UpdateTime.UTC()
}

// upsertSecretMetadata inserts or updates secret_metadata.
func (st *State) upsertSecretMetadata(ctx context.Context, tx *sqlair.TX, md secretMetadata) error {
	query := `
INSERT INTO secret_metadata (*)
VALUES ($secretMetadata.*)
ON CONFLICT(secret_id) DO UPDATE SET
    version=excluded.version,
    description=excluded.description,
    rotate_policy_id=excluded.rotate_policy_id,
    auto_prune=excluded.auto_prune,
    latest_revision_checksum=excluded.latest_revision_checksum,
    update_time=excluded.update_time`

	stmt, err := st.Prepare(query, secretMetadata{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, md).Run()
}

// upsertSecretRevision inserts or updates a secret_revision record.
func (st *State) upsertSecretRevision(ctx context.Context, tx *sqlair.TX, rev *secretRevision) error {
	query := `
INSERT INTO secret_revision (*)
VALUES ($secretRevision.*)
ON CONFLICT (uuid) DO UPDATE SET
    update_time=excluded.update_time`

	stmt, err := st.Prepare(query, secretRevision{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, rev).Run()
}

// insertSecretContent inserts key/value content for a secret revision.
func (st *State) insertSecretContent(ctx context.Context, tx *sqlair.TX, revUUID string, content coresecrets.SecretData) error {
	query := `
INSERT INTO secret_content (revision_uuid, name, content)
VALUES ($secretContent.revision_uuid, $secretContent.name, $secretContent.content)
ON CONFLICT(revision_uuid, name) DO UPDATE SET
    content=excluded.content`

	stmt, err := st.Prepare(query, secretContent{})
	if err != nil {
		return errors.Capture(err)
	}

	for key, value := range content {
		if err := tx.Query(ctx, stmt, secretContent{
			RevisionUUID: revUUID,
			Name:         key,
			Content:      value,
		}).Run(); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

// upsertSecretValueRef inserts or updates a value reference for a secret
// revision stored in an external backend.
func (st *State) upsertSecretValueRef(ctx context.Context, tx *sqlair.TX, revUUID string, valueRef *coresecrets.ValueRef) error {
	query := `
INSERT INTO secret_value_ref (*)
VALUES ($secretValueRef.*)
ON CONFLICT(revision_uuid) DO UPDATE SET
    backend_uuid=excluded.backend_uuid,
    revision_id=excluded.revision_id`

	stmt, err := st.Prepare(query, secretValueRef{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, secretValueRef{
		RevisionUUID: revUUID,
		BackendUUID:  valueRef.BackendID,
		RevisionID:   valueRef.RevisionID,
	}).Run()
}

// upsertSecretRevisionExpiry inserts or updates the expiry for a secret
// revision.
func (st *State) upsertSecretRevisionExpiry(ctx context.Context, tx *sqlair.TX, revUUID string, expireTime time.Time) error {
	query := `
INSERT INTO secret_revision_expire (*)
VALUES ($secretRevisionExpire.*)
ON CONFLICT(revision_uuid) DO UPDATE SET
    expire_time=excluded.expire_time`

	stmt, err := st.Prepare(query, secretRevisionExpire{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, secretRevisionExpire{
		RevisionUUID: revUUID,
		ExpireTime:   expireTime.UTC(),
	}).Run()
}

// upsertSecretNextRotateTime inserts or updates the next rotation time for a
// secret.
func (st *State) upsertSecretNextRotateTime(ctx context.Context, tx *sqlair.TX, secretID string, next time.Time) error {
	query := `
INSERT INTO secret_rotation (*)
VALUES ($secretRotate.*)
ON CONFLICT(secret_id) DO UPDATE SET
    next_rotation_time=excluded.next_rotation_time`

	stmt, err := st.Prepare(query, secretRotate{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, secretRotate{
		SecretID:       secretID,
		NextRotateTime: next.UTC(),
	}).Run()
}

// setSecretApplicationOwner inserts an application ownership record.
func (st *State) setSecretApplicationOwner(ctx context.Context, tx *sqlair.TX, secretID, appUUID, label string) error {
	query := `
INSERT INTO secret_application_owner (secret_id, application_uuid, label)
VALUES ($secretApplicationOwner.*)
ON CONFLICT(secret_id, application_uuid) DO UPDATE SET label=excluded.label`

	stmt, err := st.Prepare(query, secretApplicationOwner{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, secretApplicationOwner{
		SecretID:        secretID,
		ApplicationUUID: appUUID,
		Label:           label,
	}).Run()
}

// setSecretUnitOwner inserts a unit ownership record.
func (st *State) setSecretUnitOwner(ctx context.Context, tx *sqlair.TX, secretID, unitUUID, label string) error {
	query := `
INSERT INTO secret_unit_owner (secret_id, unit_uuid, label)
VALUES ($secretUnitOwner.*)
ON CONFLICT(secret_id, unit_uuid) DO UPDATE SET label=excluded.label`

	stmt, err := st.Prepare(query, secretUnitOwner{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, secretUnitOwner{
		SecretID: secretID,
		UnitUUID: unitUUID,
		Label:    label,
	}).Run()
}

// grantSecretOwnerManage grants RoleManage permission to the secret owner.
func (st *State) grantSecretOwnerManage(ctx context.Context, tx *sqlair.TX, secretID, ownerUUID string, ownerType domainsecret.GrantSubjectType) error {
	perm := secretPermissionGrant{
		SecretID:      secretID,
		RoleID:        domainsecret.RoleManage,
		SubjectUUID:   ownerUUID,
		SubjectTypeID: ownerType,
		ScopeUUID:     ownerUUID,
	}
	switch ownerType {
	case domainsecret.SubjectUnit:
		perm.ScopeTypeID = domainsecret.ScopeUnit
	case domainsecret.SubjectApplication:
		perm.ScopeTypeID = domainsecret.ScopeApplication
	}

	query := `
INSERT INTO secret_permission (*)
VALUES ($secretPermissionGrant.*)
ON CONFLICT(secret_id, subject_uuid) DO UPDATE SET
    role_id=excluded.role_id,
    subject_type_id=excluded.subject_type_id,
    scope_type_id=excluded.scope_type_id,
    scope_uuid=excluded.scope_uuid`

	stmt, err := st.Prepare(query, secretPermissionGrant{})
	if err != nil {
		return errors.Capture(err)
	}
	return tx.Query(ctx, stmt, perm).Run()
}
