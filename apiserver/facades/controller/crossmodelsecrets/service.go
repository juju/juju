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
	GetSecretValue(context.Context, *secrets.URI, int) (secrets.SecretValue, *secrets.ValueRef, error)
	GetSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, unitName string) (*secrets.SecretConsumerMetadata, error)
	SaveSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, unitName string, md *secrets.SecretConsumerMetadata) error
	GetSecretAccess(ctx context.Context, uri *secrets.URI, consumer secretservice.SecretAccessor) (secrets.SecretRole, error)
	GetSecretAccessScope(ctx context.Context, uri *secrets.URI, accessor secretservice.SecretAccessor) (secretservice.SecretAccessScope, error)
}
