// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

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

func NewTestAPI(
	service secrets.SecretsService,
	authorizer facade.Authorizer,
) (*SecretsAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretsAPI{
		authorizer:     authorizer,
		controllerUUID: coretesting.ControllerTag.Id(),
		modelUUID:      coretesting.ModelTag.Id(),
		secretsService: service,
	}, nil
}
