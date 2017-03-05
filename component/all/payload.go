// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"reflect"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/api/client"
	internalclient "github.com/juju/juju/payload/api/private/client"
	internalserver "github.com/juju/juju/payload/api/private/server"
	"github.com/juju/juju/payload/context"
	"github.com/juju/juju/payload/status"
	"github.com/juju/juju/state"
	unitercontext "github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

const payloadsHookContextFacade = "PayloadsHookContext"

type payloads struct{}

func (c payloads) registerForServer() error {
	c.registerHookContext()
	return nil
}

func (c payloads) registerForClient() error {
	c.registerPublicCommands()
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
	caller := base.NewFacadeCallerForVersion(apiCaller, payload.FacadeName, 1)

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

// XXX move to apiserver
func (c payloads) registerHookContext() {
	if !markRegistered(payload.ComponentName, "hook-context") {
		return
	}

	unitercontext.RegisterComponentFunc(payload.ComponentName,
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
	c.registerHookContextFacade()
}

func (payloads) newUnitFacadeClient(caller base.APICaller) context.APIClient {
	facadeCaller := base.NewFacadeCallerForVersion(caller, payloadsHookContextFacade, 1)
	return internalclient.NewUnitFacadeClient(facadeCaller)
}

func (payloads) newHookContextFacade(st *state.State, unit *state.Unit) (interface{}, error) {
	up, err := st.UnitPayloads(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return internalserver.NewUnitFacade(up), nil
}

func (c payloads) registerHookContextFacade() {
	const version = 1
	common.RegisterHookContextFacade(
		payloadsHookContextFacade,
		version,
		c.newHookContextFacade,
		reflect.TypeOf(&internalserver.UnitFacade{}),
	)
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
