// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"

	"github.com/juju/juju/core/secrets"
)

// The following interfaces are used to access secret services.

type SecretService interface {
	UpdateRemoteSecretRevision(ctx context.Context, uri *secrets.URI, latestRevision int) error
	RemoveRemoteSecretConsumer(ctx context.Context, unitName string) error
}
