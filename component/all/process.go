// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
	"github.com/juju/juju/process/api/server"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/process/plugin"
	procstate "github.com/juju/juju/process/state"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type workloadProcesses struct{}

func (c workloadProcesses) registerForServer() error {
	c.registerHookContext()
	c.registerState()
	return nil
}

func (c workloadProcesses) registerForClient() error {
	return nil
}

func (c workloadProcesses) registerHookContext() {
	if !markRegistered(process.ComponentName, "hook-context") {
		return
	}

	runner.RegisterComponentFunc(process.ComponentName,
		func() (jujuc.ContextComponent, error) {
			// TODO(ericsnow) The API client or facade should be passed
			// in to the factory func and passed to NewInternalClient.
			client, err := api.NewInternalClient()
			if err != nil {
				return nil, errors.Trace(err)
			}
			component, err := context.NewContextAPI(client)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return component, nil
		},
	)

	c.registerHookContextCommands()

	factory := func(state *state.State, _ *common.Resources, authorizer common.Authorizer) (*server.HookContextAPI, error) {
		if !authorizer.AuthUnitAgent() {
			return nil, common.ErrPerm
		}

		// TODO(natefinch): uncomment when the appropriate state functions exist.
		// return server.NewHookContextAPI(state), nil
		return nil, common.ErrPerm
	}

	common.RegisterStandardFacade(
		process.ComponentName,
		0,
		factory,
	)
}

type workloadProcessesHookContext struct {
	jujuc.Context
}

// Component implements context.HookContext.
func (c workloadProcessesHookContext) Component(name string) (context.Component, error) {
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

func (workloadProcesses) registerHookContextCommands() {
	if !markRegistered(process.ComponentName, "hook-context-commands") {
		return
	}

	jujuc.RegisterCommand("register", func(ctx jujuc.Context) cmd.Command {
		compCtx := workloadProcessesHookContext{ctx}
		cmd, err := context.NewProcRegistrationCommand(compCtx)
		if err != nil {
			// TODO(ericsnow) Return an error instead.
			panic(err)
		}
		return cmd
	})

	jujuc.RegisterCommand("launch", func(ctx jujuc.Context) cmd.Command {
		compCtx := workloadProcessesHookContext{ctx}
		cmd, err := context.NewProcLaunchCommand(plugin.Find, plugin.Plugin.Launch, compCtx)
		if err != nil {
			// TODO(ericsnow) Return an error instead.
			panic(err)
		}
		return cmd
	})
}

func (c workloadProcesses) registerState() {
	newUnitProcesses := func(persist state.Persistence, unit names.UnitTag, charm names.CharmTag) (state.UnitProcesses, error) {
		return procstate.NewUnitProcesses(persist, unit, &charm), nil
	}
	newProcessDefinitions := func(persist state.Persistence, charm names.CharmTag) (state.ProcessDefinitions, error) {
		return procstate.NewDefinitions(persist, charm), nil
	}
	state.SetProcessesComponent(newUnitProcesses, newProcessDefinitions)
}
