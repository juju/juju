// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/secrets"
)

type SecretsState interface {
	GetSecret(uri *secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretValue(*secrets.URI, int) (secrets.SecretValue, *secrets.ValueRef, error)
}

type SecretsConsumer interface {
	SecretAccess(uri *secrets.URI, subject names.Tag) (secrets.SecretRole, error)
}

type CrossModelState interface {
	GetRemoteEntity(string) (names.Tag, error)
}
