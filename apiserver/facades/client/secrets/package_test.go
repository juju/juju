// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"
	"testing"

	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/secrets/provider"
	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsstate.go github.com/juju/juju/apiserver/facades/client/secrets SecretsState,SecretsConsumer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsbackend.go github.com/juju/juju/internal/secrets/provider SecretsBackend,SecretBackendProvider
func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func NewTestAPI(
	authTag names.Tag,
	authorizer facade.Authorizer,
	secretsState SecretsState,
	secretsConsumer SecretsConsumer,
	adminBackendConfigGetter func(ctx context.Context) (*provider.ModelBackendConfigInfo, error),
	backendConfigGetterForUserSecretsWrite func(ctx context.Context, backendID string) (*provider.ModelBackendConfigInfo, error),
	backendGetter func(context.Context, *provider.ModelBackendConfig) (provider.SecretsBackend, error),
) (*SecretsAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretsAPI{
		authTag:                                authTag,
		authorizer:                             authorizer,
		controllerUUID:                         coretesting.ControllerTag.Id(),
		modelUUID:                              coretesting.ModelTag.Id(),
		secretsState:                           secretsState,
		secretsConsumer:                        secretsConsumer,
		backends:                               make(map[string]provider.SecretsBackend),
		adminBackendConfigGetter:               adminBackendConfigGetter,
		backendConfigGetterForUserSecretsWrite: backendConfigGetterForUserSecretsWrite,
		backendGetter:                          backendGetter,
	}, nil
}
