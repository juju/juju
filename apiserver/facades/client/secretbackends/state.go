// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

// // SecretsBackendState is used to access the juju state database.
// type SecretsBackendState interface {
// 	CreateSecretBackend(params state.CreateSecretBackendParams) (string, error)
// 	UpdateSecretBackend(params state.UpdateSecretBackendParams) error
// 	DeleteSecretBackend(name string, force bool) error
// 	ListSecretBackends() ([]*secrets.SecretBackend, error)
// 	GetSecretBackend(name string) (*secrets.SecretBackend, error)
// 	GetSecretBackendByID(ID string) (*secrets.SecretBackend, error)
// }

type SecretsState interface {
	ListModelSecrets(all bool) (map[string]set.Strings, error)
}

type StatePool interface {
	GetModel(modelUUID string) (common.Model, func() bool, error)
}

type statePoolShim struct {
	pool *state.StatePool
}

func (s *statePoolShim) GetModel(modelUUID string) (common.Model, func() bool, error) {
	m, hp, err := s.pool.GetModel(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return m, hp.Release, nil
}

type Model interface {
	UUID() string
}
