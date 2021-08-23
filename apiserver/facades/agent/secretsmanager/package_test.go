// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"testing"

	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/secrets"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretservice.go github.com/juju/juju/secrets SecretsService

func NewTestAPI(
	service secrets.SecretsService,
	authorizer facade.Authorizer,
) (*SecretsManagerAPI, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretsManagerAPI{
		controllerUUID: coretesting.ControllerTag.Id(),
		modelUUID:      coretesting.ModelTag.Id(),
		secretsService: service,
	}, nil
}
