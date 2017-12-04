// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker/caasoperator/hook"
	"github.com/juju/juju/worker/caasoperator/runner/context"
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
}

// NewRunnerFactoryFunc is a function that returns a hook/cmd/action runner factory.
type NewRunnerFactoryFunc func(context.Paths, context.ContextFactory) (Factory, error)

// NewFactory returns a Factory capable of creating runners for executing
// charm hooks, actions and commands.
func NewFactory(
	paths context.Paths,
	contextFactory context.ContextFactory,
) (
	Factory, error,
) {
	f := &factory{
		paths:          paths,
		contextFactory: contextFactory,
	}

	return f, nil
}

type factory struct {
	contextFactory context.ContextFactory

	// Fields that shouldn't change in a factory's lifetime.
	paths context.Paths
}

// NewCommandRunner exists to satisfy the Factory interface.
func (f *factory) NewCommandRunner(commandInfo context.CommandInfo) (Runner, error) {
	ctx, err := f.contextFactory.CommandContext(commandInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	runner := NewRunner(ctx, f.paths)
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
	runner := NewRunner(ctx, f.paths)
	return runner, nil
}
