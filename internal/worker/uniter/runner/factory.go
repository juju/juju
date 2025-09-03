// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	stdcontext "context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/agent/uniter"
	coreoperation "github.com/juju/juju/core/operation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
)

// Factory represents a long-lived object that can create runners
// relevant to a specific unit.
type Factory interface {

	// NewCommandRunner returns an execution context suitable for running
	// an arbitrary script.
	NewCommandRunner(stdCtx stdcontext.Context, commandInfo context.CommandInfo) (Runner, error)

	// NewHookRunner returns an execution context suitable for running the
	// supplied hook definition (which must be valid).
	NewHookRunner(stdCtx stdcontext.Context, hookInfo hook.Info) (Runner, error)

	// NewActionRunner returns an execution context suitable for running the action.
	NewActionRunner(stdCtx stdcontext.Context, action *uniter.Action, cancel <-chan struct{}) (Runner, error)
}

// NewFactory returns a Factory capable of creating runners for executing
// charm hooks, actions and commands.
func NewFactory(
	paths context.Paths,
	contextFactory context.ContextFactory,
	newProcessRunner NewRunnerFunc,
) (
	Factory, error,
) {
	f := &factory{
		paths:            paths,
		contextFactory:   contextFactory,
		newProcessRunner: newProcessRunner,
	}

	return f, nil
}

type factory struct {
	contextFactory context.ContextFactory

	// Fields that shouldn't change in a factory's lifetime.
	paths            context.Paths
	newProcessRunner NewRunnerFunc
}

// NewCommandRunner exists to satisfy the Factory interface.
func (f *factory) NewCommandRunner(stdCtx stdcontext.Context, commandInfo context.CommandInfo) (Runner, error) {
	ctx, err := f.contextFactory.CommandContext(stdCtx, commandInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	runner := f.newProcessRunner(ctx, f.paths)
	return runner, nil
}

// NewHookRunner exists to satisfy the Factory interface.
func (f *factory) NewHookRunner(stdCtx stdcontext.Context, hookInfo hook.Info) (Runner, error) {
	if err := hookInfo.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	ctx, err := f.contextFactory.HookContext(stdCtx, hookInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	runner := f.newProcessRunner(ctx, f.paths)
	return runner, nil
}

// NewActionRunner exists to satisfy the Factory interface.
func (f *factory) NewActionRunner(stdCtx stdcontext.Context, action *uniter.Action, cancel <-chan struct{}) (Runner, error) {
	charmDir := f.paths.GetCharmDir()
	meta, err := charm.ReadCharmDirMetadata(charmDir)
	if err != nil {
		return nil, errors.Trace(err)
	}
	actions, err := charm.ReadCharmDirActions(meta.Name, f.paths.GetCharmDir())
	if err != nil && !errors.Is(err, charm.FileNotFound) {
		return nil, errors.Trace(err)
	}

	name := action.Name()
	spec, ok := coreoperation.PredefinedActionsSpec[name]
	if !ok && actions != nil {
		var ok bool
		spec, ok = actions.ActionSpecs[name]
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
	ctx, err := f.contextFactory.ActionContext(stdCtx, actionData)
	if err != nil {
		return nil, charmrunner.NewBadActionError(name, err.Error())
	}
	runner := f.newProcessRunner(ctx, f.paths)
	return runner, nil
}
