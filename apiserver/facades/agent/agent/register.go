// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Agent", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newAgentAPIV3(ctx)
	}, reflect.TypeOf((*AgentAPI)(nil)))
}

// newAgentAPIV3 returns an object implementing version 3 of the Agent API
// with the given authorizer representing the currently logged in client.
func newAgentAPIV3(ctx facade.Context) (*AgentAPI, error) {
	auth := ctx.Auth()
	// Agents are defined to be any user that's not a client user.
	if !auth.AuthMachineAgent() && !auth.AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}
	getCanChange := func() (common.AuthFunc, error) {
		return auth.AuthOwner, nil
	}

	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	serviceFactory := ctx.ServiceFactory()
	controllerConfigGetter := serviceFactory.ControllerConfig()

	resources := ctx.Resources()
	return &AgentAPI{
		PasswordChanger:   common.NewPasswordChanger(st, getCanChange),
		RebootFlagClearer: common.NewRebootFlagClearer(st, getCanChange),
		ModelWatcher:      common.NewModelWatcher(model, resources, auth),
		ControllerConfigAPI: common.NewControllerConfigAPI(
			st,
			controllerConfigGetter,
			serviceFactory.ExternalController(),
		),
		CloudSpecer: cloudspec.NewCloudSpecV2(
			resources,
			cloudspec.MakeCloudSpecGetterForModel(st),
			cloudspec.MakeCloudSpecWatcherForModel(st),
			cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
			cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st),
			common.AuthFuncForTag(model.ModelTag()),
		),
		controllerConfigGetter: controllerConfigGetter,
		st:                     st,
		auth:                   auth,
		resources:              resources,
	}, nil
}
