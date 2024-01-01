// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorupgrader

import (
	"github.com/juju/loggo"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/rpc/params"
)

var logger = loggo.GetLogger("juju.controller.caasoperatorupgrader")

type API struct {
	auth facade.Authorizer

	broker caas.Upgrader
}

// NewCAASOperatorUpgraderAPI returns a new CAAS operator upgrader API facade.
func NewCAASOperatorUpgraderAPI(
	authorizer facade.Authorizer,
	broker caas.Upgrader,
) (*API, error) {
	if !authorizer.AuthController() &&
		!authorizer.AuthApplicationAgent() &&
		!authorizer.AuthUnitAgent() && // For sidecar applications.
		!authorizer.AuthModelAgent() {
		return nil, apiservererrors.ErrPerm
	}
	return &API{
		auth:   authorizer,
		broker: broker,
	}, nil
}

// UpgradeOperator upgrades the operator for the specified agents.
func (api *API) UpgradeOperator(arg params.KubernetesUpgradeArg) (params.ErrorResult, error) {
	serverErr := func(err error) params.ErrorResult {
		return params.ErrorResult{Error: apiservererrors.ServerError(err)}
	}
	tag, err := names.ParseTag(arg.AgentTag)
	if err != nil {
		return serverErr(err), nil
	}
	if !api.auth.AuthOwner(tag) {
		return serverErr(apiservererrors.ErrPerm), nil
	}

	logger.Debugf("upgrading caas agent for %s", tag)
	err = api.broker.Upgrade(arg.AgentTag, arg.Version)
	if err != nil {
		return serverErr(err), nil
	}
	return params.ErrorResult{}, nil
}
