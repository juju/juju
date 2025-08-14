// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/clock"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/leadership"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

var (
	NewUniterAPI             = newUniterAPI
	NewUniterAPIv19          = newUniterAPIv19
	NewUniterAPIWithServices = newUniterAPIWithServices
)

func NewTestAPI(
	c *tc.C,
	authorizer facade.Authorizer,
	leadership leadership.Checker,
	secretService SecretService,
	applicationService ApplicationService,
	clock clock.Clock,
) (*UniterAPI, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return &UniterAPI{
		auth:               authorizer,
		secretService:      secretService,
		applicationService: applicationService,
		leadershipChecker:  leadership,
		clock:              clock,
		logger:             loggertesting.WrapCheckLog(c),
	}, nil
}

func SetNewContainerBrokerFunc(api *UniterAPI, newBroker caas.NewContainerBrokerFunc) {
	api.containerBrokerFunc = newBroker
}
