// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
)

// Factory represents a long-lived object that can create runners
// relevant to a specific unit.
type Factory interface {

	// NewCommandRunner returns an execution context suitable for running
	// an arbitrary script.
	NewCommandRunner(commandInfo context.CommandInfo) (Runner, error)

	// NewHookRunner returns an execution context suitable for running the
	// supplied hook definition (which must be valid).
	NewHookRunner(hookInfo hook.Info) (Runner, error)

	// NewActionRunner returns an execution context suitable for running the action.
	NewActionRunner(action *uniter.Action, cancel <-chan struct{}) (Runner, error)
}

// NewFactory returns a Factory capable of creating runners for executing
// charm hooks, actions and commands.
func NewFactory(
	paths context.Paths,
	contextFactory context.ContextFactory,
	newProcessRunner NewRunnerFunc,
	remoteExecutor ExecFunc,
) (
	Factory, error,
) {
	f := &factory{
		paths:            paths,
		contextFactory:   contextFactory,
		newProcessRunner: newProcessRunner,
		remoteExecutor:   remoteExecutor,
	}

	return f, nil
}

type factory struct {
	contextFactory context.ContextFactory

	// Fields that shouldn't change in a factory's lifetime.
	paths            context.Paths
	newProcessRunner NewRunnerFunc
	remoteExecutor   ExecFunc
}

// NewCommandRunner exists to satisfy the Factory interface.
func (f *factory) NewCommandRunner(commandInfo context.CommandInfo) (Runner, error) {
	ctx, err := f.contextFactory.CommandContext(commandInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	runner := f.newProcessRunner(ctx, f.paths, f.remoteExecutor)
	return runner, nil
}

// NewHookRunner exists to satisfy the Factory interface.
func (f *factory) NewHookRunner(hookInfo hook.Info) (Runner, error) {
	if err := hookInfo.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	ctx, err := f.contextFactory.HookContext(hookInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	runner := f.newProcessRunner(ctx, f.paths, f.remoteExecutor)
	return runner, nil
}

// NewActionRunner exists to satisfy the Factory interface.
func (f *factory) NewActionRunner(action *uniter.Action, cancel <-chan struct{}) (Runner, error) {
	ch, err := getCharm(f.paths.GetCharmDir())
	if err != nil {
		return nil, errors.Trace(err)
	}

	name := action.Name()
	spec, ok := actions.PredefinedActionsSpec[name]
	if !ok {
		var ok bool
		spec, ok = ch.Actions().ActionSpecs[name]
		if !ok {
			return nil, charmrunner.NewBadActionError(name, "not defined")
		}
	}

	params := action.Params()
	if err := spec.ValidateParams(params); err != nil {
		return nil, charmrunner.NewBadActionError(name, err.Error())
	}

	tag := names.NewActionTag(action.ID())
	actionData := context.NewActionData(name, &tag, params, cancel)
	ctx, err := f.contextFactory.ActionContext(actionData)
	if err != nil {
		return nil, charmrunner.NewBadActionError(name, err.Error())
	}
	runner := f.newProcessRunner(ctx, f.paths, f.remoteExecutor)
	return runner, nil
}

func getCharm(charmPath string) (charm.Charm, error) {
	ch, err := charm.ReadCharm(charmPath)
	if err != nil {
		return nil, err
	}
	return ch, nil
}
