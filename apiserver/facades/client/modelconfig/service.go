// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"
)

// SecretBackendService is an interface for interacting with secret backend service.
type SecretBackendService interface {
	PingSecretBackend(ctx context.Context, name string) error
}
