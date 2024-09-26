// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import (
	"errors"
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

// SecretMetadata holds metadata about a secret.
type SecretMetadata struct {
	// URI is the URI of the secret.
	URI *secrets.URI
	// Label is the label of the secret.
	Label string
	// Owner is the owner of the secret.
	Owner secrets.Owner
}

// SecretRevision holds metadata and data about a secret revision.
type SecretRevision struct {
	Revision   int
	ValueRef   *secrets.ValueRef
	Data       secrets.SecretData
	CreateTime time.Time
	ExpireTime *time.Time
}

// Validate checks that the metadata is valid.
func (md SecretRevision) Validate() error {
	if md.Data == nil && md.ValueRef == nil {
		return errors.New("data or value reference must be set")
	}
	if md.Data != nil && md.ValueRef != nil {
		return errors.New("only one of data or value reference can be set")
	}
	return nil
}
