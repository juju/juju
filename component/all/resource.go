// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"os"

	jujucmd "github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	"github.com/juju/juju/cmd/juju/charmcmd"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/resource"
	internalclient "github.com/juju/juju/resource/api/private/client"
	"github.com/juju/juju/resource/cmd"
	"github.com/juju/juju/resource/context"
	contextcmd "github.com/juju/juju/resource/context/cmd"
	"github.com/juju/juju/resource/resourceadapters"
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
	r.registerPublicCommands()

	// needed for help-tool
	r.registerHookContextCommands()
	return nil
}

// registerAgentWorkers adds the resources workers to the agents.
func (r resources) registerAgentWorkers() {
	if !markRegistered(resource.ComponentName, "agent-workers") {
		return
	}

	charmrevisionupdater.RegisterLatestCharmHandler("resources", resourceadapters.NewLatestCharmHandler)
}

// registerState registers the state functionality for resources.
func (resources) registerState() {
	if !markRegistered(resource.ComponentName, "state") {
		return
	}
}

// registerPublicCommands adds the resources-related commands
// to the "juju" supercommand.
func (r resources) registerPublicCommands() {
	if !markRegistered(resource.ComponentName, "public-commands") {
		return
	}

	charmcmd.RegisterSubCommand(cmd.NewListCharmResourcesCommand())

	commands.RegisterEnvCommand(func() modelcmd.ModelCommand {
		return cmd.NewUploadCommand(cmd.UploadDeps{
			NewClient: func(c *cmd.UploadCommand) (cmd.UploadClient, error) {
				apiRoot, err := c.NewAPIRoot()
				if err != nil {
					return nil, errors.Trace(err)
				}
				return resourceadapters.NewAPIClient(apiRoot)
			},
			OpenResource: func(s string) (cmd.ReadSeekCloser, error) {
				return os.Open(s)
			},
		})

	})

	commands.RegisterEnvCommand(func() modelcmd.ModelCommand {
		return cmd.NewShowServiceCommand(cmd.ShowServiceDeps{
			NewClient: func(c *cmd.ShowServiceCommand) (cmd.ShowServiceClient, error) {
				apiRoot, err := c.NewAPIRoot()
				if err != nil {
					return nil, errors.Trace(err)
				}
				return resourceadapters.NewAPIClient(apiRoot)
			},
		})
	})
}

func (r resources) registerHookContext() {
	if markRegistered(resource.ComponentName, "hook-context") == false {
		return
	}

	unitercontext.RegisterComponentFunc(
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
	unitHTTPClient := internalclient.NewUnitHTTPClient(httpClient, unitName)

	return internalclient.NewUnitFacadeClient(facadeCaller, unitHTTPClient), nil
}
