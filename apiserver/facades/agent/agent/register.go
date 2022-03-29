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
	registry.MustRegister("Agent", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newAgentAPIV2(ctx)
	}, reflect.TypeOf((*AgentAPIV2)(nil)))
	registry.MustRegister("Agent", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newAgentAPIV3(ctx)
	}, reflect.TypeOf((*AgentAPIV3)(nil)))
}

// newAgentAPIV2 returns an object implementing version 2 of the Agent API
// with the given authorizer representing the currently logged in client.
func newAgentAPIV2(ctx facade.Context) (*AgentAPIV2, error) {
	v3, err := newAgentAPIV3(ctx)
	if err != nil {
		return nil, err
	}
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	v3.CloudSpecer = cloudspec.NewCloudSpecV1(
		ctx.Resources(),
		cloudspec.MakeCloudSpecGetterForModel(st),
		cloudspec.MakeCloudSpecWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st),
		common.AuthFuncForTag(model.ModelTag()),
	)
	return &AgentAPIV2{
		v3,
	}, nil
}

// newAgentAPIV3 returns an object implementing version 2 of the Agent API
// with the given authorizer representing the currently logged in client.
func newAgentAPIV3(ctx facade.Context) (*AgentAPIV3, error) {
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

	resources := ctx.Resources()
	return &AgentAPIV3{
		PasswordChanger:     common.NewPasswordChanger(st, getCanChange),
		RebootFlagClearer:   common.NewRebootFlagClearer(st, getCanChange),
		ModelWatcher:        common.NewModelWatcher(model, resources, auth),
		ControllerConfigAPI: common.NewStateControllerConfig(st),
		CloudSpecer: cloudspec.NewCloudSpecV2(
			resources,
			cloudspec.MakeCloudSpecGetterForModel(st),
			cloudspec.MakeCloudSpecWatcherForModel(st),
			cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
			cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st),
			common.AuthFuncForTag(model.ModelTag()),
		),
		st:        st,
		auth:      auth,
		resources: resources,
	}, nil
}
