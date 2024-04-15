// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
)

// The following interfaces are used to access secret services.

type SecretService interface {
	RemoveRemoteSecretConsumer(ctx context.Context, unitName string) error
}
