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

// UpsertSecretParams are used to upsert a secret.
// Only non-nil values are used.
type UpsertSecretParams struct {
	RotatePolicy   *RotatePolicy
	ExpireTime     *time.Time
	NextRotateTime *time.Time
	Description    *string
	Label          *string
	AutoPrune      *bool

	Data     secrets.SecretData
	ValueRef *secrets.ValueRef
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
	ScopeID     string

	SubjectTypeID GrantSubjectType
	SubjectID     string

	RoleID Role
}

// AccessParams are used when querying secret access.
type AccessParams struct {
	SubjectTypeID GrantSubjectType
	SubjectID     string
}

// AccessScope are used when querying secret access scopes.
type AccessScope struct {
	ScopeTypeID GrantScopeType
	ScopeID     string
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
