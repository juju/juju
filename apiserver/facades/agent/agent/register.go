// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"reflect"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Agent", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewAgentAPIV3(ctx)
	}, reflect.TypeOf((*AgentAPI)(nil)))
}

// NewAgentAPIV3 returns an object implementing version 3 of the Agent API
// with the given authorizer representing the currently logged in client.
func NewAgentAPIV3(ctx facade.ModelContext) (*AgentAPI, error) {
	// Agents are defined to be any user that's not a client user.
	if !ctx.Auth().AuthMachineAgent() && !ctx.Auth().AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}

	authFunc := common.AuthFuncForTag(names.NewModelTag(string(ctx.ModelUUID())))
	services := ctx.DomainServices()

	return NewAgentAPI(
		ctx.Auth(),
		authFunc,
		ctx.Resources(),
		ctx.State(),
		services.Password(),
		services.ControllerConfig(),
		services.ExternalController(),
		services.Cloud(),
		services.Credential(),
		services.Machine(),
		services.Config(),
		services.Application(),
		ctx.WatcherRegistry(),
	), nil
}
