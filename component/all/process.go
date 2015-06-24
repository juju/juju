// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/process/plugin"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type workloadProcesses struct{}

func (c workloadProcesses) registerForServer() error {
	c.registerHookContext()
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
