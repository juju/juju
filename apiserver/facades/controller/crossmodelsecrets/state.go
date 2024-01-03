// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/state"
)

// The following interfaces are used to access backend state.

type SecretsState interface {
	GetSecret(uri *secrets.URI) (*secrets.SecretMetadata, error)
	GetSecretValue(*secrets.URI, int) (secrets.SecretValue, *secrets.ValueRef, error)
}

type SecretsConsumer interface {
	GetSecretRemoteConsumer(*secrets.URI, names.Tag) (*secrets.SecretConsumerMetadata, error)
	SaveSecretRemoteConsumer(*secrets.URI, names.Tag, *secrets.SecretConsumerMetadata) error
	SecretAccess(uri *secrets.URI, subject names.Tag) (secrets.SecretRole, error)
	SecretAccessScope(uri *secrets.URI, subject names.Tag) (names.Tag, error)
}

type CrossModelState interface {
	GetRemoteApplicationTag(string) (names.Tag, error)
	GetToken(entity names.Tag) (string, error)
}

type StateBackend interface {
	HasEndpoint(key string, app string) (bool, error)
}

type stateBackendShim struct {
	*state.State
}

func (s *stateBackendShim) HasEndpoint(key string, app string) (bool, error) {
	rel, err := s.State.KeyRelation(key)
	if err != nil {
		return false, errors.Trace(err)
	}
	if rel.Suspended() {
		return false, nil
	}
	_, err = rel.Endpoint(app)
	return err == nil, nil
}

type crossModelShim struct {
	*state.RemoteEntities
}

// GetRemoteApplicationTag returns the consumer remote application
// tag for the token.
func (s *crossModelShim) GetRemoteApplicationTag(token string) (names.Tag, error) {
	return s.RemoteEntities.GetRemoteEntity(token)
}
