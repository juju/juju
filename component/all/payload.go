// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"reflect"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/api/client"
	internalclient "github.com/juju/juju/payload/api/private/client"
	internalserver "github.com/juju/juju/payload/api/private/server"
	"github.com/juju/juju/payload/api/server"
	"github.com/juju/juju/payload/context"
	"github.com/juju/juju/payload/persistence"
	payloadstate "github.com/juju/juju/payload/state"
	"github.com/juju/juju/payload/status"
	"github.com/juju/juju/state"
	unitercontext "github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

const payloadsHookContextFacade = payload.ComponentName + "-hook-context"

type payloads struct{}

func (c payloads) registerForServer() error {
	c.registerState()
	c.registerPublicFacade()

	c.registerHookContext()

	return nil
}

func (c payloads) registerForClient() error {
	c.registerPublicCommands()
	return nil
}

func (payloads) newPublicFacade(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*server.PublicAPI, error) {
	up, err := st.EnvPayloads()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return server.NewPublicAPI(up), nil
}

func (c payloads) registerPublicFacade() {
	if !markRegistered(payload.ComponentName, "public-facade") {
		return
	}

	const version = 1
	common.RegisterStandardFacade(
		payload.ComponentName,
		version,
		c.newPublicFacade,
	)
	api.RegisterFacadeVersion(payload.ComponentName, version)
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
	caller := base.NewFacadeCallerForVersion(apiCaller, payload.ComponentName, 0)

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
	facadeCaller := base.NewFacadeCallerForVersion(caller, payloadsHookContextFacade, 0)
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
	const version = 0
	common.RegisterHookContextFacade(
		payloadsHookContextFacade,
		version,
		c.newHookContextFacade,
		reflect.TypeOf(&internalserver.UnitFacade{}),
	)
	api.RegisterFacadeVersion(payloadsHookContextFacade, version)
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

func (payloads) registerState() {
	if !markRegistered(payload.ComponentName, "state") {
		return
	}

	// TODO(ericsnow) Use a more general registration mechanism.
	//state.RegisterMultiEnvCollections(persistence.Collections...)

	newUnitPayloads := func(persist state.Persistence, unit, machine string) (state.UnitPayloads, error) {
		return payloadstate.NewUnitPayloads(persist, unit, machine), nil
	}

	newEnvPayloads := func(persist state.PayloadsEnvPersistence) (state.EnvPayloads, error) {
		envPersist := persistence.NewEnvPersistence(persist)
		envPayloads := payloadstate.EnvPayloads{
			Persist: envPersist,
		}
		return envPayloads, nil
	}

	state.SetPayloadsComponent(newEnvPayloads, newUnitPayloads)
}
