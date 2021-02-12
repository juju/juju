// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	jujucmd "github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	internalclient "github.com/juju/juju/api/resources/private/client"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/context"
	contextcmd "github.com/juju/juju/resource/context/cmd"
	unitercontext "github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// resources exposes the registration methods needed
// for the top-level component machinery.
type resources struct{}

// RegisterForServer is the top-level registration method
// for the component in a jujud context.
func (r resources) registerForServer() error {
	r.registerState()
	r.registerAgentWorkers()
	r.registerHookContext()
	return nil
}

// RegisterForClient is the top-level registration method
// for the component in a "juju" command context.
func (r resources) registerForClient() error {
	// needed for hook-tool
	r.registerHookContextCommands()
	return nil
}

// registerAgentWorkers adds the resources workers to the agents.
func (r resources) registerAgentWorkers() {
	if !markRegistered(resource.ComponentName, "agent-workers") {
		return
	}
}

// registerState registers the state functionality for resources.
func (resources) registerState() {
	if !markRegistered(resource.ComponentName, "state") {
		return
	}
}

func (r resources) registerHookContext() {
	if markRegistered(resource.ComponentName, "hook-context") == false {
		return
	}

	_ = unitercontext.RegisterComponentFunc(
		resource.ComponentName,
		func(config unitercontext.ComponentConfig) (jujuc.ContextComponent, error) {
			unitID := names.NewUnitTag(config.UnitName).String()
			hctxClient, err := r.newUnitFacadeClient(unitID, config.APICaller)
			if err != nil {
				return nil, errors.Trace(err)
			}
			// TODO(ericsnow) Pass the unit's tag through to the component?
			return context.NewContextAPI(hctxClient, config.DataDir), nil
		},
	)

	r.registerHookContextCommands()
}

func (r resources) registerHookContextCommands() {
	if markRegistered(resource.ComponentName, "hook-context-commands") == false {
		return
	}

	jujuc.RegisterCommand(
		contextcmd.GetCmdName,
		func(ctx jujuc.Context) (jujucmd.Command, error) {
			compCtx, err := ctx.Component(resource.ComponentName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			cmd, err := contextcmd.NewGetCmd(compCtx)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return cmd, nil
		},
	)
}

func (r resources) newUnitFacadeClient(unitName string, caller base.APICaller) (context.APIClient, error) {

	facadeCaller := base.NewFacadeCallerForVersion(caller, context.HookContextFacade, 1)
	httpClient, err := caller.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitHTTPClient := internalclient.NewUnitHTTPClient(caller.Context(), httpClient, unitName)

	return internalclient.NewUnitFacadeClient(facadeCaller.RawAPICaller().Context(), facadeCaller, unitHTTPClient), nil
}
