// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain

import (
	"testing"

	gc "gopkg.in/check.v1"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade Context,Authorizer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/controller/usersecretsdrain SecretsState

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

var NewUserSecretsDrainAPI = newUserSecretsDrainAPI

func NewTestAPI(
	secretsState SecretsState,
	backendConfigGetter commonsecrets.BackendConfigGetter,
	drainConfigGetter commonsecrets.BackendDrainConfigGetter,
) (*SecretsDrainAPI, error) {
	return &SecretsDrainAPI{
		secretsState:        secretsState,
		backendConfigGetter: backendConfigGetter,
		drainConfigGetter:   drainConfigGetter,
	}, nil
}
