// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets

import (
	"testing"

	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/secrets/provider"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state.go github.com/juju/juju/apiserver/facades/controller/usersecrets SecretsState
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/watcher.go github.com/juju/juju/state StringsWatcher
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/secretsbackend.go github.com/juju/juju/secrets/provider SecretsBackend,SecretBackendProvider

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func NewTestAPI(
	authorizer facade.Authorizer,
	resources facade.Resources,
	authTag names.Tag,
	controllerUUID string,
	modelUUID string,
	secretsState SecretsState,
	backendConfigGetter func() (*provider.ModelBackendConfigInfo, error),
) (*UserSecretsManager, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &UserSecretsManager{
		authorizer:          authorizer,
		resources:           resources,
		authTag:             authTag,
		controllerUUID:      controllerUUID,
		modelUUID:           modelUUID,
		secretsState:        secretsState,
		backendConfigGetter: backendConfigGetter,
	}, nil
}
