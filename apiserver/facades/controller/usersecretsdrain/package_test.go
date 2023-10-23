// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain

import (
	"testing"

	gc "gopkg.in/check.v1"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/controller/usersecretsdrain SecretsState

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

var NewUserSecretsDrainAPI = newUserSecretsDrainAPI

func NewTestAPI(
	authorizer facade.Authorizer,
	secretsState SecretsState,
	backendConfigGetter commonsecrets.BackendConfigGetter,
	drainConfigGetter commonsecrets.BackendDrainConfigGetter,
) (*SecretsDrainAPI, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &SecretsDrainAPI{
		secretsState:        secretsState,
		backendConfigGetter: backendConfigGetter,
		drainConfigGetter:   drainConfigGetter,
	}, nil
}
