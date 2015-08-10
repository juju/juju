// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"reflect"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/api/base"
	apiserverclient "github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/common"
	cmdstatus "github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api/client"
	"github.com/juju/juju/process/api/server"
	"github.com/juju/juju/process/context"
	procstate "github.com/juju/juju/process/state"
	"github.com/juju/juju/process/status"
	"github.com/juju/juju/process/workers"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type workloadProcesses struct{}

func (c workloadProcesses) registerForServer() error {
	c.registerState()
	handlers := c.registerWorkers()
	c.registerHookContext(handlers)
	c.registerUnitStatus()
	return nil
}

func (workloadProcesses) registerForClient() error {
	cmdstatus.RegisterUnitStatusFormatter(process.ComponentName, status.Format)
	return nil
}

func (c workloadProcesses) registerHookContext(handlers map[string]*workers.EventHandlers) {
	if !markRegistered(process.ComponentName, "hook-context") {
		return
	}

	runner.RegisterComponentFunc(process.ComponentName,
		func(caller base.APICaller) (jujuc.ContextComponent, error) {
			var unit string // TODO(ericsnow) Pass in an arg?
			var addEvents func(...process.Event)
			if unitEventHandler, ok := handlers[unit]; ok {
				addEvents = unitEventHandler.AddEvents
			}
			hctxClient := c.newHookContextAPIClient(caller)
			// TODO(ericsnow) Pass the unit's tag through to the component?
			component, err := context.NewContextAPI(hctxClient, addEvents)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return component, nil
		},
	)

	c.registerHookContextCommands()
	c.registerHookContextFacade()
}

func (c workloadProcesses) newHookContextAPIClient(caller base.APICaller) context.APIClient {
	facadeCaller := base.NewFacadeCallerForVersion(caller, process.ComponentName, 0)
	return client.NewHookContextClient(facadeCaller)
}

func (workloadProcesses) registerHookContextFacade() {

	newHookContextApi := func(st *state.State, unit *state.Unit) (interface{}, error) {
		if st == nil {
			return nil, errors.NewNotValid(nil, "st is nil")
		}

		up, err := st.UnitProcesses(unit)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return server.NewHookContextAPI(up), nil
	}

	common.RegisterHookContextFacade(
		process.ComponentName,
		0,
		newHookContextApi,
		reflect.TypeOf(&server.HookContextAPI{}),
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

	name := context.RegisterCommandInfo.Name
	jujuc.RegisterCommand(name, func(ctx jujuc.Context) cmd.Command {
		compCtx := workloadProcessesHookContext{ctx}
		cmd, err := context.NewProcRegistrationCommand(compCtx)
		if err != nil {
			// TODO(ericsnow) Return an error instead.
			panic(err)
		}
		return cmd
	})

	name = context.LaunchCommandInfo.Name
	jujuc.RegisterCommand(name, func(ctx jujuc.Context) cmd.Command {
		compCtx := workloadProcessesHookContext{ctx}
		cmd, err := context.NewProcLaunchCommand(compCtx)
		if err != nil {
			panic(err)
		}
		return cmd
	})

	name = context.InfoCommandInfo.Name
	jujuc.RegisterCommand(name, func(ctx jujuc.Context) cmd.Command {
		compCtx := workloadProcessesHookContext{ctx}
		cmd, err := context.NewProcInfoCommand(compCtx)
		if err != nil {
			panic(err)
		}
		return cmd
	})
}

func (c workloadProcesses) registerWorkers() map[string]*workers.EventHandlers {
	if !markRegistered(process.ComponentName, "workers") {
		return nil
	}
	unitEventHandlers := make(map[string]*workers.EventHandlers)

	handlerFuncs := []func([]process.Event) error{
	// Add to-be-registered handlers here.
	}

	newWorkerFunc := func(unit string) func() (worker.Worker, error) {
		// At this point no workload process workers are running for the unit.
		if unitHandler, ok := unitEventHandlers[unit]; ok {
			// The worker must have restarted.
			// TODO(ericsnow) Could cause panics?
			unitHandler.Close()
		}

		unitHandler := workers.NewEventHandlers()
		for _, handlerFunc := range handlerFuncs {
			unitHandler.RegisterHandler(handlerFunc)
		}
		unitEventHandlers[unit] = unitHandler

		// TODO(ericsnow) Pull all existing from State (via API) and add
		// an event for each.

		// TODO(ericsnow) Handlers need ability to start or stop workers.
		// TODO(ericsnow) Handlers need ability to make API calls.

		// TODO(ericsnow) Start a state watcher?

		return unitHandler.NewWorker
	}
	err := agent.RegisterUnitAgentWorker(process.ComponentName, newWorkerFunc)
	if err != nil {
		panic(err)
	}

	return unitEventHandlers
}

func (workloadProcesses) registerState() {
	// TODO(ericsnow) Use a more general registration mechanism.
	//state.RegisterMultiEnvCollections(persistence.Collections...)

	newUnitProcesses := func(persist state.Persistence, unit names.UnitTag, getMetadata func() (*charm.Meta, error)) (state.UnitProcesses, error) {
		return procstate.NewUnitProcesses(persist, unit, getMetadata), nil
	}
	state.SetProcessesComponent(newUnitProcesses)
}

func (workloadProcesses) registerUnitStatus() {
	apiserverclient.RegisterStatusProviderForUnits(process.ComponentName,
		func(unit *state.Unit) (interface{}, error) {
			up, err := unit.Processes()
			if err != nil {
				return nil, err
			}
			procs, err := up.List()
			if err != nil {
				return nil, err
			}
			return status.UnitStatus(procs)
		})
}
