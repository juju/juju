// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"testing"

	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coretesting "github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsbackendstate.go github.com/juju/juju/apiserver/facades/client/secretbackends SecretsBackendState
func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func NewTestAPI(
	state SecretsBackendState,
	authorizer facade.Authorizer,
) (*SecretBackendsAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretBackendsAPI{
		authorizer:     authorizer,
		controllerUUID: coretesting.ControllerTag.Id(),
		state:          state,
	}, nil
}
