// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
<<<<<<< HEAD
	coretesting "github.com/juju/juju/internal/testing"
=======
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets/provider"
	coretesting "github.com/juju/juju/testing"
>>>>>>> 3.6
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/secretservice.go github.com/juju/juju/apiserver/facades/client/secrets SecretService,SecretBackendService

func NewTestAPI(
	authTag names.Tag,
	authorizer facade.Authorizer,
<<<<<<< HEAD
	secretService SecretService,
	secretBackendService SecretBackendService,
	modelName string,
=======
	secretsState SecretsState,
	secretsConsumer SecretsConsumer,
	adminBackendConfigGetter func() (*provider.ModelBackendConfigInfo, error),
	backendConfigGetterForUserSecretsWrite func(string, []*coresecrets.URI) (*provider.ModelBackendConfigInfo, error),
	backendGetter func(*provider.ModelBackendConfig) (provider.SecretsBackend, error),
>>>>>>> 3.6
) (*SecretsAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretsAPI{
		authTag:              authTag,
		authorizer:           authorizer,
		controllerUUID:       coretesting.ControllerTag.Id(),
		modelUUID:            coretesting.ModelTag.Id(),
		modelName:            modelName,
		secretService:        secretService,
		secretBackendService: secretBackendService,
	}, nil
}
