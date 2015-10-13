// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"reflect"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	apiserverclient "github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/commands"
	cmdstatus "github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/cmd/jujud/agent/unit"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	"github.com/juju/juju/worker/util"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/api/client"
	"github.com/juju/juju/workload/api/server"
	"github.com/juju/juju/workload/context"
	workloadstate "github.com/juju/juju/workload/state"
	"github.com/juju/juju/workload/status"
	"github.com/juju/juju/workload/workers"
)

const workloadsHookContextFacade = workload.ComponentName + "-hook-context"

type workloads struct{}

func (c workloads) registerForServer() error {
	c.registerState()
	c.registerPublicFacade()

	addEvents := c.registerUnitWorkers()
	c.registerHookContext(addEvents)
	c.registerUnitStatus()

	return nil
}

func (c workloads) registerForClient() error {
	c.registerPublicCommands()
	cmdstatus.RegisterUnitStatusFormatter(workload.ComponentName, status.Format)
	return nil
}

func (workloads) newPublicFacade(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*server.PublicAPI, error) {
	if st == nil {
		return nil, errors.NewNotValid(nil, "st is nil")
	}

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

func (c workloads) registerHookContext(addEvents func(...workload.Event) error) {
	if !markRegistered(workload.ComponentName, "hook-context") {
		return
	}

	runner.RegisterComponentFunc(workload.ComponentName,
		func(config runner.ComponentConfig) (jujuc.ContextComponent, error) {
			hctxClient := c.newHookContextAPIClient(config.APICaller)
			// TODO(ericsnow) Pass the unit's tag through to the component?
			component, err := context.NewContextAPI(hctxClient, config.DataDir, addEvents)
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
	return client.NewHookContextClient(facadeCaller)
}

func (workloads) newHookContextFacade(st *state.State, unit *state.Unit) (interface{}, error) {
	if st == nil {
		return nil, errors.NewNotValid(nil, "st is nil")
	}

	up, err := st.UnitWorkloads(unit)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return server.NewHookContextAPI(up), nil
}

func (c workloads) registerHookContextFacade() {
	common.RegisterHookContextFacade(
		workloadsHookContextFacade,
		0,
		c.newHookContextFacade,
		reflect.TypeOf(&server.HookContextAPI{}),
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

	jujuc.RegisterCommand(context.RegisterCmdName, func(ctx jujuc.Context) cmd.Command {
		compCtx := workloadsHookContext{ctx}
		cmd, err := context.NewRegisterCmd(compCtx)
		if err != nil {
			// TODO(ericsnow) Return an error instead.
			panic(err)
		}
		return cmd
	})

	jujuc.RegisterCommand(context.StatusSetCmdName, func(ctx jujuc.Context) cmd.Command {
		compCtx := workloadsHookContext{ctx}
		cmd, err := context.NewStatusSetCmd(compCtx)
		if err != nil {
			// TODO(ericsnow) Return an error instead.
			panic(err)
		}
		return cmd
	})

	name := context.TrackCommandInfo.Name
	jujuc.RegisterCommand(name, func(ctx jujuc.Context) cmd.Command {
		compCtx := workloadsHookContext{ctx}
		cmd, err := context.NewWorkloadTrackCommand(compCtx)
		if err != nil {
			// TODO(ericsnow) Return an error instead.
			panic(err)
		}
		return cmd
	})

	jujuc.RegisterCommand(context.UntrackCmdName, func(ctx jujuc.Context) cmd.Command {
		compCtx := workloadsHookContext{ctx}
		cmd, err := context.NewUntrackCmd(compCtx)
		if err != nil {
			// TODO(ericsnow) Return an error instead.
			panic(err)
		}
		return cmd
	})

	name = context.LaunchCommandInfo.Name
	jujuc.RegisterCommand(name, func(ctx jujuc.Context) cmd.Command {
		compCtx := workloadsHookContext{ctx}
		cmd, err := context.NewWorkloadLaunchCommand(compCtx)
		if err != nil {
			panic(err)
		}
		return cmd
	})

	name = context.InfoCommandInfo.Name
	jujuc.RegisterCommand(name, func(ctx jujuc.Context) cmd.Command {
		compCtx := workloadsHookContext{ctx}
		cmd, err := context.NewWorkloadInfoCommand(compCtx)
		if err != nil {
			panic(err)
		}
		return cmd
	})
}

// TODO(ericsnow) Use a watcher instead of passing around the event handlers?

func (c workloads) registerUnitWorkers() func(...workload.Event) error {
	if !markRegistered(workload.ComponentName, "workers") {
		return nil
	}

	handlerFuncs := []func([]workload.Event, context.APIClient, workers.Runner) error{
		workers.WorkloadHandler,
	}

	unitHandlers := workers.NewEventHandlers()
	for _, handlerFunc := range handlerFuncs {
		unitHandlers.RegisterHandler(handlerFunc)
	}

	newManifold := func(config unit.ComponentManifoldConfig) (dependency.Manifold, error) {
		// At this point no workload workers are running for the unit.
		// TODO(ericsnow) Move this code to workers.Manifold
		// (and ManifoldConfig)?
		apiConfig := util.AgentApiManifoldConfig{
			APICallerName: config.APICallerName,
			AgentName:     config.AgentName,
		}
		manifold := util.AgentApiManifold(apiConfig, func(unitAgent agent.Agent, caller base.APICaller) (worker.Worker, error) {
			apiClient := c.newHookContextAPIClient(caller)
			config := unitAgent.CurrentConfig()
			dataDir := workload.DataDir(agent.Dir(config.DataDir(), config.Tag()))
			unitHandlers.Reset(apiClient, dataDir)
			return unitHandlers.StartEngine()
		})
		return manifold, nil
	}
	err := unit.RegisterComponentManifoldFunc(workload.ComponentName, newManifold)
	if err != nil {
		panic(err)
	}

	return unitHandlers.AddEvents
}

func (workloads) registerState() {
	// TODO(ericsnow) Use a more general registration mechanism.
	//state.RegisterMultiEnvCollections(persistence.Collections...)

	newUnitWorkloads := func(persist state.Persistence, unit names.UnitTag) (state.UnitWorkloads, error) {
		return workloadstate.NewUnitWorkloads(persist, unit), nil
	}
	state.SetWorkloadsComponent(newUnitWorkloads)

	newEnvPayloads := func(persist state.Persistence, listMachines func() ([]string, error), listUnits func(string) ([]names.UnitTag, error)) (state.EnvPayloads, error) {
		envPayloads := &workloadstate.EnvPayloads{
			Base:         persist,
			ListMachines: listMachines,
			ListUnits:    listUnits,
		}
		return envPayloads, nil
	}
	state.SetPayloadsComponent(newEnvPayloads)
}

func (workloads) registerUnitStatus() {
	apiserverclient.RegisterStatusProviderForUnits(workload.ComponentName,
		func(unit *state.Unit) (interface{}, error) {
			up, err := unit.Workloads()
			if err != nil {
				return nil, err
			}
			workloads, err := up.List()
			if err != nil {
				return nil, err
			}
			return status.UnitStatus(workloads)
		})
}
