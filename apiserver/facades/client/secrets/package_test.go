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

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsbackend.go github.com/juju/juju/apiserver/facades/client/secrets SecretsBackend
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsstore.go github.com/juju/juju/secrets/provider SecretsStore
func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func NewTestAPI(
	backend SecretsBackend,
	storeGetter func() (provider.SecretsStore, error),
	authorizer facade.Authorizer,
) (*SecretsAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretsAPI{
		authorizer:     authorizer,
		controllerUUID: coretesting.ControllerTag.Id(),
		modelUUID:      coretesting.ModelTag.Id(),
		backend:        backend,
		storeGetter:    storeGetter,
	}, nil
}
