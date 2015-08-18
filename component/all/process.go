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
	"github.com/juju/juju/cmd/jujud/agent/unit"
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
		func(unit string, caller base.APICaller) (jujuc.ContextComponent, error) {
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

	handlerFuncs := []func([]process.Event, context.APIClient, workers.Runner) error{
		workers.StatusEventHandler,
	}

	newWorkerFunc := func(config unit.ManifoldsConfig, caller base.APICaller) (func() (worker.Worker, error), error) {
		// At this point no workload process workers are running for the unit.

		unitName := config.Agent.CurrentConfig().Tag().String()
		if unitHandler, ok := unitEventHandlers[unitName]; ok {
			// The worker must have restarted.
			// TODO(ericsnow) Could cause panics?
			unitHandler.Close()
		}

		apiClient := c.newHookContextAPIClient(caller)

		var runner worker.Runner // TODO(ericsnow) Wrap a dependency engine in a runner.
		unitHandler := workers.NewEventHandlers(apiClient, runner)
		for _, handlerFunc := range handlerFuncs {
			unitHandler.RegisterHandler(handlerFunc)
		}
		unitEventHandlers[unitName] = unitHandler

		// Pull all existing from State (via API) and add an event for each.
		hctx, err := context.NewContextAPI(apiClient, unitHandler.AddEvents)
		if err != nil {
			return nil, errors.Trace(err)
		}
		events, err := c.initialEvents(hctx)
		if err != nil {
			return nil, errors.Trace(err)
		}

		newWorker := func() (worker.Worker, error) {
			worker, err := unitHandler.NewWorker()
			if err != nil {
				return nil, errors.Trace(err)
			}
			unitHandler.AddEvents(events...)
			return worker, nil
		}

		// TODO(ericsnow) Start a state watcher?

		return newWorker, nil
	}
	err := unit.RegisterWorker(process.ComponentName, newWorkerFunc)
	if err != nil {
		panic(err)
	}

	return unitEventHandlers
}

func (workloadProcesses) initialEvents(hctx context.Component) ([]process.Event, error) {
	ids, err := hctx.List()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var events []process.Event
	for _, id := range ids {
		proc, err := hctx.Get(id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		//TODO(wwitzel3) (Upgrade/Restart broken) during a restart of the worker, the Plugin loses its absPath for the executable.
		plugin, err := hctx.Plugin(proc)
		if err != nil {
			return nil, errors.Trace(err)
		}

		events = append(events, process.Event{
			Kind:     process.EventKindTracked,
			ID:       proc.ID(),
			Plugin:   plugin,
			PluginID: proc.Details.ID,
		})
	}
	return events, nil
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
