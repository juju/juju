// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/api/client"
	internalclient "github.com/juju/juju/payload/api/private/client"
	"github.com/juju/juju/payload/context"
	"github.com/juju/juju/payload/status"
	unitercontext "github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type payloads struct{}

func (c payloads) registerForServer() error {
	c.registerHookContext()
	return nil
}

func (c payloads) registerForClient() error {
	c.registerPublicCommands()
	// needed for hook-tool
	c.registerHookContextCommands()
	return nil
}

func (c payloads) registerForContainerAgent() error {
	return nil
}

type facadeCaller struct {
	base.FacadeCaller
	closeFunc func() error
}

func (c facadeCaller) Close() error {
	return c.closeFunc()
}

func (payloads) newListAPIClient(cmd *status.ListCommand) (status.ListAPI, error) {
	apiCaller, err := cmd.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	caller := base.NewFacadeCallerForVersion(apiCaller, "Payloads", 1)

	listAPI := client.NewPublicClient(&facadeCaller{
		FacadeCaller: caller,
		closeFunc:    apiCaller.Close,
	})
	return listAPI, nil
}

func (c payloads) registerPublicCommands() {
	if !markRegistered(payload.ComponentName, "public-commands") {
		return
	}

	commands.RegisterEnvCommand(func() modelcmd.ModelCommand {
		return status.NewListCommand(c.newListAPIClient)
	})
}

func (c payloads) registerHookContext() {
	if !markRegistered(payload.ComponentName, "hook-context") {
		return
	}

	_ = unitercontext.RegisterComponentFunc(payload.ComponentName,
		func(config unitercontext.ComponentConfig) (jujuc.ContextComponent, error) {
			hctxClient := c.newUnitFacadeClient(config.APICaller)
			// TODO(ericsnow) Pass the unit's tag through to the component?
			component, err := context.NewContextAPI(hctxClient, config.DataDir)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return component, nil
		},
	)

	c.registerHookContextCommands()
}

type payloadsHookContext struct {
	jujuc.Context
}

// Component implements context.HookContext.
func (c payloadsHookContext) Component(name string) (context.Component, error) {
	found, err := c.Context.Component(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	compCtx, ok := found.(context.Component)
	if !ok && found != nil {
		return nil, errors.Errorf("wrong component context type registered: %T", found)
	}
	return compCtx, nil
}

func (payloads) newUnitFacadeClient(caller base.APICaller) context.APIClient {
	facadeCaller := base.NewFacadeCallerForVersion(caller, "PayloadsHookContext", 1)
	return internalclient.NewUnitFacadeClient(facadeCaller)
}

func (payloads) registerHookContextCommands() {
	if !markRegistered(payload.ComponentName, "hook-context-commands") {
		return
	}

	jujuc.RegisterCommand(context.RegisterCmdName, func(ctx jujuc.Context) (cmd.Command, error) {
		compCtx := payloadsHookContext{ctx}
		cmd, err := context.NewRegisterCmd(compCtx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cmd, nil
	})

	jujuc.RegisterCommand(context.StatusSetCmdName, func(ctx jujuc.Context) (cmd.Command, error) {
		compCtx := payloadsHookContext{ctx}
		cmd, err := context.NewStatusSetCmd(compCtx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cmd, nil
	})

	jujuc.RegisterCommand(context.UnregisterCmdName, func(ctx jujuc.Context) (cmd.Command, error) {
		compCtx := payloadsHookContext{ctx}
		cmd, err := context.NewUnregisterCmd(compCtx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cmd, nil
	})
}
