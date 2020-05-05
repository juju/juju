// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/worker/common/charmrunner"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner/context"
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

	// NewActionRunner returns an execution context suitable for running the
	// action identified by the supplied id.
	NewActionRunner(actionId string, cancel <-chan struct{}) (Runner, error)
}

// NewFactory returns a Factory capable of creating runners for executing
// charm hooks, actions and commands.
func NewFactory(
	state *uniter.State,
	paths context.Paths,
	contextFactory context.ContextFactory,
	remoteExecutor ExecFunc,
) (
	Factory, error,
) {
	f := &factory{
		state:          state,
		paths:          paths,
		contextFactory: contextFactory,
		remoteExecutor: remoteExecutor,
	}

	return f, nil
}

type factory struct {
	contextFactory context.ContextFactory

	// API connection fields.
	state *uniter.State

	// Fields that shouldn't change in a factory's lifetime.
	paths          context.Paths
	remoteExecutor ExecFunc
}

// NewCommandRunner exists to satisfy the Factory interface.
func (f *factory) NewCommandRunner(commandInfo context.CommandInfo) (Runner, error) {
	ctx, err := f.contextFactory.CommandContext(commandInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	runner := NewRunner(ctx, f.paths, f.remoteExecutor)
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
	runner := NewRunner(ctx, f.paths, f.remoteExecutor)
	return runner, nil
}

// NewActionRunner exists to satisfy the Factory interface.
func (f *factory) NewActionRunner(actionId string, cancel <-chan struct{}) (Runner, error) {
	ch, err := getCharm(f.paths.GetCharmDir())
	if err != nil {
		return nil, errors.Trace(err)
	}

	ok := names.IsValidAction(actionId)
	if !ok {
		return nil, charmrunner.NewBadActionError(actionId, "not valid actionId")
	}
	tag := names.NewActionTag(actionId)
	action, err := f.state.Action(tag)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) {
		return nil, charmrunner.ErrActionNotAvailable
	} else if params.IsCodeActionNotAvailable(err) {
		return nil, charmrunner.ErrActionNotAvailable
	} else if err != nil {
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

	actionData := context.NewActionData(name, &tag, params, cancel)
	ctx, err := f.contextFactory.ActionContext(actionData)
	if err != nil {
		return nil, charmrunner.NewBadActionError(name, err.Error())
	}
	runner := NewRunner(ctx, f.paths, f.remoteExecutor)
	return runner, nil
}

func getCharm(charmPath string) (charm.Charm, error) {
	ch, err := charm.ReadCharm(charmPath)
	if err != nil {
		return nil, err
	}
	return ch, nil
}
