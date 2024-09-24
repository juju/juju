// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/storage"
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

// ApplicationService provides access to the application service.
type ApplicationService interface {
	GetUnitLife(ctx context.Context, name string) (life.Value, error)
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
		ctx.DomainServices().ControllerConfig(),
		ctx.DomainServices().ExternalController(),
		ctx.DomainServices().Cloud(),
		ctx.DomainServices().Credential(),
		ctx.DomainServices().Machine(),
		ctx.DomainServices().Config(),
		ctx.DomainServices().Application(service.ApplicationServiceParams{
			StorageRegistry: storage.NotImplementedProviderRegistry{},
			Secrets:         service.NotImplementedSecretService{},
		}),
		ctx.WatcherRegistry(),
	)
}
