// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker

import (
	secretsprovider "github.com/juju/juju/secrets/provider"
)

// Getters is used for mocks only.
type Getters interface {
	BackendConfigGetter() (*secretsprovider.ModelBackendConfigInfo, error)
	ProviderGetter(backendType string) (secretsprovider.SecretBackendProvider, error)
}
