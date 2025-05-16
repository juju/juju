// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	stdtesting "testing"

	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package secretbackends -destination mock_service.go github.com/juju/juju/apiserver/facades/client/secretbackends SecretBackendService

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

func NewTestAPI(
	authorizer facade.Authorizer,
	backendService SecretBackendService,
) (*SecretBackendsAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretBackendsAPI{
		authorizer:     authorizer,
		controllerUUID: coretesting.ControllerTag.Id(),
		backendService: backendService,
	}, nil
}
