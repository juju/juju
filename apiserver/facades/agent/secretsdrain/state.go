// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

import (
	"context"

	"github.com/juju/names/v5"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// SecretsConsumer instances provide secret consumer apis.
type SecretsConsumer interface {
	SecretAccess(uri *secrets.URI, subject names.Tag) (secrets.SecretRole, error)
}

// SecretsState instances provide secret state apis.
type SecretsState interface {
	ListSecrets(state.SecretsFilter) ([]*secrets.SecretMetadata, error)
	ListSecretRevisions(uri *secrets.URI) ([]*secrets.SecretRevisionMetadata, error)
	ChangeSecretBackend(state.ChangeSecretBackendParams) error
}

// Model provides the subset of state.Model that is required by the secrets drain apis.
type Model interface {
	ModelConfig(context.Context) (*config.Config, error)
	Type() state.ModelType
	WatchForModelConfigChanges() state.NotifyWatcher
	UUID() string
	ControllerUUID() string
}
