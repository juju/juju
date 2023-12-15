// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/state"
)

// SecretsState instances provide secret apis.
type SecretsState interface {
	DeleteSecret(*secrets.URI, ...int) ([]secrets.ValueRef, error)
	GetSecret(*secrets.URI) (*secrets.SecretMetadata, error)
	WatchRevisionsToPrune(owners []names.Tag) (state.StringsWatcher, error)
	GetSecretRevision(uri *secrets.URI, revision int) (*secrets.SecretRevisionMetadata, error)
	ListSecretRevisions(uri *secrets.URI) ([]*secrets.SecretRevisionMetadata, error)
	ListSecrets(state.SecretsFilter) ([]*secrets.SecretMetadata, error)
}
