// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"time"

	"github.com/juju/names/v4"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/state"
)

// SecretsRotation instances provide secret rotation apis.
type SecretsRotation interface {
	WatchSecretsRotationChanges(owner string) state.SecretsRotationWatcher
	SecretRotated(uri *secrets.URI, next time.Time) error
}

// SecretsConsumer instances provide secret consumer apis.
type SecretsConsumer interface {
	GetSecretConsumer(*secrets.URI, string) (*secrets.SecretConsumerMetadata, error)
	SaveSecretConsumer(*secrets.URI, string, *secrets.SecretConsumerMetadata) error
	WatchConsumedSecretsChanges(string) state.StringsWatcher
	GrantSecretAccess(*secrets.URI, state.SecretAccessParams) error
	RevokeSecretAccess(*secrets.URI, state.SecretAccessParams) error
	SecretAccess(uri *secrets.URI, subject names.Tag) (secrets.SecretRole, error)
}

type SecretsStore interface {
	CreateSecret(*secrets.URI, state.CreateSecretParams) (*secrets.SecretMetadata, error)
	UpdateSecret(*secrets.URI, state.UpdateSecretParams) (*secrets.SecretMetadata, error)
	DeleteSecret(*secrets.URI) error
	GetSecret(*secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretValue(*secrets.URI, int) (secrets.SecretValue, error)
	ListSecrets(state.SecretsFilter) ([]*secrets.SecretMetadata, error)
}
