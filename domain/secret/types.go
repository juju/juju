// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import (
	"time"

	"github.com/juju/juju/core/secrets"
)

// These type aliases are used to specify filter terms.
type (
	Labels            []string
	ApplicationOwners []string
	UnitOwners        []string
)

// These consts are used to specify nil filter terms.
var (
	NilLabels            = Labels(nil)
	NilApplicationOwners = ApplicationOwners(nil)
	NilUnitOwners        = UnitOwners(nil)
	NilRevision          = (*int)(nil)
	NilSecretURI         = (*secrets.URI)(nil)
)

// These represent the kinds of secret owner.
const (
	ApplicationOwner = secrets.ApplicationOwner
	UnitOwner        = secrets.UnitOwner
	ModelOwner       = secrets.ModelOwner
)

// Owner is the owner of a secret.
type Owner struct {
	Kind secrets.OwnerKind
	UUID string
}

// UpsertSecretParams are used to upsert a secret.
// Only non-nil values are used.
type UpsertSecretParams struct {
	RevisionID     *string
	RotatePolicy   *RotatePolicy
	ExpireTime     *time.Time
	NextRotateTime *time.Time
	Description    *string
	Label          *string
	AutoPrune      *bool

	Data     secrets.SecretData
	ValueRef *secrets.ValueRef
	Checksum string
}

// HasUpdate returns true if at least one attribute to update is not nil.
func (u *UpsertSecretParams) HasUpdate() bool {
	return u.NextRotateTime != nil ||
		u.RotatePolicy != nil ||
		u.Description != nil ||
		u.Label != nil ||
		u.ExpireTime != nil ||
		len(u.Data) > 0 ||
		u.ValueRef != nil ||
		u.AutoPrune != nil
}

// GrantParams are used when granting access to a secret.
type GrantParams struct {
	ScopeTypeID GrantScopeType
	ScopeUUID   string

	SubjectTypeID GrantSubjectType
	SubjectUUID   string

	RoleID Role
}

// AccessParams are used when querying secrets
// granted to a unit, application, or model.
type AccessParams struct {
	SubjectTypeID GrantSubjectType
	SubjectID     string
}

// RevokeParams are used when revoking access to a secret.
type RevokeParams struct {
	SubjectTypeID GrantSubjectType
	SubjectUUID   string
}

// AccessScope is the result of querying secret access scopes.
type AccessScope struct {
	ScopeTypeID GrantScopeType
	ScopeUUID   string
}

// GrantDetails holds the access and scope details for
// a secret permission record.
type GrantDetails struct {
	ScopeTypeID GrantScopeType
	ScopeID     string
	ScopeUUID   string

	SubjectTypeID GrantSubjectType
	SubjectID     string

	RoleID Role
}

// RotationExpiryInfo holds information about the rotation and expiry of a secret.
type RotationExpiryInfo struct {
	// RotatePolicy is the rotation policy of the secret.
	RotatePolicy secrets.RotatePolicy
	// NextRotateTime is when the secret should be rotated.
	NextRotateTime *time.Time
	// LatestExpireTime is the expire time of the most recent revision.
	LatestExpireTime *time.Time
	// LatestRevision is the most recent secret revision.
	LatestRevision int
}

// RotationInfo holds information about the rotation of a secret.
type RotationInfo struct {
	URI             *secrets.URI
	Revision        int
	NextTriggerTime time.Time
}

// ExpiryInfo holds information about the expiry of a secret revision.
type ExpiryInfo struct {
	URI             *secrets.URI
	Revision        int
	RevisionID      string
	NextTriggerTime time.Time
}

// ConsumerInfo holds information about a secret consumer.
type ConsumerInfo struct {
	SubjectTypeID   GrantSubjectType
	SubjectID       string
	Label           string
	CurrentRevision int
}

// RemoteSecretInfo holds information about a remote secret
// for a given consumer.
type RemoteSecretInfo struct {
	URI             *secrets.URI
	SubjectTypeID   GrantSubjectType
	SubjectID       string
	Label           string
	CurrentRevision int
	LatestRevision  int
}
