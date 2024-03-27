// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/secrets"
)

func (s *SecretService) GetSecretGrants(ctx context.Context, uri *secrets.URI, role secrets.SecretRole) ([]secrets.AccessInfo, error) {
	panic("implement me")
}

func (s *SecretService) GetSecretAccess(ctx context.Context, uri *secrets.URI, consumer SecretAccessor) (secrets.SecretRole, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SecretService) GetSecretAccessScope(ctx context.Context, uri *secrets.URI) (SecretConsumer, error) {
	panic("implement me")
}

func (s *SecretService) GrantSecretAccess(ctx context.Context, uri *secrets.URI, params SecretAccessParams) error {
	panic("implement me")
}

func (s *SecretService) RevokeSecretAccess(ctx context.Context, uri *secrets.URI, params SecretAccessParams) error {
	panic("implement me")
}
