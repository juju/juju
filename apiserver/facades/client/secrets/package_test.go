// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/secretservice.go github.com/juju/juju/apiserver/facades/client/secrets SecretService,SecretBackendService

func NewTestAPI(
	authTag names.Tag,
	authorizer facade.Authorizer,
	secretService SecretService,
	secretBackendService SecretBackendService,
) (*SecretsAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretsAPI{
		authTag:              authTag,
		authorizer:           authorizer,
		controllerUUID:       coretesting.ControllerTag.Id(),
		modelUUID:            coretesting.ModelTag.Id(),
		secretService:        secretService,
		secretBackendService: secretBackendService,
	}, nil
}
