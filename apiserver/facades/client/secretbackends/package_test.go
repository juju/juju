// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"testing"

	"github.com/juju/clock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsbackendstate.go github.com/juju/juju/apiserver/facades/client/secretbackends SecretsBackendState
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretstate.go github.com/juju/juju/apiserver/facades/client/secretbackends SecretsState
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state.go github.com/juju/juju/apiserver/facades/client/secretbackends StatePool
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/provider_mock.go github.com/juju/juju/internal/secrets/provider SecretBackendProvider,SecretsBackend
func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func NewTestAPI(
	backendState SecretsBackendState,
	secretState SecretsState,
	statePool StatePool,
	authorizer facade.Authorizer,
	clock clock.Clock,
) (*SecretBackendsAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretBackendsAPI{
		clock:          clock,
		authorizer:     authorizer,
		controllerUUID: coretesting.ControllerTag.Id(),
		statePool:      statePool,
		backendState:   backendState,
		secretState:    secretState,
	}, nil
}
