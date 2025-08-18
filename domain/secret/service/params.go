// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/juju/core/secrets"
)

// CreateCharmSecretParams are used to create charm a secret.
type CreateCharmSecretParams struct {
	UpdateCharmSecretParams
	Version int

	CharmOwner CharmSecretOwner
}

// UpdateCharmSecretParams are used to update a charm secret.
type UpdateCharmSecretParams struct {
	Accessor SecretAccessor

	RotatePolicy *secrets.RotatePolicy
	ExpireTime   *time.Time
	Description  *string
	Label        *string
	Params       map[string]interface{}
	Data         secrets.SecretData
	ValueRef     *secrets.ValueRef
	Checksum     string
}

// CreateUserSecretParams are used to create a user secret.
type CreateUserSecretParams struct {
	UpdateUserSecretParams
	Version int
}

// UpdateUserSecretParams are used to update a user secret.
type UpdateUserSecretParams struct {
	Accessor SecretAccessor

	Description *string
	Label       *string
	Params      map[string]interface{}
	Data        secrets.SecretData
	Checksum    string
	AutoPrune   *bool
}

// DeleteSecretParams are used to delete a secret.
type DeleteSecretParams struct {
	Accessor SecretAccessor

	Revisions []int
}

// SecretRotatedParams are used to mark a secret as rotated.
type SecretRotatedParams struct {
	Accessor SecretAccessor

	OriginalRevision int
	Skip             bool
}

// SecretAccessParams are used to define access to a secret.
type SecretAccessParams struct {
	Accessor SecretAccessor

	Scope   SecretAccessScope
	Subject SecretAccessor
	Role    secrets.SecretRole
}

// ChangeSecretBackendParams are used to change the backend of a secret.
type ChangeSecretBackendParams struct {
	Accessor SecretAccessor

	ValueRef *secrets.ValueRef
	Data     secrets.SecretData
}

// SecretAccessorKind represents the kind of an entity which can access a secret.
type SecretAccessorKind string

// These represent the kinds of secret accessor.
const (
	ApplicationAccessor       SecretAccessorKind = "application"
	RemoteApplicationAccessor SecretAccessorKind = "remote-application"
	UnitAccessor              SecretAccessorKind = "unit"
	ModelAccessor             SecretAccessorKind = "model"
)

// GrantedSecretsGetter returns the revisions on the given backend for which
// consumers have access with the given role.
type GrantedSecretsGetter func(
	ctx context.Context, backendID string, role secrets.SecretRole, consumers ...SecretAccessor,
) ([]*secrets.SecretRevisionRef, error)

// SecretAccessor represents an entity that can access a secret.
type SecretAccessor struct {
	Kind SecretAccessorKind
	ID   string
}

// SecretAccessScopeKind represents the kind of an access scope for a secret permission.
type SecretAccessScopeKind string

// These represent the kinds of secret accessor.
const (
	ApplicationAccessScope SecretAccessScopeKind = "application"
	UnitAccessScope        SecretAccessScopeKind = "unit"
	RelationAccessScope    SecretAccessScopeKind = "relation"
	ModelAccessScope       SecretAccessScopeKind = "model"
)

// SecretAccessScope represents the scope of a secret permission.
type SecretAccessScope struct {
	Kind SecretAccessScopeKind
	ID   string
}

// SecretAccess is used to define access to a secret.
type SecretAccess struct {
	Scope   SecretAccessScope
	Subject SecretAccessor
	Role    secrets.SecretRole
}

// CharmSecretOwnerKind represents the kind of a charm secret owner entity.
type CharmSecretOwnerKind string

// These represent the kinds of charm secret owner.
const (
	ApplicationOwner CharmSecretOwnerKind = "application"
	UnitOwner        CharmSecretOwnerKind = "unit"
)

// CharmSecretOwner is the owner of a secret.
// This is used to query or watch secrets for specified owners.
type CharmSecretOwner struct {
	Kind CharmSecretOwnerKind
	ID   string
}

func (o CharmSecretOwner) String() string {
	return fmt.Sprintf("%s-%s", o.Kind, o.ID)
}

// SecretExport defines all the secret data from a model
// which is exported/imported as part of model migration.
type SecretExport struct {
	// Secrets is a slice of the core secret metadata.
	Secrets []*secrets.SecretMetadata
	// Revisions are the secret revisions keyed by secret ID.
	Revisions map[string][]*secrets.SecretRevisionMetadata
	// Content are the locally stored secret content keyed by secret ID.
	Content map[string]map[int]secrets.SecretData
	// Consumers are the secret consumers keyed by secret ID.
	Consumers map[string][]ConsumerInfo
	// RemoteConsumers are the secret remote consumers keyed by secret ID.
	RemoteConsumers map[string][]ConsumerInfo
	// Access are the secret access details keyed by secret ID.
	Access map[string][]SecretAccess
	// RemoteSecrets is a slice of references to cross model secrets.
	RemoteSecrets []RemoteSecret
}

// ConsumerInfo holds information about the consumer of a secret.
type ConsumerInfo struct {
	secrets.SecretConsumerMetadata
	Accessor SecretAccessor
}

// RemoteSecret holds information about a cross model secret.
type RemoteSecret struct {
	URI             *secrets.URI
	Label           string
	CurrentRevision int
	LatestRevision  int
	Accessor        SecretAccessor
}
