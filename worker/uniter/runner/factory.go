// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/hook"
)

type CommandInfo struct {
	// RelationId is the relation context to execute the commands in.
	RelationId int
	// RemoteUnitName is the remote unit for the relation context.
	RemoteUnitName string
	// ForceRemoteUnit skips unit inference and existence validation.
	ForceRemoteUnit bool
}

// Factory represents a long-lived object that can create runners
// relevant to a specific unit.
type Factory interface {

	// NewCommandRunner returns an execution context suitable for running
	// an arbitrary script.
	NewCommandRunner(commandInfo CommandInfo) (Runner, error)

	// NewHookRunner returns an execution context suitable for running the
	// supplied hook definition (which must be valid).
	NewHookRunner(hookInfo hook.Info) (Runner, error)

	// NewActionRunner returns an execution context suitable for running the
	// action identified by the supplied id.
	NewActionRunner(actionId string) (Runner, error)
}

// NewFactory returns a Factory capable of creating runners for executing
// charm hooks, actions and commands.
func NewFactory(
	state *uniter.State,
	paths Paths,
	contextFactory ContextFactory,
) (
	Factory, error,
) {
	f := &factory{
		state:          state,
		paths:          paths,
		contextFactory: contextFactory,
	}

	return f, nil
}

type factory struct {
	contextFactory ContextFactory

	// API connection fields.
	state *uniter.State

	// Fields that shouldn't change in a factory's lifetime.
	paths Paths
}

// NewCommandRunner exists to satisfy the Factory interface.
func (f *factory) NewCommandRunner(commandInfo CommandInfo) (Runner, error) {
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

// NewActionRunner exists to satisfy the Factory interface.
func (f *factory) NewActionRunner(actionId string) (Runner, error) {
	ch, err := getCharm(f.paths.GetCharmDir())
	if err != nil {
		return nil, errors.Trace(err)
	}

	ok := names.IsValidAction(actionId)
	if !ok {
		return nil, &badActionError{actionId, "not valid actionId"}
	}
	tag := names.NewActionTag(actionId)
	action, err := f.state.Action(tag)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) {
		return nil, ErrActionNotAvailable
	} else if params.IsCodeActionNotAvailable(err) {
		return nil, ErrActionNotAvailable
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	name := action.Name()
	spec, ok := ch.Actions().ActionSpecs[name]
	if !ok {
		return nil, &badActionError{name, "not defined"}
	}
	params := action.Params()
	if err := spec.ValidateParams(params); err != nil {
		return nil, &badActionError{name, err.Error()}
	}

	actionData := newActionData(name, &tag, params)
	ctx, err := f.contextFactory.ActionContext(actionData)
	runner := NewRunner(ctx, f.paths)
	return runner, nil
}

func getCharm(charmPath string) (charm.Charm, error) {
	ch, err := charm.ReadCharm(charmPath)
	if err != nil {
		return nil, err
	}
	return ch, nil
}
