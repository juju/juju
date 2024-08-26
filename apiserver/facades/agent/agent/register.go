// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/watcher"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Agent", 3, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return NewAgentAPIV3(ctx)
	}, reflect.TypeOf((*AgentAPI)(nil)))
}

type CredentialService interface {
	WatchCredential(ctx context.Context, key credential.Key) (watcher.NotifyWatcher, error)
}

// NewAgentAPIV3 returns an object implementing version 3 of the Agent API
// with the given authorizer representing the currently logged in client.
func NewAgentAPIV3(ctx facade.ModelContext) (*AgentAPI, error) {
	// Agents are defined to be any user that's not a client user.
	if !ctx.Auth().AuthMachineAgent() && !ctx.Auth().AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return NewAgentAPI(
		ctx.Auth(),
		ctx.Resources(),
		ctx.State(),
		ctx.ServiceFactory().ControllerConfig(),
		ctx.ServiceFactory().ExternalController(),
		ctx.ServiceFactory().Cloud(),
		ctx.ServiceFactory().Credential(),
		ctx.ServiceFactory().Machine(),
	)
}
