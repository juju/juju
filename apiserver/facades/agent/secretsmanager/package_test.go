// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/names/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/secrets"
	coretesting "github.com/juju/juju/testing"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretservice.go github.com/juju/juju/secrets SecretsService
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsrotationservice.go github.com/juju/juju/apiserver/facades/agent/secretsmanager SecretsRotation
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/secretsrotationwatcher.go github.com/juju/juju/state SecretsRotationWatcher

func NewTestAPI(
	authorizer facade.Authorizer,
	resources facade.Resources,
	service secrets.SecretsService,
	secretsRotation SecretsRotation,
	accessSecret common.GetAuthFunc,
	ownerTag names.Tag,
	clock clock.Clock,
) (*SecretsManagerAPI, error) {
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return &SecretsManagerAPI{
		authOwner:       ownerTag,
		controllerUUID:  coretesting.ControllerTag.Id(),
		modelUUID:       coretesting.ModelTag.Id(),
		resources:       resources,
		secretsService:  service,
		secretsRotation: secretsRotation,
		accessSecret:    accessSecret,
		clock:           clock,
	}, nil
}
