// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type workloadProcesses struct{}

func (c workloadProcesses) registerAll() {
	c.registerHookContext()
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

func (workloadProcesses) registerHookContextCommands() {
	if !markRegistered(process.ComponentName, "hook-context-commands") {
		return
	}

	jujuc.RegisterCommand("register", func(ctx jujuc.Context) cmd.Command {
		cmd, err := context.NewProcRegistrationCommand(ctx)
		if err != nil {
			panic(err)
		}
		return cmd
	})
}
