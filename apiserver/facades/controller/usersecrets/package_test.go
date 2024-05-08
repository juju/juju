// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecrets

import (
	"testing"

	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/service.go github.com/juju/juju/apiserver/facades/controller/usersecrets SecretService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/watcher.go github.com/juju/juju/core/watcher NotifyWatcher

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func NewTestAPI(
	authorizer facade.Authorizer,
	resources facade.Resources,
	secretService SecretService,
) (*UserSecretsManager, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &UserSecretsManager{
		authorizer:    authorizer,
		resources:     resources,
		secretService: secretService,
	}, nil
}
