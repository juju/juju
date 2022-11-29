// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/state"
)

// SecretsBackendState is used to access the juju state database.
type SecretsBackendState interface {
	CreateSecretBackend(params state.CreateSecretBackendParams) error
	ListSecretBackends() ([]*secrets.SecretBackend, error)
}
