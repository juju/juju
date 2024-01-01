// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/state"
)

// SecretsState instances provide secret apis.
type SecretsState interface {
	CreateSecret(*secrets.URI, state.CreateSecretParams) (*secrets.SecretMetadata, error)
	UpdateSecret(*secrets.URI, state.UpdateSecretParams) (*secrets.SecretMetadata, error)
	GetSecret(uri *secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretValue(*secrets.URI, int) (secrets.SecretValue, *secrets.ValueRef, error)
	GetSecretRevision(uri *secrets.URI, revision int) (*secrets.SecretRevisionMetadata, error)
	DeleteSecret(*secrets.URI, ...int) ([]secrets.ValueRef, error)
	ListSecrets(state.SecretsFilter) ([]*secrets.SecretMetadata, error)
	ListSecretRevisions(uri *secrets.URI) ([]*secrets.SecretRevisionMetadata, error)
	ListUnusedSecretRevisions(uri *secrets.URI) ([]int, error)
	SecretGrants(uri *secrets.URI, role secrets.SecretRole) ([]secrets.AccessInfo, error)
}

// SecretsConsumer instances provide secret consumer apis.
type SecretsConsumer interface {
	GrantSecretAccess(*secrets.URI, state.SecretAccessParams) error
	RevokeSecretAccess(*secrets.URI, state.SecretAccessParams) error
	SecretAccess(uri *secrets.URI, subject names.Tag) (secrets.SecretRole, error)
}
