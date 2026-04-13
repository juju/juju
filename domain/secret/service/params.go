// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secret"
)

// CreateUserSecretParams are used to create a user secret.
type CreateUserSecretParams struct {
	UpdateUserSecretParams
	Version int
}

// UpdateUserSecretParams are used to update a user secret.
type UpdateUserSecretParams struct {
	Accessor secret.SecretAccessor

	Description *string
	Label       *string
	Params      map[string]any
	Data        secrets.SecretData
	Checksum    string
	AutoPrune   *bool
}

// SecretRotatedParams are used to mark a secret as rotated.
type SecretRotatedParams struct {
	Accessor secret.SecretAccessor

	OriginalRevision int
	Skip             bool
}

// ChangeSecretBackendParams are used to change the backend of a secret.
type ChangeSecretBackendParams struct {
	Accessor secret.SecretAccessor

	ValueRef *secrets.ValueRef
	Data     secrets.SecretData
}

// GrantedSecretsGetter returns the revisions on the given backend for which
// consumers have access with the given role.
type GrantedSecretsGetter func(
	ctx context.Context, backendID string, role secrets.SecretRole, consumers ...secret.SecretAccessor,
) ([]*secrets.SecretRevisionRef, error)

// SecretAccess is used to define access to a secret.
type SecretAccess struct {
	Scope   secret.SecretAccessScope
	Subject secret.SecretAccessor
	Role    secrets.SecretRole
}

// SecretImport defines all the secret data from a model
// which is imported as part of model migration, except for CMR
// secrets which are imported in CMR domain.
type SecretImport struct {
	// Secrets is a slice of the core secret metadata.
	Secrets []*secrets.SecretMetadata
	// Revisions are the secret revisions keyed by secret ID.
	Revisions map[string][]*secrets.SecretRevisionMetadata
	// Content are the locally stored secret content keyed by secret ID.
	Content map[string]map[int]secrets.SecretData
	// Consumers are the secret consumers keyed by secret ID.
	Consumers map[string][]ConsumerInfo
	// Access are the secret access details keyed by secret ID.
	Access map[string][]SecretAccess
}

// ConsumerInfo holds information about the consumer of a secret.
type ConsumerInfo struct {
	secrets.SecretConsumerMetadata
	Accessor secret.SecretAccessor
}

// RemoteSecret holds information about a cross model secret.
type RemoteSecret struct {
	URI             *secrets.URI
	Label           string
	CurrentRevision int
	LatestRevision  int
	Accessor        secret.SecretAccessor
}
