// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorupgrader

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/state/stateenvirons"
)

var logger = loggo.GetLogger("juju.controller.caasoperatorupgrader")

type API struct {
	auth facade.Authorizer

	broker caas.Upgrader
}

// NewStateCAASOperatorUpgraderAPI provides the signature required for facade registration.
func NewStateCAASOperatorUpgraderAPI(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(model)
	if err != nil {
		return nil, errors.Annotate(err, "getting caas client")
	}
	return NewCAASOperatorUpgraderAPI(authorizer, broker)
}

// NewCAASOperatorUpgraderAPI returns a new CAAS operator upgrader API facade.
func NewCAASOperatorUpgraderAPI(
	authorizer facade.Authorizer,
	broker caas.Upgrader,
) (*API, error) {
	if !authorizer.AuthController() && !authorizer.AuthApplicationAgent() {
		return nil, common.ErrPerm
	}
	return &API{
		auth:   authorizer,
		broker: broker,
	}, nil
}

// UpgradeOperator upgrades the operator for the specified agents.
func (api *API) UpgradeOperator(arg params.KubernetesUpgradeArg) (params.ErrorResult, error) {
	serverErr := func(err error) params.ErrorResult {
		return params.ErrorResult{common.ServerError(err)}
	}
	tag, err := names.ParseTag(arg.AgentTag)
	if err != nil {
		return serverErr(err), nil
	}
	if !api.auth.AuthOwner(tag) {
		return serverErr(common.ErrPerm), nil
	}
	appName := tag.Id()

	// Nodes representing controllers really mean the controller operator.
	if tag.Kind() == names.MachineTagKind || tag.Kind() == names.ControllerAgentTagKind {
		appName = bootstrap.ControllerModelName
	}
	logger.Debugf("upgrading caas app %v", appName)
	err = api.broker.Upgrade(appName, arg.Version)
	if err != nil {
		return serverErr(err), nil
	}
	return params.ErrorResult{}, nil
}
