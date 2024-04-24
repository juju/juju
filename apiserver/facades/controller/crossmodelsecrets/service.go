// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"context"

	"github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
)

// The following interfaces are used to access secret services.

type SecretService interface {
	GetSecret(context.Context, *secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretValue(context.Context, *secrets.URI, int, secretservice.SecretAccessor) (secrets.SecretValue, *secrets.ValueRef, error)
	UpdateRemoteConsumedRevision(ctx context.Context, uri *secrets.URI, unitName string, refresh bool) (int, error)
	GetSecretAccessScope(ctx context.Context, uri *secrets.URI, accessor secretservice.SecretAccessor) (secretservice.SecretAccessScope, error)
}
