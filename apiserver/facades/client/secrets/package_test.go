// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"testing"

	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/secrets/provider"
	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsstate.go github.com/juju/juju/apiserver/facades/client/secrets SecretsState,SecretsConsumer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsbackend.go github.com/juju/juju/secrets/provider SecretsBackend
func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func NewTestAPI(
	secretsState SecretsState,
	secretsConsumer SecretsConsumer,
	backendConfigGetter func() (*provider.ModelBackendConfigInfo, error),
	backendGetter func(*provider.ModelBackendConfig) (provider.SecretsBackend, error),
	authorizer facade.Authorizer,
) (*SecretsAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretsAPI{
		authorizer:          authorizer,
		controllerUUID:      coretesting.ControllerTag.Id(),
		modelUUID:           coretesting.ModelTag.Id(),
		secretsState:        secretsState,
		secretsConsumer:     secretsConsumer,
		backends:            make(map[string]provider.SecretsBackend),
		backendConfigGetter: backendConfigGetter,
		backendGetter:       backendGetter,
	}, nil
}
