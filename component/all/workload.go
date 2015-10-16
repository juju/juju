// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"reflect"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api/client"
	internalclient "github.com/juju/juju/workload/api/internal/client"
	internalserver "github.com/juju/juju/workload/api/internal/server"
	"github.com/juju/juju/workload/api/server"
	"github.com/juju/juju/workload/context"
	"github.com/juju/juju/workload/persistence"
	workloadstate "github.com/juju/juju/workload/state"
	"github.com/juju/juju/workload/status"
)

const workloadsHookContextFacade = workload.ComponentName + "-hook-context"

type workloads struct{}

func (c workloads) registerForServer() error {
	c.registerState()
	c.registerPublicFacade()

	c.registerHookContext()

	return nil
}

func (c workloads) registerForClient() error {
	c.registerPublicCommands()
	return nil
}

func (workloads) newPublicFacade(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*server.PublicAPI, error) {
	up, err := st.EnvPayloads()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return server.NewPublicAPI(up), nil
}

func (c workloads) registerPublicFacade() {
	common.RegisterStandardFacade(
		workload.ComponentName,
		0,
		c.newPublicFacade,
	)
}

type facadeCaller struct {
	base.FacadeCaller
	closeFunc func() error
}

func (c facadeCaller) Close() error {
	return c.closeFunc()
}

func (workloads) newListAPIClient(cmd *status.ListCommand) (status.ListAPI, error) {
	apiCaller, err := cmd.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	caller := base.NewFacadeCallerForVersion(apiCaller, workload.ComponentName, 0)

	listAPI := client.NewPublicClient(&facadeCaller{
		FacadeCaller: caller,
		closeFunc:    apiCaller.Close,
	})
	return listAPI, nil
}

func (c workloads) registerPublicCommands() {
	if !markRegistered(workload.ComponentName, "public-commands") {
		return
	}

	commands.RegisterEnvCommand(func() envcmd.EnvironCommand {
		return status.NewListCommand(c.newListAPIClient)
	})
}

func (c workloads) registerHookContext() {
	if !markRegistered(workload.ComponentName, "hook-context") {
		return
	}

	runner.RegisterComponentFunc(workload.ComponentName,
		func(config runner.ComponentConfig) (jujuc.ContextComponent, error) {
			hctxClient := c.newHookContextAPIClient(config.APICaller)
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

func (workloads) newHookContextAPIClient(caller base.APICaller) context.APIClient {
	facadeCaller := base.NewFacadeCallerForVersion(caller, workloadsHookContextFacade, 0)
	return internalclient.NewHookContextClient(facadeCaller)
}

func (workloads) newHookContextFacade(st *state.State, unit *state.Unit) (interface{}, error) {
	up, err := st.UnitWorkloads(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return internalserver.NewHookContextAPI(up), nil
}

func (c workloads) registerHookContextFacade() {
	common.RegisterHookContextFacade(
		workloadsHookContextFacade,
		0,
		c.newHookContextFacade,
		reflect.TypeOf(&internalserver.HookContextAPI{}),
	)
}

type workloadsHookContext struct {
	jujuc.Context
}

// Component implements context.HookContext.
func (c workloadsHookContext) Component(name string) (context.Component, error) {
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

func (workloads) registerHookContextCommands() {
	if !markRegistered(workload.ComponentName, "hook-context-commands") {
		return
	}

	jujuc.RegisterCommand(context.RegisterCmdName, func(ctx jujuc.Context) (cmd.Command, error) {
		compCtx := workloadsHookContext{ctx}
		cmd, err := context.NewRegisterCmd(compCtx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cmd, nil
	})

	jujuc.RegisterCommand(context.StatusSetCmdName, func(ctx jujuc.Context) (cmd.Command, error) {
		compCtx := workloadsHookContext{ctx}
		cmd, err := context.NewStatusSetCmd(compCtx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cmd, nil
	})

	jujuc.RegisterCommand(context.UnregisterCmdName, func(ctx jujuc.Context) (cmd.Command, error) {
		compCtx := workloadsHookContext{ctx}
		cmd, err := context.NewUnregisterCmd(compCtx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cmd, nil
	})
}

func (workloads) registerState() {
	// TODO(ericsnow) Use a more general registration mechanism.
	//state.RegisterMultiEnvCollections(persistence.Collections...)

	newUnitWorkloads := func(persist state.Persistence, unit names.UnitTag) (state.UnitWorkloads, error) {
		return workloadstate.NewUnitWorkloads(persist, unit), nil
	}
	state.SetWorkloadsComponent(newUnitWorkloads)

	newEnvPayloads := func(persist state.PayloadsEnvPersistence) (state.EnvPayloads, error) {
		envPersist := persistence.NewEnvPersistence(persist)
		envPayloads := workloadstate.EnvPayloads{
			Persist: envPersist,
		}
		return envPayloads, nil
	}
	state.SetPayloadsComponent(newEnvPayloads)
}
